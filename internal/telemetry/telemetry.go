// Package telemetry implements fixed-size async telemetry buffers and frames.
package telemetry

import (
	"encoding/json"
	"io"
	"math"
	"sync/atomic"

	"matchpoint/contracts"
)

const (
	DefaultRingCapacity uint64 = 65_536
	SystemSegment       uint8  = 255
)

type EventType uint8

const (
	EventQueueDepth EventType = iota
	EventMatchMade
	EventChurnAlert
	EventAllocSnapshot
	EventTickDuration
	EventRedisLatency
	EventSimDrop
)

type Status uint8

const (
	StatusOK Status = iota
	StatusInvalidConfig
	StatusEmpty
	StatusInvalidInput
)

// Event is the fixed telemetry payload stored in ring slots.
type Event struct {
	TimestampUnixNano int64
	Value1            float32
	Value2            float32
	Segment           uint8
	Type              EventType
}

type eventSlot struct {
	timestamp atomic.Int64
	value1    atomic.Uint32
	value2    atomic.Uint32
	meta      atomic.Uint32
}

type Ring struct {
	head     atomic.Uint64
	tail     atomic.Uint64
	capacity uint64
	slots    []eventSlot
}

func NewRing(capacity uint64) (*Ring, Status) {
	if capacity == 0 {
		return nil, StatusInvalidConfig
	}
	return &Ring{
		capacity: capacity,
		slots:    make([]eventSlot, capacity),
	}, StatusOK
}

func (r *Ring) Write(event Event) Status {
	if r == nil || r.capacity == 0 {
		return StatusInvalidInput
	}
	head := r.head.Load()
	next := head + 1
	r.slots[head%r.capacity].store(event)
	r.head.Store(next)
	if next-r.tail.Load() > r.capacity {
		r.tail.Store(next - r.capacity)
	}
	return StatusOK
}

func (r *Ring) Read(out *Event) Status {
	if r == nil || out == nil || r.capacity == 0 {
		return StatusInvalidInput
	}
	tail := r.tail.Load()
	if tail >= r.head.Load() {
		return StatusEmpty
	}
	r.slots[tail%r.capacity].load(out)
	r.tail.Store(tail + 1)
	return StatusOK
}

func (r *Ring) Len() uint64 {
	if r == nil || r.capacity == 0 {
		return 0
	}
	head := r.head.Load()
	tail := r.tail.Load()
	if head < tail {
		return 0
	}
	length := head - tail
	if length > r.capacity {
		return r.capacity
	}
	return length
}

func (s *eventSlot) store(event Event) {
	s.timestamp.Store(event.TimestampUnixNano)
	s.value1.Store(math.Float32bits(event.Value1))
	s.value2.Store(math.Float32bits(event.Value2))
	s.meta.Store(uint32(event.Segment)<<8 | uint32(event.Type))
}

func (s *eventSlot) load(out *Event) {
	meta := s.meta.Load()
	*out = Event{
		TimestampUnixNano: s.timestamp.Load(),
		Value1:            math.Float32frombits(s.value1.Load()),
		Value2:            math.Float32frombits(s.value2.Load()),
		Segment:           uint8(meta >> 8),
		Type:              EventType(meta),
	}
}

type Frame struct {
	TimestampUnixNano int64     `json:"ts"`
	QueueDepths       [5]uint32 `json:"queueDepths"`
	MatchesLastTick   uint32    `json:"matchesLastTick"`
	EOMMAccuracy      float32   `json:"eommAccuracy"`
	AllocBytesHeap    uint64    `json:"allocBytesHeap"`
	ChurnAlerts       uint32    `json:"churnAlerts"`
}

type Sink struct {
	ring              *Ring
	matchesLastTick   atomic.Uint32
	churnAlerts       atomic.Uint32
	simDrops          atomic.Uint64
	redisLatencyNanos atomic.Int64
	queueDepths       [5]atomic.Uint32
}

func NewSink(ring *Ring) *Sink {
	if ring == nil {
		ring, _ = NewRing(DefaultRingCapacity)
	}
	return &Sink{ring: ring}
}

func (s *Sink) RecordTick(metrics contracts.MatchTickMetrics) {
	if s == nil {
		return
	}
	s.matchesLastTick.Store(metrics.MatchesMade)
	s.write(Event{
		TimestampUnixNano: int64(metrics.TickID),
		Type:              EventMatchMade,
		Segment:           SystemSegment,
		Value1:            float32(metrics.MatchesMade),
		Value2:            float32(metrics.DurationNanos),
	})
}

func (s *Sink) RecordOverrun(tickID uint64, durationNanos int64, consecutive uint32) {
	if s == nil {
		return
	}
	s.write(Event{
		TimestampUnixNano: int64(tickID),
		Type:              EventTickDuration,
		Segment:           SystemSegment,
		Value1:            float32(durationNanos),
		Value2:            float32(consecutive),
	})
}

func (s *Sink) RecordSkippedTick(tickID uint64) {
	if s == nil {
		return
	}
	s.write(Event{TimestampUnixNano: int64(tickID), Type: EventTickDuration, Segment: SystemSegment, Value2: 1})
}

func (s *Sink) RecordDualBooking(matchID uint64) {
	if s == nil {
		return
	}
	s.write(Event{TimestampUnixNano: int64(matchID), Type: EventMatchMade, Segment: SystemSegment, Value2: 1})
}

func (s *Sink) RecordEmptyQuery(pool contracts.RedisQueuePool) {
	if s == nil {
		return
	}
	segment := uint8(pool)
	if int(segment) < len(s.queueDepths) {
		s.queueDepths[segment].Store(0)
	}
	s.write(Event{Type: EventQueueDepth, Segment: segment})
}

func (s *Sink) RecordRedisStatus(status contracts.RedisQueueStatus, elapsedNanos int64) {
	if s == nil {
		return
	}
	s.redisLatencyNanos.Store(elapsedNanos)
	s.write(Event{
		Type:    EventRedisLatency,
		Segment: SystemSegment,
		Value1:  float32(elapsedNanos),
		Value2:  float32(status),
	})
}

func (s *Sink) RecordSimDrop(playerID uint64) {
	if s == nil {
		return
	}
	s.simDrops.Add(1)
	s.write(Event{TimestampUnixNano: int64(playerID), Type: EventSimDrop, Segment: SystemSegment})
}

func (s *Sink) RecordQueueDepth(segment uint8, depth uint32, timestampUnixNano int64) {
	if s == nil {
		return
	}
	if int(segment) < len(s.queueDepths) {
		s.queueDepths[segment].Store(depth)
	}
	s.write(Event{
		TimestampUnixNano: timestampUnixNano,
		Type:              EventQueueDepth,
		Segment:           segment,
		Value1:            float32(depth),
	})
}

func (s *Sink) RecordChurnAlert(event contracts.EOMMChurnAlertEvent, timestampUnixNano int64) {
	if s == nil {
		return
	}
	s.churnAlerts.Add(1)
	s.write(Event{
		TimestampUnixNano: timestampUnixNano,
		Type:              EventChurnAlert,
		Segment:           SystemSegment,
		Value1:            event.CurrentChurnRisk,
		Value2:            event.RollingWinRate,
	})
}

func (s *Sink) SnapshotFrame(timestampUnixNano int64, eommAccuracy float32, allocBytesHeap uint64, out *Frame) Status {
	if s == nil || out == nil {
		return StatusInvalidInput
	}
	*out = Frame{
		TimestampUnixNano: timestampUnixNano,
		MatchesLastTick:   s.matchesLastTick.Load(),
		EOMMAccuracy:      eommAccuracy,
		AllocBytesHeap:    allocBytesHeap,
		ChurnAlerts:       s.churnAlerts.Load(),
	}
	for i := range out.QueueDepths {
		out.QueueDepths[i] = s.queueDepths[i].Load()
	}
	return StatusOK
}

func EmitFrame(w io.Writer, frame Frame) Status {
	if w == nil {
		return StatusInvalidInput
	}
	if err := json.NewEncoder(w).Encode(frame); err != nil {
		return StatusInvalidInput
	}
	return StatusOK
}

func (s *Sink) write(event Event) {
	if s.ring != nil {
		_ = s.ring.Write(event)
	}
}
