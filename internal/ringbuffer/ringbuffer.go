// Package ringbuffer implements the MatchPoint ingestion-to-match-core handoff.
package ringbuffer

import (
	"errors"
	"runtime"
	"sync/atomic"
	"time"

	"matchpoint/contracts"
)

const (
	RingBackpressureWaitNanos = contracts.RingBackpressureWaitNanos

	duplicateGateOpen   uint32 = 0
	duplicateGateClosed uint32 = 1
)

const (
	RingStateOpen     = contracts.RingStateOpen
	RingStateDraining = contracts.RingStateDraining
	RingStateClosed   = contracts.RingStateClosed
)

const (
	RingWriteAccepted            = contracts.RingWriteAccepted
	RingWriteFull                = contracts.RingWriteFull
	RingWriteBackpressureTimeout = contracts.RingWriteBackpressureTimeout
	RingWriteClosed              = contracts.RingWriteClosed
	RingWriteDuplicatePublisher  = contracts.RingWriteDuplicatePublisher
)

const (
	RingReadOK     = contracts.RingReadOK
	RingReadEmpty  = contracts.RingReadEmpty
	RingReadClosed = contracts.RingReadClosed
)

type RingState = contracts.RingState
type RingWriteStatus = contracts.RingWriteStatus
type RingReadStatus = contracts.RingReadStatus
type RingShardID = contracts.RingShardID
type RingSequence = contracts.RingSequence
type RingConfig = contracts.RingConfig
type RingCursor = contracts.RingCursor
type RingSlot = contracts.RingSlot
type RingShard = contracts.RingShard
type RingSnapshot = contracts.RingSnapshot
type RingWriteResult = contracts.RingWriteResult
type RingReadResult = contracts.RingReadResult
type RingDrainResult = contracts.RingDrainResult
type ShardSelector = contracts.ShardSelector
type TicketRingBuffer = contracts.TicketRingBuffer

var (
	errZeroShards      = errors.New("ringbuffer: shards must be greater than zero")
	errInvalidCapacity = errors.New("ringbuffer: capacity per shard must be a non-zero power of two")
	errDuplicateWindow = errors.New("ringbuffer: duplicate window per shard must be greater than zero")
)

type ringBuffer struct {
	shards     []ringShard
	shardCount uint64
}

type ringShard struct {
	contracts.RingShard
	duplicateGate atomic.Uint32
	duplicates    []atomic.Uint64
	onFullWait    func()
}

func newRingBuffer(config contracts.RingConfig) (*ringBuffer, error) {
	if config.Shards == 0 {
		return nil, errZeroShards
	}
	if !isPowerOfTwo(config.CapacityPerShard) {
		return nil, errInvalidCapacity
	}
	if config.DuplicateWindowPerShard == 0 {
		return nil, errDuplicateWindow
	}

	buffer := &ringBuffer{
		shards:     make([]ringShard, int(config.Shards)),
		shardCount: uint64(config.Shards),
	}
	capacity := uint64(config.CapacityPerShard)
	for shardID := range buffer.shards {
		shard := &buffer.shards[shardID]
		shard.Slots = make([]contracts.RingSlot, int(config.CapacityPerShard))
		shard.Capacity = capacity
		shard.Mask = capacity - 1
		shard.ShardID = contracts.RingShardID(shardID)
		shard.State.Store(uint32(contracts.RingStateOpen))
		shard.duplicates = make([]atomic.Uint64, int(config.DuplicateWindowPerShard))
		for slotIndex := range shard.Slots {
			shard.Slots[slotIndex].Sequence.Store(uint64(slotIndex))
		}
	}
	return buffer, nil
}

// NewRingBuffer creates an ingestion handoff ring buffer.
func NewRingBuffer(config contracts.RingConfig) (contracts.TicketRingBuffer, error) {
	return newRingBuffer(config)
}

func isPowerOfTwo(value uint32) bool {
	return value != 0 && value&(value-1) == 0
}

func (b *ringBuffer) ShardForPlayer(playerID uint64) contracts.RingShardID {
	if b == nil || b.shardCount == 0 {
		return 0
	}
	return contracts.RingShardID(playerID % b.shardCount)
}

func (b *ringBuffer) WriteTicket(ticket *contracts.Ticket, nowUnixNano int64) contracts.RingWriteResult {
	_ = nowUnixNano
	if b == nil || ticket == nil || ticket.PlayerID == 0 || b.shardCount == 0 {
		return contracts.RingWriteResult{Status: contracts.RingWriteClosed}
	}

	shardID := b.ShardForPlayer(ticket.PlayerID)
	shard := &b.shards[shardID]
	result := contracts.RingWriteResult{ShardID: shardID}
	if shard.state() != contracts.RingStateOpen {
		result.Status = contracts.RingWriteClosed
		return result
	}

	if !shard.claimDuplicate(ticket.PlayerID) {
		result.Status = contracts.RingWriteDuplicatePublisher
		return result
	}

	sequence, accepted, full := shard.tryWrite(ticket)
	if accepted {
		result.Status = contracts.RingWriteAccepted
		result.Sequence = sequence
		return result
	}
	if !full {
		shard.releaseDuplicate(ticket.PlayerID)
		result.Status = contracts.RingWriteClosed
		return result
	}

	if shard.onFullWait != nil {
		shard.onFullWait()
	}
	start := time.Now().UnixNano()
	deadline := start + contracts.RingBackpressureWaitNanos
	for spins := uint32(0); ; spins++ {
		if shard.state() != contracts.RingStateOpen {
			shard.releaseDuplicate(ticket.PlayerID)
			result.Status = contracts.RingWriteClosed
			result.WaitedNanos = clampWait(time.Now().UnixNano() - start)
			return result
		}
		sequence, accepted, full = shard.tryWrite(ticket)
		if accepted {
			result.Status = contracts.RingWriteAccepted
			result.Sequence = sequence
			result.WaitedNanos = clampWait(time.Now().UnixNano() - start)
			return result
		}
		if !full {
			shard.releaseDuplicate(ticket.PlayerID)
			result.Status = contracts.RingWriteClosed
			result.WaitedNanos = clampWait(time.Now().UnixNano() - start)
			return result
		}
		if time.Now().UnixNano() >= deadline {
			shard.releaseDuplicate(ticket.PlayerID)
			result.Status = contracts.RingWriteBackpressureTimeout
			result.WaitedNanos = contracts.RingBackpressureWaitNanos
			return result
		}
		if spins&7 == 7 {
			runtime.Gosched()
		}
	}
}

func clampWait(waited int64) int64 {
	if waited < 0 {
		return 0
	}
	if waited > contracts.RingBackpressureWaitNanos {
		return contracts.RingBackpressureWaitNanos
	}
	return waited
}

func (s *ringShard) tryWrite(ticket *contracts.Ticket) (contracts.RingSequence, bool, bool) {
	for {
		if s.state() != contracts.RingStateOpen {
			return 0, false, false
		}
		tail := s.Tail.Value.Load()
		head := s.Head.Value.Load()
		if tail-head >= s.Capacity {
			return 0, false, true
		}
		slot := &s.Slots[tail&s.Mask]
		if slot.Sequence.Load() != tail {
			runtime.Gosched()
			continue
		}
		if !s.Tail.Value.CompareAndSwap(tail, tail+1) {
			continue
		}

		sequence := tail + 1
		slot.Ticket = ticket
		slot.PlayerID.Store(ticket.PlayerID)
		slot.Sequence.Store(sequence)
		slot.Published.Store(1)
		return contracts.RingSequence(sequence), true, false
	}
}

func (b *ringBuffer) ReadTicket(shardID contracts.RingShardID) contracts.RingReadResult {
	if b == nil || int(shardID) >= len(b.shards) {
		return contracts.RingReadResult{Status: contracts.RingReadClosed, ShardID: shardID}
	}
	shard := &b.shards[shardID]
	for {
		head := shard.Head.Value.Load()
		tail := shard.Tail.Value.Load()
		if head >= tail {
			if shard.state() == contracts.RingStateOpen {
				return contracts.RingReadResult{Status: contracts.RingReadEmpty, ShardID: shardID}
			}
			shard.State.CompareAndSwap(uint32(contracts.RingStateDraining), uint32(contracts.RingStateClosed))
			return contracts.RingReadResult{Status: contracts.RingReadClosed, ShardID: shardID}
		}

		slot := &shard.Slots[head&shard.Mask]
		if slot.Sequence.Load() != head+1 || slot.Published.Load() != 1 {
			if shard.state() == contracts.RingStateOpen {
				return contracts.RingReadResult{Status: contracts.RingReadEmpty, ShardID: shardID}
			}
			return contracts.RingReadResult{Status: contracts.RingReadEmpty, ShardID: shardID}
		}
		if !shard.Head.Value.CompareAndSwap(head, head+1) {
			continue
		}

		ticket := slot.Ticket
		sequence := slot.Sequence.Load()
		playerID := slot.PlayerID.Load()
		slot.Ticket = nil
		slot.PlayerID.Store(0)
		slot.Published.Store(0)
		slot.Sequence.Store(head + shard.Capacity)
		shard.releaseDuplicate(playerID)
		return contracts.RingReadResult{
			Status:   contracts.RingReadOK,
			ShardID:  shardID,
			Ticket:   ticket,
			Sequence: contracts.RingSequence(sequence),
		}
	}
}

func (b *ringBuffer) DrainShard(shardID contracts.RingShardID, dst []*contracts.Ticket) contracts.RingDrainResult {
	if b == nil || int(shardID) >= len(b.shards) {
		return contracts.RingDrainResult{Status: contracts.RingReadClosed, ShardID: shardID}
	}
	var count uint32
	for count < uint32(len(dst)) {
		result := b.ReadTicket(shardID)
		if result.Status != contracts.RingReadOK {
			if count > 0 {
				return contracts.RingDrainResult{
					Status:    contracts.RingReadOK,
					ShardID:   shardID,
					Count:     count,
					Remaining: b.remaining(shardID),
				}
			}
			return contracts.RingDrainResult{
				Status:    result.Status,
				ShardID:   shardID,
				Remaining: b.remaining(shardID),
			}
		}
		dst[count] = result.Ticket
		count++
	}
	if count > 0 {
		return contracts.RingDrainResult{
			Status:    contracts.RingReadOK,
			ShardID:   shardID,
			Count:     count,
			Remaining: b.remaining(shardID),
		}
	}
	return contracts.RingDrainResult{
		Status:    contracts.RingReadEmpty,
		ShardID:   shardID,
		Remaining: b.remaining(shardID),
	}
}

func (b *ringBuffer) SnapshotShard(shardID contracts.RingShardID) contracts.RingSnapshot {
	if b == nil || int(shardID) >= len(b.shards) {
		return contracts.RingSnapshot{ShardID: shardID, State: contracts.RingStateClosed}
	}
	shard := &b.shards[shardID]
	head := shard.Head.Value.Load()
	tail := shard.Tail.Value.Load()
	depth := uint64(0)
	if tail >= head {
		depth = tail - head
	}
	if depth > shard.Capacity {
		depth = shard.Capacity
	}
	return contracts.RingSnapshot{
		ShardID:  shardID,
		State:    shard.state(),
		Head:     head,
		Tail:     tail,
		Capacity: shard.Capacity,
		Depth:    depth,
	}
}

func (b *ringBuffer) CloseShard(shardID contracts.RingShardID) contracts.RingState {
	if b == nil || int(shardID) >= len(b.shards) {
		return contracts.RingStateClosed
	}
	shard := &b.shards[shardID]
	for {
		state := shard.state()
		if state != contracts.RingStateOpen {
			return state
		}
		if shard.State.CompareAndSwap(uint32(contracts.RingStateOpen), uint32(contracts.RingStateDraining)) {
			return contracts.RingStateDraining
		}
	}
}

func (b *ringBuffer) Close() contracts.RingState {
	if b == nil {
		return contracts.RingStateClosed
	}
	final := contracts.RingStateClosed
	for shardID := range b.shards {
		state := b.CloseShard(contracts.RingShardID(shardID))
		if state != contracts.RingStateClosed {
			final = contracts.RingStateDraining
		}
	}
	return final
}

func (b *ringBuffer) remaining(shardID contracts.RingShardID) uint64 {
	if b == nil || int(shardID) >= len(b.shards) {
		return 0
	}
	snapshot := b.SnapshotShard(shardID)
	return snapshot.Depth
}

func (s *ringShard) state() contracts.RingState {
	return contracts.RingState(s.State.Load())
}

func (s *ringShard) claimDuplicate(playerID uint64) bool {
	if playerID == 0 || len(s.duplicates) == 0 {
		return false
	}
	for !s.duplicateGate.CompareAndSwap(duplicateGateOpen, duplicateGateClosed) {
		runtime.Gosched()
	}
	defer s.duplicateGate.Store(duplicateGateOpen)

	free := -1
	for index := range s.duplicates {
		current := s.duplicates[index].Load()
		if current == playerID {
			return false
		}
		if current == 0 && free < 0 {
			free = index
		}
	}
	if free < 0 {
		return false
	}
	s.duplicates[free].Store(playerID)
	return true
}

func (s *ringShard) releaseDuplicate(playerID uint64) {
	if playerID == 0 || len(s.duplicates) == 0 {
		return
	}
	for !s.duplicateGate.CompareAndSwap(duplicateGateOpen, duplicateGateClosed) {
		runtime.Gosched()
	}
	for index := range s.duplicates {
		if s.duplicates[index].Load() == playerID {
			s.duplicates[index].Store(0)
			break
		}
	}
	s.duplicateGate.Store(duplicateGateOpen)
}
