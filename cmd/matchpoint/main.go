package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"matchpoint/contracts"
	"matchpoint/internal/matchcore"
	"matchpoint/internal/redisqueue"
	"matchpoint/internal/ringbuffer"
	"matchpoint/internal/simulation"
	"matchpoint/internal/telemetry"
)

type queueJoin struct {
	PlayerID uint64     `json:"playerId"`
	Trophies int32      `json:"trophies"`
	Vector   [8]float32 `json:"deckVector"`
	Churn    float32    `json:"churnRisk"`
	Spend    float32    `json:"monetizationP"`
	Pool     uint8      `json:"poolTag"`
	Losses   int8       `json:"consecLosses"`
	Wins     int8       `json:"consecWins"`
}

type queueAck struct {
	Status  string `json:"status"`
	Player  uint64 `json:"playerId"`
	Shard   uint16 `json:"shard"`
	Depth   uint64 `json:"depth"`
	Message string `json:"message,omitempty"`
}

type simulateRequest struct {
	Players uint32 `json:"players"`
	Rounds  uint32 `json:"rounds"`
	Seed    uint64 `json:"seed"`
}

type simulateResponse struct {
	Players           uint32    `json:"players"`
	Rounds            uint32    `json:"rounds"`
	Seed              uint64    `json:"seed"`
	ElapsedMillis     int64     `json:"elapsedMillis"`
	Queued            uint64    `json:"queued"`
	Completed         uint64    `json:"completed"`
	Mutated           uint64    `json:"mutated"`
	Quit              uint64    `json:"quit"`
	ConvergenceStatus uint8     `json:"convergenceStatus"`
	Converged         bool      `json:"converged"`
	FailedGate        uint8     `json:"failedGate"`
	SegmentDepths     [6]uint32 `json:"segmentDepths"`
}

type simCounters struct {
	queued    atomic.Uint64
	completed atomic.Uint64
	mutated   atomic.Uint64
	quit      atomic.Uint64
}

type app struct {
	ring      contracts.TicketRingBuffer
	telemetry *telemetry.Sink
}

func main() {
	var (
		addr     = flag.String("addr", envString("MP_ADDR", ":8080"), "HTTP listen address")
		redisURL = flag.String("redis", envString("MP_REDIS_ADDR", "localhost:6379"), "Redis address")
		redisPW  = flag.String("redis-password", envString("MP_REDIS_PASSWORD", ""), "Redis password")
		shards   = flag.Uint("shards", uint(runtime.NumCPU()), "ring buffer shard count")
		capacity = flag.Uint("ring-capacity", 1024, "ring slots per shard, power of two")
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rdb := redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs:    []string{*redisURL},
		Password: *redisPW,
		PoolSize: runtime.NumCPU() * 4,
	})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Error("redis unavailable", "addr", *redisURL, "err", err)
		os.Exit(1)
	}

	redisMetrics := redisqueue.NewMetrics()
	store, scripts := redisqueue.NewUniversalStore(rdb, contracts.RedisQueueConfig{
		CommandTimeoutNanos: contracts.RedisCommandTimeoutNanos,
		PoolSize:            uint16(runtime.NumCPU() * 4),
		CandidateLimit:      contracts.RedisCandidateLimit,
		MatchTTLSeconds:     contracts.RedisMatchTTLSeconds,
	}, redisMetrics)
	if result := scripts.LoadScripts(ctx); result.Status != contracts.RedisStatusOK {
		slog.Error("redis script load failed", "status", result.Status)
		os.Exit(1)
	}

	ring, err := ringbuffer.NewRingBuffer(contracts.RingConfig{
		Shards:                  uint16(*shards),
		CapacityPerShard:        uint32(*capacity),
		DuplicateWindowPerShard: uint32(*capacity) * 2,
	})
	if err != nil {
		slog.Error("ring buffer init failed", "err", err)
		os.Exit(1)
	}

	telemetryRing, status := telemetry.NewRing(telemetry.DefaultRingCapacity)
	if status != telemetry.StatusOK {
		slog.Error("telemetry ring init failed", "status", status)
		os.Exit(1)
	}
	telemetrySink := telemetry.NewSink(telemetryRing)
	core, coreStatus := matchcore.NewMatchCore(matchConfig(), uint16(*shards), ring, store, redisqueue.NewKeyer(), redisqueue.NewScoreCodec(), nil, telemetrySink, nil)
	if coreStatus != contracts.MatchCoreStatusOK {
		slog.Error("matchcore init failed", "status", coreStatus)
		os.Exit(1)
	}

	go func() {
		status := core.Run(ctx)
		slog.Info("matchcore stopped", "status", status)
	}()

	a := &app{ring: ring, telemetry: telemetrySink}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("/queue", a.handleQueue)
	mux.HandleFunc("/simulate", a.handleSimulate)
	mux.Handle("/", telemetry.NewServer(telemetrySink))

	server := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	slog.Info("matchpoint listening", "addr", *addr, "redis", *redisURL)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}

func matchConfig() contracts.MatchCoreConfig {
	return contracts.MatchCoreConfig{
		TickIntervalNanos:    contracts.MatchTickIntervalNanos,
		HardBudgetNanos:      contracts.MatchTickHardBudgetNanos,
		BaseTolerance:        contracts.MatchDefaultBaseTolerance,
		ToleranceK:           contracts.MatchDefaultToleranceK,
		MaxTolerance:         contracts.MatchMaxTolerance,
		OverrunWarnThreshold: contracts.MatchOverrunWarnThreshold,
		DrainBatchSize:       1024,
		CandidateLimit:       contracts.MatchCandidateScratchLimit,
	}
}

func (a *app) handleQueue(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "websocket upgrade required", http.StatusBadRequest)
		return
	}
	conn, rw, ok := acceptWebSocket(w, r)
	if !ok {
		return
	}
	defer conn.Close()

	for {
		payload, err := readClientTextFrame(rw.Reader)
		if err != nil {
			return
		}
		var join queueJoin
		if err := json.Unmarshal(payload, &join); err != nil {
			writeAck(rw, queueAck{Status: "rejected", Message: "malformed json"})
			continue
		}
		ack := a.enqueue(join)
		writeAck(rw, ack)
	}
}

func (a *app) handleSimulate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	var req simulateRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "malformed json", http.StatusBadRequest)
		return
	}
	if req.Players == 0 {
		req.Players = 10_000
	}
	if req.Rounds == 0 {
		req.Rounds = 8
	}
	if req.Seed == 0 {
		req.Seed = 42
	}
	if req.Players > 250_000 || req.Rounds > 64 {
		http.Error(w, "simulation limits exceeded", http.StatusBadRequest)
		return
	}
	resp, err := runSimulation(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func runSimulation(req simulateRequest) (simulateResponse, error) {
	engine, status := simulation.NewEngine(contracts.SimConfig{ConcurrentPlayers: req.Players})
	if status != contracts.SimStatusOK {
		return simulateResponse{}, fmt.Errorf("simulation init failed: %d", status)
	}
	states := make([]contracts.SimPlayerState, int(req.Players))
	if status := engine.SeedPopulation(states, req.Seed); status != contracts.SimStatusOK {
		return simulateResponse{}, fmt.Errorf("seed population failed: %d", status)
	}

	start := time.Now()
	var metrics simCounters
	var wg sync.WaitGroup
	wg.Add(len(states))
	for i := range states {
		i := i
		go func() {
			defer wg.Done()
			runSimPlayer(engine, &states[i], req.Rounds, uint64(i)+req.Seed, &metrics)
		}()
	}
	wg.Wait()

	input := contracts.SimConvergenceInput{
		SegmentDepths:                simSegmentDepths(states),
		LoserPoolDepth:               100,
		RetentionPoolDepth:           200,
		ConsecutiveNonZeroMatchTicks: uint32(minUint64(metrics.completed.Load(), 30)),
		StableHeapTicks:              60,
		WarmupElapsedNanos:           60_000_000_000,
	}
	var convergence contracts.SimConvergenceResult
	convergenceStatus := engine.CheckConvergence(input, &convergence)
	return simulateResponse{
		Players:           req.Players,
		Rounds:            req.Rounds,
		Seed:              req.Seed,
		ElapsedMillis:     time.Since(start).Milliseconds(),
		Queued:            metrics.queued.Load(),
		Completed:         metrics.completed.Load(),
		Mutated:           metrics.mutated.Load(),
		Quit:              metrics.quit.Load(),
		ConvergenceStatus: uint8(convergenceStatus),
		Converged:         convergence.Converged,
		FailedGate:        uint8(convergence.FirstFailedGate),
		SegmentDepths:     input.SegmentDepths,
	}, nil
}

func runSimPlayer(engine contracts.SimulationEngine, state *contracts.SimPlayerState, rounds uint32, seed uint64, metrics *simCounters) {
	var out contracts.SimTickOutput
	now := int64(seed * 1_000)
	for round := uint32(0); round < rounds && state.Phase != contracts.SimPhaseQuit; round++ {
		if status := engine.SimPlayerTick(contracts.SimTickInput{NowUnixNano: now}, state, &out); status == contracts.SimStatusOK && out.PublishedTicket {
			metrics.queued.Add(1)
		}
		result := contracts.MatchResult{
			MatchID:       seed<<32 | uint64(round+1),
			PlayerA:       state.Ticket.PlayerID,
			PlayerB:       state.Ticket.PlayerID + 1,
			PredictedWinP: simPredictedWin(seed, round),
			AssignedAt:    now + 1,
		}
		_ = engine.SimPlayerTick(contracts.SimTickInput{NowUnixNano: now + 1, HasResult: true, Result: result}, state, &out)
		_ = engine.SimPlayerTick(contracts.SimTickInput{NowUnixNano: now + 2, OutcomeRoll: simRoll(seed, round, 3)}, state, &out)
		_ = engine.SimPlayerTick(contracts.SimTickInput{NowUnixNano: state.MatchEndsAt, OutcomeRoll: simRoll(seed, round, 5)}, state, &out)
		if status := engine.SimPlayerTick(contracts.SimTickInput{
			NowUnixNano:  state.MatchEndsAt + 1,
			OutcomeRoll:  simRoll(seed, round, 7),
			MutationRoll: simRoll(seed, round, 11),
			MutationDim:  uint8((seed + uint64(round)) % contracts.VectorDimensionCount),
			MutationSign: int8(1 - int(round%2)*2),
		}, state, &out); status == contracts.SimStatusOK {
			metrics.completed.Add(1)
			if out.MutatedDeck {
				metrics.mutated.Add(1)
			}
			if out.QuitSession {
				metrics.quit.Add(1)
			}
		}
		now = state.MatchEndsAt + contracts.SimDefaultTickRateNanos
	}
}

func simPredictedWin(seed uint64, round uint32) float32 {
	return 0.35 + 0.3*simRoll(seed, round, 13)
}

func simRoll(seed uint64, round uint32, salt uint64) float32 {
	x := seed + uint64(round)*0x9e3779b97f4a7c15 + salt
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return float32(x%10_000) / 10_000
}

func simSegmentDepths(states []contracts.SimPlayerState) [6]uint32 {
	var depths [6]uint32
	for i := range states {
		trophies := states[i].Ticket.Trophies
		switch {
		case trophies <= 1000:
			depths[0]++
		case trophies <= 3000:
			depths[1]++
		case trophies <= 6000:
			depths[2]++
		case trophies <= 9000:
			depths[3]++
		case trophies <= 12000:
			depths[4]++
		default:
			depths[5]++
		}
	}
	return depths
}

func minUint64(a uint64, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func (a *app) enqueue(join queueJoin) queueAck {
	if join.PlayerID == 0 || join.Trophies < 0 {
		return queueAck{Status: "rejected", Player: join.PlayerID, Message: "invalid player or trophies"}
	}
	vector := join.Vector
	if vector == [8]float32{} {
		vector[0] = 1
	}
	ticket := &contracts.Ticket{
		PlayerID:      join.PlayerID,
		EnqueuedAt:    time.Now().UnixNano(),
		DeckVector:    vector,
		Trophies:      join.Trophies,
		ChurnRisk:     clamp01(join.Churn),
		MonetizationP: clamp01(join.Spend),
		ConsecLosses:  join.Losses,
		ConsecWins:    join.Wins,
		PoolTag:       contracts.TicketPoolTag(join.Pool),
	}
	if ticket.PoolTag > contracts.PoolMonetize {
		ticket.PoolTag = contracts.PoolMainstream
	}
	result := a.ring.WriteTicket(ticket, ticket.EnqueuedAt)
	snapshot := a.ring.SnapshotShard(result.ShardID)
	a.telemetry.RecordQueueDepth(uint8(result.ShardID), uint32(snapshot.Depth), ticket.EnqueuedAt)
	if result.Status != contracts.RingWriteAccepted {
		return queueAck{Status: "rejected", Player: join.PlayerID, Shard: uint16(result.ShardID), Depth: snapshot.Depth, Message: fmt.Sprintf("ring status %d", result.Status)}
	}
	return queueAck{Status: "queued", Player: join.PlayerID, Shard: uint16(result.ShardID), Depth: snapshot.Depth}
}

func acceptWebSocket(w http.ResponseWriter, r *http.Request) (net.Conn, *bufio.ReadWriter, bool) {
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing websocket key", http.StatusBadRequest)
		return nil, nil, false
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return nil, nil, false
	}
	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return nil, nil, false
	}
	_, _ = io.WriteString(rw, "HTTP/1.1 101 Switching Protocols\r\n")
	_, _ = io.WriteString(rw, "Upgrade: websocket\r\n")
	_, _ = io.WriteString(rw, "Connection: Upgrade\r\n")
	_, _ = io.WriteString(rw, "Sec-WebSocket-Accept: "+telemetry.WebSocketAcceptKey(key)+"\r\n\r\n")
	_ = rw.Flush()
	return conn, rw, true
}

func readClientTextFrame(r *bufio.Reader) ([]byte, error) {
	header := [2]byte{}
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	opcode := header[0] & 0x0f
	if opcode == 0x8 {
		return nil, io.EOF
	}
	if opcode != 0x1 {
		return nil, fmt.Errorf("unsupported opcode %d", opcode)
	}
	masked := header[1]&0x80 != 0
	length := int(header[1] & 0x7f)
	if length == 126 {
		ext := [2]byte{}
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return nil, err
		}
		length = int(ext[0])<<8 | int(ext[1])
	}
	if length > 4096 {
		return nil, fmt.Errorf("frame too large")
	}
	mask := [4]byte{}
	if masked {
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return nil, err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return payload, nil
}

func writeAck(w *bufio.ReadWriter, ack queueAck) {
	payload, _ := json.Marshal(ack)
	_ = telemetry.WriteWebSocketText(w, payload)
	_ = w.Flush()
}

func envString(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
