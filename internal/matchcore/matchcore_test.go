package matchcore

import (
	"context"
	"math"
	"runtime"
	"sync/atomic"
	"testing"

	"matchpoint/contracts"
)

func TestConfigValidationAndRunCancellation(t *testing.T) {
	config := productionConfig()
	if status := validateConfig(config); status != contracts.MatchCoreStatusOK {
		t.Fatalf("valid config status=%d", status)
	}

	invalid := config
	invalid.TickIntervalNanos++
	if status := validateConfig(invalid); status != contracts.MatchCoreStatusInvalidConfig {
		t.Fatalf("B-MATCHCORE-6 tick interval status=%d", status)
	}
	invalid = config
	invalid.BaseTolerance = 0
	if status := validateConfig(invalid); status != contracts.MatchCoreStatusInvalidConfig {
		t.Fatalf("B-MATCHCORE-7 base tolerance status=%d", status)
	}
	invalid = config
	invalid.MaxTolerance = config.BaseTolerance - 1
	if status := validateConfig(invalid); status != contracts.MatchCoreStatusInvalidConfig {
		t.Fatalf("B-MATCHCORE-7 max tolerance status=%d", status)
	}
	invalid = config
	invalid.ToleranceK = math.Inf(1)
	if status := validateConfig(invalid); status != contracts.MatchCoreStatusInvalidConfig {
		t.Fatalf("B-MATCHCORE-7 non-finite k status=%d", status)
	}
	invalid = config
	invalid.HardBudgetNanos = config.TickIntervalNanos + 1
	if status := validateConfig(invalid); status != contracts.MatchCoreStatusInvalidConfig {
		t.Fatalf("B-MATCHCORE-8 hard budget status=%d", status)
	}
	invalid = config
	invalid.OverrunWarnThreshold = 0
	if status := validateConfig(invalid); status != contracts.MatchCoreStatusInvalidConfig {
		t.Fatalf("B-MATCHCORE-8 threshold status=%d", status)
	}

	core := mustCore(t, nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if status := core.Run(ctx); status != contracts.MatchCoreStatusCanceled {
		t.Fatalf("B-MATCHCORE-9 canceled run status=%d", status)
	}
}

func TestHandleTickOverrunSkipAndWarning(t *testing.T) {
	clock := &fakeClock{}
	clock.set(1+contracts.MatchTickHardBudgetNanos+1, 1_000+contracts.MatchTickHardBudgetNanos+1, 2_000+contracts.MatchTickHardBudgetNanos+1)
	metrics := &fakeMetrics{}
	logger := &fakeLogger{}
	core := mustCore(t, &fakeRing{}, &fakeStore{}, clock)
	core.metrics = metrics
	core.logger = logger

	first := core.HandleTick(context.Background(), contracts.MatchTickInput{TickID: 1, StartedUnixNano: 1})
	if first.Status != contracts.MatchCoreStatusOverrun || !core.state.skipNextTick.Load() {
		t.Fatalf("B-MATCHCORE-4 first overrun=%+v skip=%v", first, core.state.skipNextTick.Load())
	}
	skipped := core.HandleTick(context.Background(), contracts.MatchTickInput{TickID: 2, StartedUnixNano: 500})
	if skipped.Status != contracts.MatchCoreStatusSkipped || metrics.skipped.Load() != 1 {
		t.Fatalf("B-MATCHCORE-35 skipped=%+v metric=%d", skipped, metrics.skipped.Load())
	}
	second := core.HandleTick(context.Background(), contracts.MatchTickInput{TickID: 3, StartedUnixNano: 1_000})
	_ = core.HandleTick(context.Background(), contracts.MatchTickInput{TickID: 4, StartedUnixNano: 1_500})
	third := core.HandleTick(context.Background(), contracts.MatchTickInput{TickID: 5, StartedUnixNano: 2_000})
	if second.Metrics.ConsecutiveOverruns != 2 {
		t.Fatalf("B-MATCHCORE-4 consecutive=%d", second.Metrics.ConsecutiveOverruns)
	}
	if third.Status != contracts.MatchCoreStatusWarnOverrun || logger.warnings.Load() != 1 {
		t.Fatalf("B-MATCHCORE-5 third=%+v warnings=%d", third, logger.warnings.Load())
	}
}

func TestHandleTickNormalStateAndNoCatchup(t *testing.T) {
	clock := &fakeClock{}
	clock.set(100, 120, 140, 160)
	core := mustCore(t, &fakeRing{}, &fakeStore{}, clock)
	core.state.consecutiveOverrun.Store(2)
	result := core.HandleTick(context.Background(), contracts.MatchTickInput{TickID: 10, StartedUnixNano: 100})
	if result.Status != contracts.MatchCoreStatusNoWork || result.TickID != 10 || core.state.consecutiveOverrun.Load() != 0 || core.state.skipNextTick.Load() {
		t.Fatalf("B-MATCHCORE-1/B-MATCHCORE-2/B-MATCHCORE-3 result=%+v consecutive=%d skip=%v", result, core.state.consecutiveOverrun.Load(), core.state.skipNextTick.Load())
	}
}

func TestDrainRingsBoundedAndIgnoresEmptyClosed(t *testing.T) {
	ring := &fakeRing{
		shards: [][]*contracts.Ticket{
			{ticket(1, 1000, 100)},
			nil,
			{ticket(2, 1001, 110), ticket(3, 1002, 120)},
		},
		statuses: []contracts.RingReadStatus{contracts.RingReadOK, contracts.RingReadEmpty, contracts.RingReadOK},
	}
	drainer := newRingDrainer(3, 4)
	dst := make([]contracts.MatchDrainedTicket, 2)
	count, status := drainer.DrainRings(ring, 100, dst)
	if status != contracts.MatchCoreStatusOK || count != 2 || dst[0].ShardID != 0 || dst[1].ShardID != 2 {
		t.Fatalf("B-MATCHCORE-10/B-MATCHCORE-11/B-MATCHCORE-12 count=%d status=%d dst=%+v", count, status, dst)
	}
	if ring.positions[2] != 1 {
		t.Fatalf("B-MATCHCORE-12 unread tickets not preserved: pos=%d", ring.positions[2])
	}
}

func TestComputeToleranceEdges(t *testing.T) {
	calc := newToleranceCalculator()
	var out contracts.MatchToleranceResult
	input := contracts.MatchToleranceInput{
		EnqueuedAtUnixNano: 100,
		NowUnixNano:        100,
		BaseTolerance:      contracts.MatchDefaultBaseTolerance,
		K:                  contracts.MatchDefaultToleranceK,
		MaxTolerance:       contracts.MatchMaxTolerance,
	}
	if status := calc.ComputeTolerance(input, &out); status != contracts.MatchCoreStatusOK || out.ToleranceTrophies != contracts.MatchDefaultBaseTolerance || out.WaitNanos != 0 || out.Clamped {
		t.Fatalf("B-MATCHCORE-13 out=%+v status=%d", out, status)
	}
	input.NowUnixNano = 10_000_000_100
	if status := calc.ComputeTolerance(input, &out); status != contracts.MatchCoreStatusOK || out.ToleranceTrophies <= contracts.MatchDefaultBaseTolerance {
		t.Fatalf("B-MATCHCORE-14 out=%+v status=%d", out, status)
	}
	input.BaseTolerance = 1900
	input.K = 1
	input.NowUnixNano = 1_000_000_100
	if status := calc.ComputeTolerance(input, &out); status != contracts.MatchCoreStatusOK || out.ToleranceTrophies != contracts.MatchMaxTolerance || !out.Clamped {
		t.Fatalf("B-MATCHCORE-15 out=%+v status=%d", out, status)
	}
	input.K = 11
	input.NowUnixNano = 1_000_000_100
	if status := calc.ComputeTolerance(input, &out); status != contracts.MatchCoreStatusOK || out.Product <= contracts.MatchToleranceOverflowProduct || out.ToleranceTrophies != contracts.MatchMaxTolerance {
		t.Fatalf("B-MATCHCORE-16 out=%+v status=%d", out, status)
	}
	input.NowUnixNano = 99
	input.K = contracts.MatchDefaultToleranceK
	input.BaseTolerance = contracts.MatchDefaultBaseTolerance
	if status := calc.ComputeTolerance(input, &out); status != contracts.MatchCoreStatusOK || out.WaitNanos != 0 || out.ToleranceTrophies != contracts.MatchDefaultBaseTolerance {
		t.Fatalf("B-MATCHCORE-17 out=%+v status=%d", out, status)
	}
}

func TestQueueIngressCandidatePlanningAndRedisStatuses(t *testing.T) {
	store := &fakeStore{}
	tolerance := &atomic.Int64{}
	codec := fakeCodec{lastTolerance: tolerance}
	keyer := fakeKeyer{}
	ingress := newQueueIngress(4)
	drained := []contracts.MatchDrainedTicket{{Ticket: ticket(10, 2000, 1000)}}
	if status := ingress.EnqueueDrained(context.Background(), store, keyer, codec, drained); status != contracts.MatchCoreStatusOK || store.enqueues.Load() != 1 {
		t.Fatalf("B-MATCHCORE-18 status=%d enqueues=%d", status, store.enqueues.Load())
	}
	store.enqueueStatus = contracts.RedisStatusTimeout
	if status := ingress.EnqueueDrained(context.Background(), store, keyer, codec, drained); status != contracts.MatchCoreStatusRedisTimeout {
		t.Fatalf("B-MATCHCORE-19 timeout status=%d", status)
	}
	store.enqueueStatus = contracts.RedisStatusUnavailable
	if status := ingress.EnqueueDrained(context.Background(), store, keyer, codec, drained); status != contracts.MatchCoreStatusRedisUnavailable {
		t.Fatalf("B-MATCHCORE-19 unavailable status=%d", status)
	}

	planner := newCandidatePlanner(productionConfig())
	entries := []contracts.RedisQueueEntry{entryFor(t, 11, 2500, 2_000)}
	ranges := make([]contracts.RedisScoreRange, 1)
	results := [][]contracts.RedisCandidate{make([]contracts.RedisCandidate, contracts.MatchCandidateScratchLimit)}
	batch := contracts.RedisQueryBatch{Ranges: ranges, Results: results}
	if status := planner.BuildCandidateQueries(codec, entries, 3_000, &batch); status != contracts.MatchCoreStatusOK || batch.Count != 1 || ranges[0].Limit != int64(contracts.MatchCandidateScratchLimit) || tolerance.Load() == 0 {
		t.Fatalf("B-MATCHCORE-20/B-MATCHCORE-21 status=%d batch=%+v tolerance=%d", status, batch, tolerance.Load())
	}

	core := mustCore(t, &fakeRing{shards: [][]*contracts.Ticket{{ticket(12, 3000, 1000)}}, repeat: true}, &fakeStore{batchStatus: contracts.RedisStatusEmpty}, &fakeClock{values: []int64{1000, 1001}})
	result := core.HandleTick(context.Background(), contracts.MatchTickInput{TickID: 20, StartedUnixNano: 1000})
	if result.Status != contracts.MatchCoreStatusNoWork || result.Metrics.EmptyQueries == 0 {
		t.Fatalf("B-MATCHCORE-22 result=%+v", result)
	}
}

func TestScoringSelectionAndPoolTagImmutability(t *testing.T) {
	scorer := newBaselineScorer()
	anchor := *ticket(20, 5000, 100)
	candidate := entryFor(t, 21, 5020, 90)
	var score contracts.MatchCandidateScore
	status := scorer.ScoreCandidate(contracts.MatchCandidateContext{
		Anchor:            anchor,
		Candidate:         candidate,
		AnchorPool:        contracts.RedisPoolSegment2,
		CandidatePool:     contracts.RedisPoolSegment2,
		ToleranceTrophies: 50,
	}, &score)
	if status != contracts.MatchCoreStatusOK || score.Decision != contracts.MatchCandidateReplaceBest {
		t.Fatalf("B-MATCHCORE-23 score=%+v status=%d", score, status)
	}
	vector := noOpVectorScorer{}.CosineSimilarity([8]float32{1}, [8]float32{1})
	if vector != 1 {
		t.Fatalf("B-MATCHCORE-23 vector=%f", vector)
	}
	self := candidate
	self.Ticket.PlayerID = anchor.PlayerID
	status = scorer.ScoreCandidate(contracts.MatchCandidateContext{Anchor: anchor, Candidate: self, ToleranceTrophies: 50}, &score)
	if status != contracts.MatchCoreStatusInvalidCandidate || score.Decision != contracts.MatchCandidateReject {
		t.Fatalf("B-MATCHCORE-24 self status=%d score=%+v", status, score)
	}
	far := candidate
	far.Ticket.Trophies = 6000
	status = scorer.ScoreCandidate(contracts.MatchCandidateContext{Anchor: anchor, Candidate: far, AnchorPool: contracts.RedisPoolSegment2, CandidatePool: contracts.RedisPoolSegment2, ToleranceTrophies: 50}, &score)
	if status != contracts.MatchCoreStatusInvalidCandidate {
		t.Fatalf("B-MATCHCORE-24 trophy status=%d", status)
	}

	lower := pair(20, 22, 5000, 5050, 200, 90)
	higher := pair(20, 23, 5000, 5010, 300, 80)
	best, decision := chooseCandidate(lower, higher, true)
	if decision != contracts.MatchCandidateReplaceBest || best.PlayerB.Ticket.PlayerID != 23 {
		t.Fatalf("B-MATCHCORE-25 best=%+v decision=%d", best, decision)
	}
	older := pair(20, 24, 5000, 5010, 300, 70)
	best, decision = chooseCandidate(higher, older, true)
	if decision != contracts.MatchCandidateReplaceBest || best.PlayerB.Ticket.PlayerID != 24 {
		t.Fatalf("B-MATCHCORE-26 older tie best=%+v decision=%d", best, decision)
	}
	lowerID := pair(20, 21, 5000, 5010, 300, 80)
	best, decision = chooseCandidate(higher, lowerID, true)
	if decision != contracts.MatchCandidateReplaceBest || best.PlayerB.Ticket.PlayerID != 21 {
		t.Fatalf("B-MATCHCORE-26 lower id tie best=%+v decision=%d", best, decision)
	}
	before := anchor.PoolTag
	_ = scorer.ScoreCandidate(contracts.MatchCandidateContext{Anchor: anchor, Candidate: candidate, AnchorPool: contracts.RedisPoolSegment2, CandidatePool: contracts.RedisPoolSegment2, ToleranceTrophies: 50}, &score)
	if anchor.PoolTag != before {
		t.Fatalf("B-MATCHCORE-27 pool tag mutated")
	}
}

func TestAssignPairStatusMappingAndResult(t *testing.T) {
	store := &fakeStore{}
	clock := &fakeClock{values: []int64{9000}}
	assign := newAssigner(newIDGenerator(100), clock)
	out := contracts.MatchResult{}
	self := pair(30, 30, 1000, 1000, 1, 1)
	if status := assign.AssignPair(context.Background(), store, self, &out); status != contracts.MatchCoreStatusInvalidCandidate || store.assigns.Load() != 0 {
		t.Fatalf("B-MATCHCORE-28 status=%d assigns=%d", status, store.assigns.Load())
	}
	good := pair(30, 31, 1000, 1005, 1, 2)
	if status := assign.AssignPair(context.Background(), store, good, &out); status != contracts.MatchCoreStatusOK || out.MatchID == 0 || out.PlayerA != 30 || out.PlayerB != 31 || out.PredictedWinP != good.Score.PredictedWinP || out.AssignedAt != 9000 {
		t.Fatalf("B-MATCHCORE-29/B-MATCHCORE-31 status=%d out=%+v", status, out)
	}
	store.assignStatus = contracts.RedisStatusDualBooking
	if status := assign.AssignPair(context.Background(), store, good, &out); status != contracts.MatchCoreStatusDualBooking || out.MatchID != 0 {
		t.Fatalf("B-MATCHCORE-30 status=%d out=%+v", status, out)
	}
	for _, redisStatus := range []contracts.RedisQueueStatus{contracts.RedisStatusTimeout, contracts.RedisStatusUnavailable, contracts.RedisStatusCanceled, contracts.RedisStatusNoScript} {
		store.assignStatus = redisStatus
		if status := assign.AssignPair(context.Background(), store, good, &out); status == contracts.MatchCoreStatusOK {
			t.Fatalf("B-MATCHCORE-32 redis status=%d mapped OK", redisStatus)
		}
	}
}

func TestHandleTickMetricsRedisAndSnapshot(t *testing.T) {
	clock := &fakeClock{values: []int64{10, 20, 30, 40}}
	store := &fakeStore{
		candidates: [][]contracts.RedisCandidate{
			{candidate(41, 1005, 5, contracts.RedisPoolSegment1)},
		},
	}
	metrics := &fakeMetrics{}
	core := mustCore(t, &fakeRing{shards: [][]*contracts.Ticket{{ticket(40, 1000, 1)}}}, store, clock)
	core.metrics = metrics
	result := core.HandleTick(context.Background(), contracts.MatchTickInput{TickID: 33, StartedUnixNano: 10})
	if result.Status != contracts.MatchCoreStatusOK || result.Metrics.DrainedTickets != 1 || result.Metrics.CandidateQueries != 1 || result.Metrics.MatchesMade != 1 || result.Metrics.DurationNanos != 10 {
		t.Fatalf("B-MATCHCORE-33/B-MATCHCORE-37 result=%+v", result)
	}
	if metrics.redisStatuses.Load() == 0 {
		t.Fatalf("B-MATCHCORE-36 redis statuses not recorded")
	}
	var state contracts.MatchTickState
	if status := core.SnapshotTickState(&state); status != contracts.MatchCoreStatusOK || state.LastStartedUnixNano != 10 || state.LastFinishedUnixNano != 20 {
		t.Fatalf("B-MATCHCORE-38 status=%d state=%+v", status, state)
	}

	overrunClock := &fakeClock{values: []int64{100 + contracts.MatchTickHardBudgetNanos + 1}}
	overrunMetrics := &fakeMetrics{}
	overrunCore := mustCore(t, &fakeRing{}, &fakeStore{}, overrunClock)
	overrunCore.metrics = overrunMetrics
	overrunResult := overrunCore.HandleTick(context.Background(), contracts.MatchTickInput{TickID: 34, StartedUnixNano: 100})
	if overrunResult.Status != contracts.MatchCoreStatusOverrun || overrunMetrics.overruns.Load() != 1 || overrunMetrics.ticks.Load() != 1 {
		t.Fatalf("B-MATCHCORE-34 result=%+v metrics=%+v", overrunResult, overrunMetrics)
	}

	timeoutCore := mustCore(t, &fakeRing{shards: [][]*contracts.Ticket{{ticket(50, 1000, 1)}}}, &fakeStore{enqueueStatus: contracts.RedisStatusTimeout}, &fakeClock{values: []int64{1, 2}})
	timeoutResult := timeoutCore.HandleTick(context.Background(), contracts.MatchTickInput{TickID: 36, StartedUnixNano: 1})
	if timeoutResult.Status != contracts.MatchCoreStatusRedisTimeout || timeoutResult.Metrics.RedisTimeouts == 0 {
		t.Fatalf("B-MATCHCORE-36 timeout metrics=%+v", timeoutResult.Metrics)
	}

	assignMetrics := &fakeMetrics{}
	assignStore := &fakeStore{
		assignStatus: contracts.RedisStatusNoScript,
		candidates: [][]contracts.RedisCandidate{
			{candidate(61, 1005, 1, contracts.RedisPoolSegment0)},
		},
	}
	assignCore := mustCore(t, &fakeRing{shards: [][]*contracts.Ticket{{ticket(60, 1000, 1)}}}, assignStore, &fakeClock{values: []int64{1, 2}})
	assignCore.metrics = assignMetrics
	assignResult := assignCore.HandleTick(context.Background(), contracts.MatchTickInput{TickID: 37, StartedUnixNano: 1})
	if assignResult.Status != contracts.MatchCoreStatusRedisUnavailable || contracts.RedisQueueStatus(assignMetrics.lastRedisStatus.Load()) != contracts.RedisStatusNoScript {
		t.Fatalf("B-MATCHCORE-36 assign result=%+v lastRedis=%d", assignResult, assignMetrics.lastRedisStatus.Load())
	}
}

func BenchmarkNowUnixNanoGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkNowUnixNano)
}

func BenchmarkNowUnixNanoGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkNowUnixNano)
}

func BenchmarkComputeToleranceGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkComputeTolerance)
}

func BenchmarkComputeToleranceGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkComputeTolerance)
}

func BenchmarkDrainRingsGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkDrainRings)
}

func BenchmarkDrainRingsGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkDrainRings)
}

func BenchmarkEnqueueDrainedGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkEnqueueDrained)
}

func BenchmarkEnqueueDrainedGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkEnqueueDrained)
}

func BenchmarkBuildCandidateQueriesGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkBuildCandidateQueries)
}

func BenchmarkBuildCandidateQueriesGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkBuildCandidateQueries)
}

func BenchmarkScoreCandidateGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkScoreCandidate)
}

func BenchmarkScoreCandidateGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkScoreCandidate)
}

func BenchmarkCandidateTieBreakingGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkCandidateTieBreaking)
}

func BenchmarkCandidateTieBreakingGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkCandidateTieBreaking)
}

func BenchmarkAssignPairGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkAssignPair)
}

func BenchmarkAssignPairGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkAssignPair)
}

func BenchmarkMetricsSinkGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkMetricsSink)
}

func BenchmarkMetricsSinkGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkMetricsSink)
}

func BenchmarkNextMatchIDGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkNextMatchID)
}

func BenchmarkNextMatchIDGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkNextMatchID)
}

func BenchmarkSnapshotTickStateGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkSnapshotTickState)
}

func BenchmarkSnapshotTickStateGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkSnapshotTickState)
}

func BenchmarkMatchLoop(b *testing.B) {
	benchmarkMatchLoop(b)
}

func BenchmarkMatchLoopGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkMatchLoop)
}

func BenchmarkMatchLoopGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkMatchLoop)
}

func benchmarkWithProcs(b *testing.B, procs int, fn func(*testing.B)) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	fn(b)
}

func benchmarkNowUnixNano(b *testing.B) {
	clock := &fakeClock{values: []int64{1}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = clock.NowUnixNano()
	}
}

func benchmarkComputeTolerance(b *testing.B) {
	calc := newToleranceCalculator()
	input := contracts.MatchToleranceInput{EnqueuedAtUnixNano: 1, NowUnixNano: 10_000_000_001, BaseTolerance: contracts.MatchDefaultBaseTolerance, K: contracts.MatchDefaultToleranceK, MaxTolerance: contracts.MatchMaxTolerance}
	var out contracts.MatchToleranceResult
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = calc.ComputeTolerance(input, &out)
	}
}

func benchmarkDrainRings(b *testing.B) {
	ring := &fakeRing{shards: [][]*contracts.Ticket{{ticket(1, 1000, 1)}}, repeat: true}
	drainer := newRingDrainer(1, 1)
	dst := make([]contracts.MatchDrainedTicket, 1)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = drainer.DrainRings(ring, 1, dst)
	}
}

func benchmarkEnqueueDrained(b *testing.B) {
	ingress := newQueueIngress(1)
	store := &fakeStore{}
	drained := []contracts.MatchDrainedTicket{{Ticket: ticket(1, 1000, 1)}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ingress.EnqueueDrained(context.Background(), store, fakeKeyer{}, fakeCodec{}, drained)
	}
}

func benchmarkBuildCandidateQueries(b *testing.B) {
	planner := newCandidatePlanner(productionConfig())
	entries := []contracts.RedisQueueEntry{entryFor(b, 1, 1000, 1)}
	ranges := make([]contracts.RedisScoreRange, 1)
	results := [][]contracts.RedisCandidate{make([]contracts.RedisCandidate, 1)}
	batch := contracts.RedisQueryBatch{Ranges: ranges, Results: results}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = planner.BuildCandidateQueries(fakeCodec{}, entries, 10_000_000_001, &batch)
	}
}

func benchmarkScoreCandidate(b *testing.B) {
	scorer := newBaselineScorer()
	input := contracts.MatchCandidateContext{Anchor: *ticket(1, 1000, 1), Candidate: entryFor(b, 2, 1001, 2), AnchorPool: contracts.RedisPoolSegment0, CandidatePool: contracts.RedisPoolSegment0, ToleranceTrophies: 50}
	var out contracts.MatchCandidateScore
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = scorer.ScoreCandidate(input, &out)
	}
}

func benchmarkCandidateTieBreaking(b *testing.B) {
	current := pair(1, 2, 1000, 1005, 10, 1)
	candidate := pair(1, 3, 1000, 1003, 10, 0)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = chooseCandidate(current, candidate, true)
	}
}

func benchmarkAssignPair(b *testing.B) {
	assign := newAssigner(newIDGenerator(0), &fakeClock{values: []int64{1}})
	store := &fakeStore{}
	pair := pair(1, 2, 1000, 1001, 1, 2)
	var out contracts.MatchResult
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = assign.AssignPair(context.Background(), store, pair, &out)
	}
}

func benchmarkMetricsSink(b *testing.B) {
	metrics := newMetricsSink()
	tick := contracts.MatchTickMetrics{TickID: 1}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		metrics.RecordTick(tick)
		metrics.RecordRedisStatus(contracts.RedisStatusOK, 1)
	}
}

func benchmarkNextMatchID(b *testing.B) {
	ids := newIDGenerator(0)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ids.NextMatchID()
	}
}

func benchmarkSnapshotTickState(b *testing.B) {
	core := mustCoreB(b)
	var out contracts.MatchTickState
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = core.SnapshotTickState(&out)
	}
}

func benchmarkMatchLoop(b *testing.B) {
	clock := &fakeClock{values: []int64{1, 2}}
	store := &fakeStore{candidates: [][]contracts.RedisCandidate{{candidate(2, 1001, 2, contracts.RedisPoolSegment0)}}}
	core := mustCoreBWith(&fakeRing{shards: [][]*contracts.Ticket{{ticket(1, 1000, 1)}}, repeat: true}, store, clock)
	input := contracts.MatchTickInput{TickID: 1, StartedUnixNano: 1}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = core.HandleTick(context.Background(), input)
	}
}

type fakeClock struct {
	values []int64
	index  atomic.Uint64
}

func (c *fakeClock) set(values ...int64) {
	c.values = values
	c.index.Store(0)
}

func (c *fakeClock) NowUnixNano() int64 {
	if c == nil || len(c.values) == 0 {
		return 0
	}
	index := c.index.Add(1) - 1
	if int(index) >= len(c.values) {
		return c.values[len(c.values)-1]
	}
	return c.values[index]
}

type fakeRing struct {
	shards    [][]*contracts.Ticket
	statuses  []contracts.RingReadStatus
	positions [8]int
	repeat    bool
}

func (r *fakeRing) WriteTicket(_ *contracts.Ticket, _ int64) contracts.RingWriteResult {
	return contracts.RingWriteResult{Status: contracts.RingWriteAccepted}
}

func (r *fakeRing) ReadTicket(_ contracts.RingShardID) contracts.RingReadResult {
	return contracts.RingReadResult{Status: contracts.RingReadEmpty}
}

func (r *fakeRing) DrainShard(shardID contracts.RingShardID, dst []*contracts.Ticket) contracts.RingDrainResult {
	index := int(shardID)
	if r == nil || index >= len(r.shards) {
		return contracts.RingDrainResult{Status: contracts.RingReadClosed, ShardID: shardID}
	}
	status := contracts.RingReadOK
	if index < len(r.statuses) && r.statuses[index] != 0 {
		status = r.statuses[index]
	}
	if status != contracts.RingReadOK {
		return contracts.RingDrainResult{Status: status, ShardID: shardID}
	}
	if len(r.shards[index]) == 0 {
		return contracts.RingDrainResult{Status: contracts.RingReadEmpty, ShardID: shardID}
	}
	count := uint32(0)
	for count < uint32(len(dst)) {
		pos := r.positions[index]
		if pos >= len(r.shards[index]) {
			if !r.repeat {
				break
			}
			pos = 0
		}
		dst[count] = r.shards[index][pos]
		count++
		pos++
		r.positions[index] = pos
		if r.repeat {
			r.positions[index] = 0
			break
		}
	}
	if count == 0 {
		return contracts.RingDrainResult{Status: contracts.RingReadEmpty, ShardID: shardID}
	}
	return contracts.RingDrainResult{Status: contracts.RingReadOK, ShardID: shardID, Count: count}
}

func (r *fakeRing) SnapshotShard(shardID contracts.RingShardID) contracts.RingSnapshot {
	return contracts.RingSnapshot{ShardID: shardID}
}

func (r *fakeRing) CloseShard(_ contracts.RingShardID) contracts.RingState {
	return contracts.RingStateClosed
}

func (r *fakeRing) Close() contracts.RingState {
	return contracts.RingStateClosed
}

type fakeKeyer struct{}

func (fakeKeyer) SegmentForTrophies(trophies int32) contracts.RedisQueuePool {
	switch {
	case trophies <= 1000:
		return contracts.RedisPoolSegment0
	case trophies <= 3000:
		return contracts.RedisPoolSegment1
	case trophies <= 6000:
		return contracts.RedisPoolSegment2
	case trophies <= 10000:
		return contracts.RedisPoolSegment3
	default:
		return contracts.RedisPoolSegment4
	}
}

func (fakeKeyer) KeyForPool(pool contracts.RedisQueuePool) (string, contracts.RedisQueueStatus) {
	if pool > contracts.RedisPoolMonetize {
		return "", contracts.RedisStatusInvalidSegment
	}
	return "key", contracts.RedisStatusOK
}

func (fakeKeyer) SegmentRange(pool contracts.RedisQueuePool) (contracts.RedisSegmentRange, contracts.RedisQueueStatus) {
	return contracts.RedisSegmentRange{Pool: pool}, contracts.RedisStatusOK
}

type fakeCodec struct {
	lastTolerance *atomic.Int64
}

func (fakeCodec) EncodeMember(playerID uint64, out *contracts.RedisMember) contracts.RedisQueueStatus {
	if playerID == 0 || out == nil {
		return contracts.RedisStatusInvalidScore
	}
	*out = contracts.RedisMember{PlayerID: playerID, Len: 1}
	out.Bytes[0] = '1'
	return contracts.RedisStatusOK
}

func (fakeCodec) EncodeScore(trophies int32, enqueuedAtUnixNano int64, out *contracts.RedisScore) contracts.RedisQueueStatus {
	if out == nil || trophies < 0 || enqueuedAtUnixNano < 0 {
		return contracts.RedisStatusInvalidScore
	}
	micros := (enqueuedAtUnixNano / 1000) % contracts.RedisScoreTrophyScale
	*out = contracts.RedisScore{Value: float64(int64(trophies)*contracts.RedisScoreTrophyScale + micros), Trophies: trophies, EnqueuedAtMicros: micros}
	return contracts.RedisStatusOK
}

func (c fakeCodec) ScoreRange(trophies int32, enqueuedAtUnixNano int64, toleranceTrophies int32, pool contracts.RedisQueuePool, out *contracts.RedisScoreRange) contracts.RedisQueueStatus {
	if c.lastTolerance != nil {
		c.lastTolerance.Store(int64(toleranceTrophies))
	}
	var score contracts.RedisScore
	if status := c.EncodeScore(trophies, enqueuedAtUnixNano, &score); status != contracts.RedisStatusOK {
		return status
	}
	delta := float64(int64(toleranceTrophies) * contracts.RedisScoreTrophyScale)
	*out = contracts.RedisScoreRange{Pool: pool, Min: score.Value - delta, Max: score.Value + delta, Limit: int64(contracts.MatchCandidateScratchLimit)}
	return contracts.RedisStatusOK
}

type fakeStore struct {
	enqueueStatus contracts.RedisQueueStatus
	batchStatus   contracts.RedisQueueStatus
	assignStatus  contracts.RedisQueueStatus
	candidates    [][]contracts.RedisCandidate
	enqueues      atomic.Uint64
	assigns       atomic.Uint64
}

func (s *fakeStore) Enqueue(_ context.Context, _ *contracts.RedisQueueEntry) contracts.RedisOperationResult {
	s.enqueues.Add(1)
	status := s.enqueueStatus
	if status == 0 {
		status = contracts.RedisStatusOK
	}
	return contracts.RedisOperationResult{Status: status, Count: 1, ElapsedNanos: 1}
}

func (s *fakeStore) Remove(_ context.Context, pool contracts.RedisQueuePool, _ contracts.RedisMember) contracts.RedisOperationResult {
	return contracts.RedisOperationResult{Status: contracts.RedisStatusOK, Pool: pool, Count: 1}
}

func (s *fakeStore) FetchCandidates(_ context.Context, query contracts.RedisScoreRange, dst []contracts.RedisCandidate) contracts.RedisOperationResult {
	if len(dst) == 0 {
		return contracts.RedisOperationResult{Status: contracts.RedisStatusEmpty, Pool: query.Pool}
	}
	dst[0] = candidate(2, 1000, 1, query.Pool)
	return contracts.RedisOperationResult{Status: contracts.RedisStatusOK, Pool: query.Pool, Count: 1}
}

func (s *fakeStore) FetchCandidateBatch(_ context.Context, batch *contracts.RedisQueryBatch) contracts.RedisOperationResult {
	status := s.batchStatus
	if status == 0 {
		status = contracts.RedisStatusOK
	}
	if status == contracts.RedisStatusEmpty {
		return contracts.RedisOperationResult{Status: status}
	}
	var count uint16
	for i := 0; i < int(batch.Count) && i < len(s.candidates); i++ {
		for j := 0; j < len(batch.Results[i]) && j < len(s.candidates[i]); j++ {
			batch.Results[i][j] = s.candidates[i][j]
			count++
		}
	}
	if count == 0 && status == contracts.RedisStatusOK {
		status = contracts.RedisStatusEmpty
	}
	return contracts.RedisOperationResult{Status: status, Count: count, ElapsedNanos: 1}
}

func (s *fakeStore) MovePool(_ context.Context, move contracts.RedisMoveRequest) contracts.RedisOperationResult {
	return contracts.RedisOperationResult{Status: contracts.RedisStatusOK, Pool: move.To, Count: 1}
}

func (s *fakeStore) AssignMatch(_ context.Context, req contracts.RedisAssignRequest) contracts.RedisAssignResult {
	s.assigns.Add(1)
	status := s.assignStatus
	if status == 0 {
		status = contracts.RedisStatusOK
	}
	return contracts.RedisAssignResult{Status: status, MatchID: req.MatchID, PlayerA: req.PlayerA, PlayerB: req.PlayerB, ElapsedNanos: 1}
}

type fakeMetrics struct {
	ticks           atomic.Uint64
	overruns        atomic.Uint64
	skipped         atomic.Uint64
	dualBookings    atomic.Uint64
	emptyQueries    atomic.Uint64
	redisStatuses   atomic.Uint64
	lastRedisStatus atomic.Uint32
}

func (m *fakeMetrics) RecordTick(_ contracts.MatchTickMetrics) {
	m.ticks.Add(1)
}

func (m *fakeMetrics) RecordOverrun(_ uint64, _ int64, _ uint32) {
	m.overruns.Add(1)
}

func (m *fakeMetrics) RecordSkippedTick(_ uint64) {
	m.skipped.Add(1)
}

func (m *fakeMetrics) RecordDualBooking(_ uint64) {
	m.dualBookings.Add(1)
}

func (m *fakeMetrics) RecordEmptyQuery(_ contracts.RedisQueuePool) {
	m.emptyQueries.Add(1)
}

func (m *fakeMetrics) RecordRedisStatus(status contracts.RedisQueueStatus, _ int64) {
	m.redisStatuses.Add(1)
	m.lastRedisStatus.Store(uint32(status))
}

type fakeLogger struct {
	warnings atomic.Uint64
}

func (l *fakeLogger) WarnConsecutiveOverruns(_ uint64, _ uint32, _ int64) {
	l.warnings.Add(1)
}

func mustCore(t *testing.T, ring contracts.TicketRingBuffer, store contracts.RedisQueueStore, clock contracts.MatchClock) *matchCore {
	t.Helper()
	return mustCoreWithT(t, ring, store, clock)
}

func mustCoreB(b *testing.B) *matchCore {
	b.Helper()
	return mustCoreBWith(&fakeRing{}, &fakeStore{}, &fakeClock{values: []int64{1}})
}

func mustCoreBWith(ring contracts.TicketRingBuffer, store contracts.RedisQueueStore, clock contracts.MatchClock) *matchCore {
	core, status := newMatchCore(productionConfig(), 1, ring, store, fakeKeyer{}, fakeCodec{}, clock, nil, nil)
	if status != contracts.MatchCoreStatusOK {
		panic(status)
	}
	return core
}

func mustCoreWithT(t *testing.T, ring contracts.TicketRingBuffer, store contracts.RedisQueueStore, clock contracts.MatchClock) *matchCore {
	if ring == nil {
		ring = &fakeRing{}
	}
	if store == nil {
		store = &fakeStore{}
	}
	if clock == nil {
		clock = &fakeClock{values: []int64{1}}
	}
	core, status := newMatchCore(productionConfig(), 3, ring, store, fakeKeyer{}, fakeCodec{}, clock, nil, nil)
	if status != contracts.MatchCoreStatusOK {
		t.Fatalf("newMatchCore status=%d", status)
	}
	return core
}

func ticket(playerID uint64, trophies int32, enqueuedAt int64) *contracts.Ticket {
	return &contracts.Ticket{
		PlayerID:   playerID,
		EnqueuedAt: enqueuedAt,
		Trophies:   trophies,
		DeckVector: [8]float32{1},
		ChurnRisk:  0.1,
		PoolTag:    contracts.PoolMainstream,
	}
}

func entryFor(tb testing.TB, playerID uint64, trophies int32, enqueuedAt int64) contracts.RedisQueueEntry {
	tb.Helper()
	var entry contracts.RedisQueueEntry
	if status := buildQueueEntry(ticket(playerID, trophies, enqueuedAt), fakeKeyer{}, fakeCodec{}, &entry); status != contracts.RedisStatusOK {
		tb.Fatalf("build entry status=%d", status)
	}
	return entry
}

func candidate(playerID uint64, trophies int32, enqueuedAtMicros int64, pool contracts.RedisQueuePool) contracts.RedisCandidate {
	return contracts.RedisCandidate{
		Member: contracts.RedisMember{PlayerID: playerID, Len: 1},
		Score:  contracts.RedisScore{Value: float64(int64(trophies)*contracts.RedisScoreTrophyScale + enqueuedAtMicros), Trophies: trophies, EnqueuedAtMicros: enqueuedAtMicros},
		Pool:   pool,
	}
}

func pair(a uint64, b uint64, trophiesA int32, trophiesB int32, fitness float32, enqueuedB int64) contracts.MatchPair {
	playerA := mustEntry(a, trophiesA, 1)
	playerB := mustEntry(b, trophiesB, enqueuedB)
	delta := trophiesA - trophiesB
	if delta < 0 {
		delta = -delta
	}
	return contracts.MatchPair{
		PlayerA: playerA,
		PlayerB: playerB,
		SourceA: playerA.Pool,
		SourceB: playerB.Pool,
		Score: contracts.MatchCandidateScore{
			Fitness:       fitness,
			TrophyDelta:   delta,
			PredictedWinP: 0.5,
			Decision:      contracts.MatchCandidateReplaceBest,
		},
	}
}

func mustEntry(playerID uint64, trophies int32, enqueuedAt int64) contracts.RedisQueueEntry {
	var entry contracts.RedisQueueEntry
	if status := buildQueueEntry(ticket(playerID, trophies, enqueuedAt), fakeKeyer{}, fakeCodec{}, &entry); status != contracts.RedisStatusOK {
		panic("entry build failed")
	}
	return entry
}
