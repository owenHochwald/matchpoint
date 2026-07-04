package telemetry

import (
	"bytes"
	"runtime"
	"testing"
	"unsafe"

	"matchpoint/contracts"
)

func TestRingWriteReadAndOverwrite(t *testing.T) {
	ring, status := NewRing(2)
	if status != StatusOK {
		t.Fatalf("NewRing status=%v", status)
	}

	if status := ring.Write(Event{TimestampUnixNano: 1, Type: EventQueueDepth, Segment: 1, Value1: 10}); status != StatusOK {
		t.Fatalf("Write status=%v", status)
	}
	if status := ring.Write(Event{TimestampUnixNano: 2, Type: EventMatchMade, Segment: SystemSegment, Value1: 1}); status != StatusOK {
		t.Fatalf("Write status=%v", status)
	}
	if status := ring.Write(Event{TimestampUnixNano: 3, Type: EventChurnAlert, Segment: SystemSegment, Value1: 0.8}); status != StatusOK {
		t.Fatalf("Write status=%v", status)
	}
	if got := ring.Len(); got != 2 {
		t.Fatalf("Len=%d, want 2", got)
	}

	var event Event
	if status := ring.Read(&event); status != StatusOK || event.TimestampUnixNano != 2 || event.Type != EventMatchMade {
		t.Fatalf("first read status=%v event=%+v", status, event)
	}
	if status := ring.Read(&event); status != StatusOK || event.TimestampUnixNano != 3 || event.Type != EventChurnAlert {
		t.Fatalf("second read status=%v event=%+v", status, event)
	}
	if status := ring.Read(&event); status != StatusEmpty {
		t.Fatalf("empty read status=%v", status)
	}
}

func TestSinkImplementsMetricsAndBuildsFrame(t *testing.T) {
	ring, _ := NewRing(8)
	sink := NewSink(ring)

	var matchSink contracts.MatchMetricsSink = sink
	var simMetrics contracts.SimMetrics = sink

	matchSink.RecordTick(contracts.MatchTickMetrics{TickID: 10, MatchesMade: 47, DurationNanos: 123})
	matchSink.RecordRedisStatus(contracts.RedisStatusTimeout, 99)
	matchSink.RecordEmptyQuery(contracts.RedisPoolSegment2)
	sink.RecordQueueDepth(2, 340, 20)
	sink.RecordChurnAlert(contracts.EOMMChurnAlertEvent{CurrentChurnRisk: 0.9, RollingWinRate: 0.2}, 30)
	simMetrics.RecordSimDrop(99)

	var frame Frame
	if status := sink.SnapshotFrame(40, 0.82, 4_194_304, &frame); status != StatusOK {
		t.Fatalf("SnapshotFrame status=%v", status)
	}
	if frame.TimestampUnixNano != 40 || frame.QueueDepths[2] != 340 ||
		frame.MatchesLastTick != 47 || frame.EOMMAccuracy != 0.82 ||
		frame.AllocBytesHeap != 4_194_304 || frame.ChurnAlerts != 1 {
		t.Fatalf("frame=%+v", frame)
	}
	if ring.Len() != 6 {
		t.Fatalf("ring Len=%d, want 6", ring.Len())
	}
}

func TestEmitFrameWritesNewlineDelimitedJSON(t *testing.T) {
	var buf bytes.Buffer
	frame := Frame{
		TimestampUnixNano: 1718000000000000000,
		QueueDepths:       [5]uint32{120, 340, 89, 12, 3},
		MatchesLastTick:   47,
		EOMMAccuracy:      0.82,
		AllocBytesHeap:    4_194_304,
		ChurnAlerts:       3,
	}
	if status := EmitFrame(&buf, frame); status != StatusOK {
		t.Fatalf("EmitFrame status=%v", status)
	}
	want := `{"ts":1718000000000000000,"queueDepths":[120,340,89,12,3],"matchesLastTick":47,"eommAccuracy":0.82,"allocBytesHeap":4194304,"churnAlerts":3}` + "\n"
	if got := buf.String(); got != want {
		t.Fatalf("json=%q, want %q", got, want)
	}
}

func TestEventSizeBound(t *testing.T) {
	if size := unsafe.Sizeof(Event{}); size > 24 {
		t.Fatalf("Event size=%d, want <= 24", size)
	}
}

func BenchmarkRingWriteGOMAXPROCS1(b *testing.B) {
	benchmarkRingWrite(b, 1)
}

func BenchmarkRingWriteGOMAXPROCSCPU(b *testing.B) {
	benchmarkRingWrite(b, runtime.NumCPU())
}

func BenchmarkRingReadGOMAXPROCS1(b *testing.B) {
	benchmarkRingRead(b, 1)
}

func BenchmarkRingReadGOMAXPROCSCPU(b *testing.B) {
	benchmarkRingRead(b, runtime.NumCPU())
}

func BenchmarkRecordTickGOMAXPROCS1(b *testing.B) {
	benchmarkRecordTick(b, 1)
}

func BenchmarkRecordTickGOMAXPROCSCPU(b *testing.B) {
	benchmarkRecordTick(b, runtime.NumCPU())
}

func benchmarkRingWrite(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	ring, _ := NewRing(1024)
	event := Event{TimestampUnixNano: 1, Type: EventMatchMade, Segment: SystemSegment, Value1: 1}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if status := ring.Write(event); status != StatusOK {
			b.Fatalf("Write status=%v", status)
		}
	}
}

func benchmarkRingRead(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	ring, _ := NewRing(1024)
	event := Event{TimestampUnixNano: 1, Type: EventMatchMade, Segment: SystemSegment, Value1: 1}
	var out Event
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ring.Write(event)
		if status := ring.Read(&out); status != StatusOK {
			b.Fatalf("Read status=%v", status)
		}
	}
}

func benchmarkRecordTick(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	ring, _ := NewRing(1024)
	sink := NewSink(ring)
	metrics := contracts.MatchTickMetrics{TickID: 1, MatchesMade: 1, DurationNanos: 10}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink.RecordTick(metrics)
	}
}
