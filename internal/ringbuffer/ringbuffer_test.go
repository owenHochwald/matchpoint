package ringbuffer

import (
	"runtime"
	"sync"
	"testing"
	"unsafe"

	"matchpoint/contracts"
)

func TestConstructionValidationAndShardInitialization(t *testing.T) {
	t.Run("B-RINGBUFFER-1 zero shards fail", func(t *testing.T) {
		if _, err := newRingBuffer(contracts.RingConfig{CapacityPerShard: 2, DuplicateWindowPerShard: 2}); err == nil {
			t.Fatal("expected zero-shard config to fail")
		}
	})
	t.Run("B-RINGBUFFER-2 non-power-of-two capacity fails", func(t *testing.T) {
		if _, err := newRingBuffer(contracts.RingConfig{Shards: 1, CapacityPerShard: 3, DuplicateWindowPerShard: 3}); err == nil {
			t.Fatal("expected non-power-of-two capacity to fail")
		}
	})
	t.Run("B-RINGBUFFER-3 valid config preallocates open shards", func(t *testing.T) {
		buffer := mustRing(t, 4, 8, 16)
		for shardID := range buffer.shards {
			shard := &buffer.shards[shardID]
			if shard.Capacity != uint64(len(shard.Slots)) {
				t.Fatalf("capacity mismatch: got %d len %d", shard.Capacity, len(shard.Slots))
			}
			if shard.Mask != shard.Capacity-1 {
				t.Fatalf("mask mismatch: got %d", shard.Mask)
			}
			if shard.ShardID != contracts.RingShardID(shardID) {
				t.Fatalf("shard id mismatch: got %d want %d", shard.ShardID, shardID)
			}
			if got := shard.state(); got != contracts.RingStateOpen {
				t.Fatalf("state = %d, want open", got)
			}
			if len(shard.Slots) != 8 {
				t.Fatalf("slots not preallocated")
			}
		}
	})
}

func TestShardForPlayerIsStableModulo(t *testing.T) {
	buffer := mustRing(t, 5, 2, 8)
	for _, playerID := range []uint64{1, 2, 5, 9, 42, 99} {
		first := buffer.ShardForPlayer(playerID)
		second := buffer.ShardForPlayer(playerID)
		if first != contracts.RingShardID(playerID%5) || second != first {
			t.Fatalf("B-RINGBUFFER-4 shard = %d/%d, want %d", first, second, playerID%5)
		}
	}
}

func TestBackpressureBudgetConstant(t *testing.T) {
	if RingBackpressureWaitNanos != 10_000 {
		t.Fatalf("B-RINGBUFFER-5 budget = %d, want 10000", RingBackpressureWaitNanos)
	}
}

func TestWriteReadOrderingAndPublication(t *testing.T) {
	buffer := mustRing(t, 1, 4, 8)
	tickets := []*contracts.Ticket{ticket(1), ticket(2), ticket(3)}

	for i, ticket := range tickets {
		result := buffer.WriteTicket(ticket, int64(i))
		if result.Status != contracts.RingWriteAccepted {
			t.Fatalf("B-RINGBUFFER-6 write %d status = %d", i, result.Status)
		}
		if result.ShardID != 0 {
			t.Fatalf("shard = %d, want 0", result.ShardID)
		}
		if result.Sequence == 0 {
			t.Fatalf("B-RINGBUFFER-27 accepted write returned zero sequence")
		}
		slot := &buffer.shards[0].Slots[uint64(i)&buffer.shards[0].Mask]
		if slot.Sequence.Load() == 0 || slot.Published.Load() != 1 || slot.Ticket != ticket {
			t.Fatalf("B-RINGBUFFER-7 slot not fully published before readable state")
		}
	}

	for i, want := range tickets {
		result := buffer.ReadTicket(0)
		if result.Status != contracts.RingReadOK {
			t.Fatalf("B-RINGBUFFER-13 read %d status = %d", i, result.Status)
		}
		if result.Ticket != want {
			t.Fatalf("read %d ticket = %p, want %p", i, result.Ticket, want)
		}
		if result.Sequence == 0 {
			t.Fatalf("B-RINGBUFFER-14 read sequence was zero")
		}
	}
}

func TestFullShardTimeoutAndNoOverwrite(t *testing.T) {
	// B-RINGBUFFER-30 is covered with this timeout path and the freed-during-wait test below.
	buffer := mustRing(t, 1, 1, 4)
	first := ticket(10)
	if result := buffer.WriteTicket(first, 0); result.Status != contracts.RingWriteAccepted {
		t.Fatalf("initial write status = %d", result.Status)
	}

	second := ticket(11)
	result := buffer.WriteTicket(second, 1)
	if result.Status != contracts.RingWriteBackpressureTimeout {
		t.Fatalf("B-RINGBUFFER-8/B-RINGBUFFER-10 status = %d, want timeout", result.Status)
	}
	if result.WaitedNanos > RingBackpressureWaitNanos {
		t.Fatalf("waited %d > budget %d", result.WaitedNanos, RingBackpressureWaitNanos)
	}

	read := buffer.ReadTicket(0)
	if read.Ticket != first {
		t.Fatalf("overwrite occurred: got %p want first %p", read.Ticket, first)
	}
	if second.PlayerID != 11 {
		t.Fatalf("caller did not retain rejected ticket ownership")
	}
}

func TestFullShardAcceptsSlotFreedDuringBoundedWait(t *testing.T) {
	buffer := mustRing(t, 1, 1, 4)
	first := ticket(20)
	if result := buffer.WriteTicket(first, 0); result.Status != contracts.RingWriteAccepted {
		t.Fatalf("initial write status = %d", result.Status)
	}

	freed := false
	buffer.shards[0].onFullWait = func() {
		if freed {
			return
		}
		freed = true
		if result := buffer.ReadTicket(0); result.Status != contracts.RingReadOK || result.Ticket != first {
			t.Fatalf("B-RINGBUFFER-9 setup read status=%d ticket=%p", result.Status, result.Ticket)
		}
	}

	result := buffer.WriteTicket(ticket(21), 1)
	if result.Status != contracts.RingWriteAccepted {
		t.Fatalf("B-RINGBUFFER-9 status = %d, want accepted", result.Status)
	}
	if result.WaitedNanos < 0 || result.WaitedNanos > RingBackpressureWaitNanos {
		t.Fatalf("waited = %d outside budget", result.WaitedNanos)
	}
}

func TestDuplicatePublisherRejectionAndConcurrentRace(t *testing.T) {
	buffer := mustRing(t, 1, 16, 32)
	if result := buffer.WriteTicket(ticket(30), 0); result.Status != contracts.RingWriteAccepted {
		t.Fatalf("initial write status = %d", result.Status)
	}
	duplicate := buffer.WriteTicket(ticket(30), 1)
	if duplicate.Status != contracts.RingWriteDuplicatePublisher {
		t.Fatalf("B-RINGBUFFER-11 duplicate status = %d", duplicate.Status)
	}

	buffer = mustRing(t, 1, 16, 32)
	const publishers = 16
	var wg sync.WaitGroup
	results := make([]contracts.RingWriteStatus, publishers)
	wg.Add(publishers)
	for index := 0; index < publishers; index++ {
		go func(index int) {
			defer wg.Done()
			results[index] = buffer.WriteTicket(ticket(31), int64(index)).Status
		}(index)
	}
	wg.Wait()
	accepted := 0
	for _, status := range results {
		if status == contracts.RingWriteAccepted {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("B-RINGBUFFER-12 accepted = %d, want 1", accepted)
	}
}

func TestReadEmptyInvalidShardAndNonOKZeroValues(t *testing.T) {
	buffer := mustRing(t, 1, 2, 4)
	empty := buffer.ReadTicket(0)
	if empty.Status != contracts.RingReadEmpty || empty.Ticket != nil {
		t.Fatalf("B-RINGBUFFER-15 empty result = %+v", empty)
	}
	head := buffer.shards[0].Head.Value.Load()
	if empty.Sequence != 0 || buffer.shards[0].Head.Value.Load() != head {
		t.Fatalf("B-RINGBUFFER-28/B-RINGBUFFER-15 non-OK read mutated or retained data")
	}

	if result := buffer.ReadTicket(99); result.Status != contracts.RingReadClosed {
		t.Fatalf("B-RINGBUFFER-18 invalid read status = %d", result.Status)
	}
	if result := buffer.DrainShard(99, nil); result.Status != contracts.RingReadClosed {
		t.Fatalf("B-RINGBUFFER-18 invalid drain status = %d", result.Status)
	}
	if snapshot := buffer.SnapshotShard(99); snapshot.State != contracts.RingStateClosed {
		t.Fatalf("B-RINGBUFFER-18 invalid snapshot = %+v", snapshot)
	}
	if state := buffer.CloseShard(99); state != contracts.RingStateClosed {
		t.Fatalf("B-RINGBUFFER-18 invalid close state = %d", state)
	}
}

func TestDrainShardBoundedByDestination(t *testing.T) {
	buffer := mustRing(t, 1, 8, 16)
	for playerID := uint64(40); playerID < 45; playerID++ {
		if result := buffer.WriteTicket(ticket(playerID), 0); result.Status != contracts.RingWriteAccepted {
			t.Fatalf("write status = %d", result.Status)
		}
	}

	dst := make([]*contracts.Ticket, 3)
	result := buffer.DrainShard(0, dst)
	if result.Status != contracts.RingReadOK || result.Count != 3 {
		t.Fatalf("B-RINGBUFFER-16 drain result = %+v", result)
	}
	for index, got := range dst {
		if got == nil || got.PlayerID != uint64(40+index) {
			t.Fatalf("dst[%d] = %+v", index, got)
		}
	}
	if result.Remaining != 2 {
		t.Fatalf("B-RINGBUFFER-29 remaining = %d, want 2", result.Remaining)
	}

	dst = make([]*contracts.Ticket, 4)
	result = buffer.DrainShard(0, dst)
	if result.Status != contracts.RingReadOK || result.Count != 2 {
		t.Fatalf("B-RINGBUFFER-17 partial drain result = %+v", result)
	}
	result = buffer.DrainShard(0, dst)
	if result.Status != contracts.RingReadEmpty || result.Count != 0 {
		t.Fatalf("B-RINGBUFFER-17 empty drain result = %+v", result)
	}
}

func TestCloseShardAndCloseLifecycle(t *testing.T) {
	buffer := mustRing(t, 2, 4, 8)
	first := ticket(50)
	if result := buffer.WriteTicket(first, 0); result.Status != contracts.RingWriteAccepted {
		t.Fatalf("write status = %d", result.Status)
	}
	shardID := buffer.ShardForPlayer(first.PlayerID)
	if state := buffer.CloseShard(shardID); state != contracts.RingStateDraining {
		t.Fatalf("B-RINGBUFFER-19 close shard state = %d", state)
	}
	if result := buffer.WriteTicket(ticket(first.PlayerID+2), 0); result.Status != contracts.RingWriteClosed {
		t.Fatalf("B-RINGBUFFER-19 write after close status = %d", result.Status)
	}
	if result := buffer.ReadTicket(shardID); result.Status != contracts.RingReadOK || result.Ticket != first {
		t.Fatalf("drain readable during close result = %+v", result)
	}
	if result := buffer.ReadTicket(shardID); result.Status != contracts.RingReadClosed {
		t.Fatalf("B-RINGBUFFER-20 closed read status = %d", result.Status)
	}
	if result := buffer.ReadTicket(shardID); result.Status != contracts.RingReadClosed {
		t.Fatalf("B-RINGBUFFER-22 closed read status = %d", result.Status)
	}
	if result := buffer.DrainShard(shardID, make([]*contracts.Ticket, 2)); result.Status != contracts.RingReadClosed || result.Count != 0 {
		t.Fatalf("B-RINGBUFFER-22 closed drain result = %+v", result)
	}

	buffer = mustRing(t, 2, 4, 8)
	if result := buffer.WriteTicket(ticket(51), 0); result.Status != contracts.RingWriteAccepted {
		t.Fatalf("write status = %d", result.Status)
	}
	if state := buffer.Close(); state != contracts.RingStateDraining {
		t.Fatalf("B-RINGBUFFER-21 close state = %d", state)
	}
	if result := buffer.WriteTicket(ticket(53), 0); result.Status != contracts.RingWriteClosed {
		t.Fatalf("B-RINGBUFFER-21 write after close status = %d", result.Status)
	}
}

func TestSnapshotConcurrentAndDepthClamped(t *testing.T) {
	buffer := mustRing(t, 1, 8, 16)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for playerID := uint64(60); playerID < 64; playerID++ {
			_ = buffer.WriteTicket(ticket(playerID), 0)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 4; i++ {
			_ = buffer.SnapshotShard(0)
			runtime.Gosched()
		}
	}()
	wg.Wait()
	snapshot := buffer.SnapshotShard(0)
	if snapshot.ShardID != 0 || snapshot.Capacity != 8 || snapshot.Depth > snapshot.Capacity {
		t.Fatalf("B-RINGBUFFER-23 snapshot = %+v", snapshot)
	}
}

func TestCacheLineAndSequenceWrapInvariants(t *testing.T) {
	if size := unsafe.Sizeof(contracts.RingCursor{}); size != 64 {
		t.Fatalf("B-RINGBUFFER-24 cursor size = %d, want 64", size)
	}

	buffer := mustRing(t, 1, 2, 4)
	for i := uint64(0); i < 10; i++ {
		write := buffer.WriteTicket(ticket(100+i), 0)
		if write.Status != contracts.RingWriteAccepted {
			t.Fatalf("wrap write %d status = %d", i, write.Status)
		}
		read := buffer.ReadTicket(0)
		if read.Status != contracts.RingReadOK || read.Ticket.PlayerID != 100+i {
			t.Fatalf("B-RINGBUFFER-25 wrap read = %+v", read)
		}
	}
	if head, tail := buffer.shards[0].Head.Value.Load(), buffer.shards[0].Tail.Value.Load(); head != 10 || tail != 10 {
		t.Fatalf("B-RINGBUFFER-26 cursors head=%d tail=%d", head, tail)
	}
}

func TestInvalidTicketsRejected(t *testing.T) {
	buffer := mustRing(t, 1, 2, 4)
	if result := buffer.WriteTicket(nil, 0); result.Status == contracts.RingWriteAccepted {
		t.Fatal("nil ticket was accepted")
	}
	if result := buffer.WriteTicket(ticket(0), 0); result.Status == contracts.RingWriteAccepted {
		t.Fatal("zero-player ticket was accepted")
	}
}

func BenchmarkShardForPlayerGOMAXPROCS1(b *testing.B) {
	benchmarkShardForPlayer(b, 1)
}

func BenchmarkShardForPlayerGOMAXPROCSCPU(b *testing.B) {
	benchmarkShardForPlayer(b, runtime.NumCPU())
}

func BenchmarkWriteTicketAcceptedGOMAXPROCS1(b *testing.B) {
	benchmarkWriteAccepted(b, 1)
}

func BenchmarkWriteTicketAcceptedGOMAXPROCSCPU(b *testing.B) {
	benchmarkWriteAccepted(b, runtime.NumCPU())
}

func BenchmarkWriteTicketTimeoutGOMAXPROCS1(b *testing.B) {
	benchmarkWriteTimeout(b, 1)
}

func BenchmarkWriteTicketTimeoutGOMAXPROCSCPU(b *testing.B) {
	benchmarkWriteTimeout(b, runtime.NumCPU())
}

func BenchmarkReadTicketGOMAXPROCS1(b *testing.B) {
	benchmarkReadTicket(b, 1)
}

func BenchmarkReadTicketGOMAXPROCSCPU(b *testing.B) {
	benchmarkReadTicket(b, runtime.NumCPU())
}

func BenchmarkDrainShardGOMAXPROCS1(b *testing.B) {
	benchmarkDrainShard(b, 1)
}

func BenchmarkDrainShardGOMAXPROCSCPU(b *testing.B) {
	benchmarkDrainShard(b, runtime.NumCPU())
}

func BenchmarkSnapshotShardGOMAXPROCS1(b *testing.B) {
	benchmarkSnapshotShard(b, 1)
}

func BenchmarkSnapshotShardGOMAXPROCSCPU(b *testing.B) {
	benchmarkSnapshotShard(b, runtime.NumCPU())
}

func BenchmarkCloseShardGOMAXPROCS1(b *testing.B) {
	benchmarkCloseShard(b, 1)
}

func BenchmarkCloseShardGOMAXPROCSCPU(b *testing.B) {
	benchmarkCloseShard(b, runtime.NumCPU())
}

func BenchmarkCloseGOMAXPROCS1(b *testing.B) {
	benchmarkClose(b, 1)
}

func BenchmarkCloseGOMAXPROCSCPU(b *testing.B) {
	benchmarkClose(b, runtime.NumCPU())
}

func benchmarkShardForPlayer(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	buffer := mustBenchRing(b, 64, 1024, 2048)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buffer.ShardForPlayer(uint64(i + 1))
	}
}

func benchmarkWriteAccepted(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	buffer := mustBenchRing(b, 1, 1024, 2048)
	tickets := make([]contracts.Ticket, 1024)
	for i := range tickets {
		tickets[i].PlayerID = uint64(i + 1)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		index := i & 1023
		if result := buffer.WriteTicket(&tickets[index], 0); result.Status != contracts.RingWriteAccepted {
			b.Fatalf("status = %d", result.Status)
		}
		if result := buffer.ReadTicket(0); result.Status != contracts.RingReadOK {
			b.Fatalf("cleanup read status = %d", result.Status)
		}
	}
}

func benchmarkWriteTimeout(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	buffer := mustBenchRing(b, 1, 1, 4096)
	first := contracts.Ticket{PlayerID: 1}
	if result := buffer.WriteTicket(&first, 0); result.Status != contracts.RingWriteAccepted {
		b.Fatalf("initial status = %d", result.Status)
	}
	rejected := contracts.Ticket{PlayerID: 2}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buffer.WriteTicket(&rejected, 0)
	}
}

func benchmarkReadTicket(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	buffer := mustBenchRing(b, 1, 1024, 2048)
	tickets := make([]contracts.Ticket, 1024)
	for i := range tickets {
		tickets[i].PlayerID = uint64(i + 1)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		index := i & 1023
		if result := buffer.WriteTicket(&tickets[index], 0); result.Status != contracts.RingWriteAccepted {
			b.Fatalf("refill status = %d", result.Status)
		}
		if result := buffer.ReadTicket(0); result.Status != contracts.RingReadOK {
			b.Fatalf("read status = %d", result.Status)
		}
	}
}

func benchmarkDrainShard(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	buffer := mustBenchRing(b, 1, 1024, 2048)
	tickets := make([]contracts.Ticket, 1024)
	for i := range tickets {
		tickets[i].PlayerID = uint64(i + 1)
	}
	dst := make([]*contracts.Ticket, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		index := i & 1023
		if result := buffer.WriteTicket(&tickets[index], 0); result.Status != contracts.RingWriteAccepted {
			b.Fatalf("refill status = %d", result.Status)
		}
		if result := buffer.DrainShard(0, dst); result.Status != contracts.RingReadOK {
			b.Fatalf("drain status = %d", result.Status)
		}
	}
}

func benchmarkSnapshotShard(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	buffer := mustBenchRing(b, 1, 1024, 2048)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buffer.SnapshotShard(0)
	}
}

func benchmarkCloseShard(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	buffer := mustBenchRing(b, 1, 1, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buffer.CloseShard(0)
		buffer.shards[0].State.Store(uint32(contracts.RingStateOpen))
	}
}

func benchmarkClose(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	buffer := mustBenchRing(b, 4, 1, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buffer.Close()
		for shardID := range buffer.shards {
			buffer.shards[shardID].State.Store(uint32(contracts.RingStateOpen))
		}
	}
}

func mustRing(t *testing.T, shards uint16, capacity uint32, duplicateWindow uint32) *ringBuffer {
	t.Helper()
	buffer, err := newRingBuffer(contracts.RingConfig{
		Shards:                  shards,
		CapacityPerShard:        capacity,
		DuplicateWindowPerShard: duplicateWindow,
	})
	if err != nil {
		t.Fatalf("newRingBuffer: %v", err)
	}
	return buffer
}

func mustBenchRing(b *testing.B, shards uint16, capacity uint32, duplicateWindow uint32) *ringBuffer {
	b.Helper()
	buffer, err := newRingBuffer(contracts.RingConfig{
		Shards:                  shards,
		CapacityPerShard:        capacity,
		DuplicateWindowPerShard: duplicateWindow,
	})
	if err != nil {
		b.Fatalf("newRingBuffer: %v", err)
	}
	return buffer
}

func ticket(playerID uint64) *contracts.Ticket {
	return &contracts.Ticket{PlayerID: playerID}
}
