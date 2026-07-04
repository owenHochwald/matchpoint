// Package telemetry implements fixed-size async telemetry buffers and frames.
package telemetry

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"matchpoint/contracts"
)

var errWebSocketPayloadTooLarge = errors.New("telemetry: websocket payload too large")

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

type Server struct {
	Sink          *Sink
	FrameEvery    time.Duration
	NowUnixNano   func() int64
	EOMMAccuracy  func() float32
	AllocSnapshot func() uint64
}

func NewServer(sink *Sink) *Server {
	return &Server{
		Sink:       sink,
		FrameEvery: 100 * time.Millisecond,
		NowUnixNano: func() int64 {
			return time.Now().UnixNano()
		},
		AllocSnapshot: heapAllocBytes,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/telemetry" {
		s.serveWebSocket(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, VisualizerHTML)
}

func (s *Server) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "websocket upgrade required", http.StatusBadRequest)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return
	}
	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	writeHandshake(rw, key)
	rw.Flush()

	interval := s.FrameEvery
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if status := s.writeWebSocketFrame(rw, conn); status != StatusOK {
			return
		}
		rw.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) writeWebSocketFrame(w io.Writer, conn net.Conn) Status {
	if s == nil || s.Sink == nil {
		return StatusInvalidInput
	}
	var frame Frame
	status := s.Sink.SnapshotFrame(s.now(), s.accuracy(), s.allocBytes(), &frame)
	if status != StatusOK {
		return status
	}
	payload, err := json.Marshal(frame)
	if err != nil {
		return StatusInvalidInput
	}
	conn.SetWriteDeadline(time.Now().Add(time.Second))
	if err := WriteWebSocketText(w, payload); err != nil {
		return StatusInvalidInput
	}
	return StatusOK
}

func (s *Server) now() int64 {
	if s.NowUnixNano != nil {
		return s.NowUnixNano()
	}
	return time.Now().UnixNano()
}

func (s *Server) accuracy() float32 {
	if s.EOMMAccuracy != nil {
		return s.EOMMAccuracy()
	}
	return 0
}

func (s *Server) allocBytes() uint64 {
	if s.AllocSnapshot != nil {
		return s.AllocSnapshot()
	}
	return heapAllocBytes()
}

func writeHandshake(w *bufio.ReadWriter, key string) {
	_, _ = io.WriteString(w, "HTTP/1.1 101 Switching Protocols\r\n")
	_, _ = io.WriteString(w, "Upgrade: websocket\r\n")
	_, _ = io.WriteString(w, "Connection: Upgrade\r\n")
	_, _ = io.WriteString(w, "Sec-WebSocket-Accept: "+WebSocketAcceptKey(key)+"\r\n\r\n")
}

func WebSocketAcceptKey(key string) string {
	hash := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(hash[:])
}

func WriteWebSocketText(w io.Writer, payload []byte) error {
	header := [4]byte{0x81}
	switch {
	case len(payload) < 126:
		header[1] = byte(len(payload))
		if _, err := w.Write(header[:2]); err != nil {
			return err
		}
	case len(payload) <= 65535:
		header[1] = 126
		header[2] = byte(len(payload) >> 8)
		header[3] = byte(len(payload))
		if _, err := w.Write(header[:4]); err != nil {
			return err
		}
	default:
		return errWebSocketPayloadTooLarge
	}
	_, err := w.Write(payload)
	return err
}

func heapAllocBytes() uint64 {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

func (s *Sink) write(event Event) {
	if s.ring != nil {
		_ = s.ring.Write(event)
	}
}

const VisualizerHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>MatchPoint Telemetry</title>
<style>
:root{font-family:Inter,ui-sans-serif,system-ui,sans-serif;color:#172033;background:#f5f7fb}
body{margin:0}
main{max-width:1120px;margin:0 auto;padding:28px}
header{display:flex;align-items:end;justify-content:space-between;gap:20px;margin-bottom:24px}
h1{font-size:30px;margin:0}
.status{font-size:13px;color:#526071}
.grid{display:grid;grid-template-columns:repeat(4,1fr);gap:14px;margin-bottom:20px}
.panel{background:white;border:1px solid #dbe2ee;border-radius:8px;padding:16px}
.label{font-size:12px;color:#66758a;text-transform:uppercase}
.value{font-size:28px;font-weight:700;margin-top:6px}
.bars{display:grid;gap:10px}
.bar{height:28px;background:#e8edf6;border-radius:6px;overflow:hidden;position:relative}
.fill{height:100%;background:#1f8a70;width:0}
.bar span{position:absolute;inset:0;display:flex;align-items:center;padding-left:10px;font-size:13px}
@media (max-width:800px){.grid{grid-template-columns:1fr 1fr}header{display:block}}
</style>
</head>
<body>
<main>
<header><div><h1>MatchPoint Telemetry</h1><div class="status" id="status">connecting</div></div></header>
<section class="grid">
<div class="panel"><div class="label">Matches Last Tick</div><div class="value" id="matches">0</div></div>
<div class="panel"><div class="label">EOMM Accuracy</div><div class="value" id="accuracy">0.00</div></div>
<div class="panel"><div class="label">Heap MB</div><div class="value" id="heap">0.0</div></div>
<div class="panel"><div class="label">Churn Alerts</div><div class="value" id="churn">0</div></div>
</section>
<section class="panel"><div class="label">Queue Depths</div><div class="bars" id="bars"></div></section>
</main>
<script>
const bars=document.getElementById('bars');
for(let i=0;i<5;i++){const b=document.createElement('div');b.className='bar';b.innerHTML='<div class="fill"></div><span>Segment '+i+': 0</span>';bars.appendChild(b)}
const ws=new WebSocket('ws://'+location.host+'/telemetry');
ws.onopen=()=>status.textContent='connected';
ws.onclose=()=>status.textContent='disconnected';
ws.onmessage=e=>{const f=JSON.parse(e.data);matches.textContent=f.matchesLastTick;accuracy.textContent=Number(f.eommAccuracy).toFixed(2);heap.textContent=(f.allocBytesHeap/1048576).toFixed(1);churn.textContent=f.churnAlerts;const max=Math.max(1,...f.queueDepths);[...bars.children].forEach((b,i)=>{b.children[0].style.width=(f.queueDepths[i]/max*100)+'%';b.children[1].textContent='Segment '+i+': '+f.queueDepths[i]})};
</script>
</body>
</html>`
