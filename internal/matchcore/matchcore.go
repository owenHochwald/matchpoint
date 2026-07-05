// Package matchcore implements the MatchPoint 200ms matchmaking tick loop.
package matchcore

import (
	"context"
	"math"
	"sync/atomic"
	"time"

	"matchpoint/contracts"
)

const (
	MatchTickIntervalNanos        = contracts.MatchTickIntervalNanos
	MatchTickHardBudgetNanos      = contracts.MatchTickHardBudgetNanos
	MatchRingDrainBudgetNanos     = contracts.MatchRingDrainBudgetNanos
	MatchRedisQueryBudgetNanos    = contracts.MatchRedisQueryBudgetNanos
	MatchFitnessBudgetNanos       = contracts.MatchFitnessBudgetNanos
	MatchAssignBudgetNanos        = contracts.MatchAssignBudgetNanos
	MatchTelemetryBudgetNanos     = contracts.MatchTelemetryBudgetNanos
	MatchOverrunWarnThreshold     = contracts.MatchOverrunWarnThreshold
	MatchDefaultBaseTolerance     = contracts.MatchDefaultBaseTolerance
	MatchMaxTolerance             = contracts.MatchMaxTolerance
	MatchDefaultToleranceK        = contracts.MatchDefaultToleranceK
	MatchToleranceOverflowProduct = contracts.MatchToleranceOverflowProduct
	MatchCandidateScratchLimit    = contracts.MatchCandidateScratchLimit
)

const (
	MatchCoreStatusOK               = contracts.MatchCoreStatusOK
	MatchCoreStatusNoWork           = contracts.MatchCoreStatusNoWork
	MatchCoreStatusSkipped          = contracts.MatchCoreStatusSkipped
	MatchCoreStatusOverrun          = contracts.MatchCoreStatusOverrun
	MatchCoreStatusWarnOverrun      = contracts.MatchCoreStatusWarnOverrun
	MatchCoreStatusRedisTimeout     = contracts.MatchCoreStatusRedisTimeout
	MatchCoreStatusRedisUnavailable = contracts.MatchCoreStatusRedisUnavailable
	MatchCoreStatusDualBooking      = contracts.MatchCoreStatusDualBooking
	MatchCoreStatusInvalidConfig    = contracts.MatchCoreStatusInvalidConfig
	MatchCoreStatusInvalidCandidate = contracts.MatchCoreStatusInvalidCandidate
	MatchCoreStatusCanceled         = contracts.MatchCoreStatusCanceled
)

const (
	MatchCandidateReject       = contracts.MatchCandidateReject
	MatchCandidateKeepExisting = contracts.MatchCandidateKeepExisting
	MatchCandidateReplaceBest  = contracts.MatchCandidateReplaceBest
)

type MatchCoreStatus = contracts.MatchCoreStatus
type MatchCandidateDecision = contracts.MatchCandidateDecision
type MatchCoreConfig = contracts.MatchCoreConfig
type MatchTickInput = contracts.MatchTickInput
type MatchTickState = contracts.MatchTickState
type MatchToleranceInput = contracts.MatchToleranceInput
type MatchToleranceResult = contracts.MatchToleranceResult
type MatchDrainedTicket = contracts.MatchDrainedTicket
type MatchCandidateContext = contracts.MatchCandidateContext
type MatchCandidateScore = contracts.MatchCandidateScore
type MatchPair = contracts.MatchPair
type MatchResult = contracts.MatchResult
type MatchTickMetrics = contracts.MatchTickMetrics
type MatchTickResult = contracts.MatchTickResult
type MatchClock = contracts.MatchClock
type MatchToleranceCalculator = contracts.MatchToleranceCalculator
type MatchRingDrainer = contracts.MatchRingDrainer
type MatchQueueIngress = contracts.MatchQueueIngress
type MatchCandidatePlanner = contracts.MatchCandidatePlanner
type MatchFitnessScorer = contracts.MatchFitnessScorer
type MatchVectorScorer = contracts.MatchVectorScorer
type MatchAssigner = contracts.MatchAssigner
type MatchMetricsSink = contracts.MatchMetricsSink
type MatchOverrunLogger = contracts.MatchOverrunLogger
type MatchIDGenerator = contracts.MatchIDGenerator
type MatchCoreLoop = contracts.MatchCoreLoop

type systemClock struct{}

type toleranceCalculator struct{}

type ringDrainer struct {
	shards  uint16
	tickets []*contracts.Ticket
}

type queueIngress struct {
	entries []contracts.RedisQueueEntry
}

type candidatePlanner struct {
	config    contracts.MatchCoreConfig
	tolerance toleranceCalculator
}

type baselineScorer struct{}

type noOpVectorScorer struct{}

type assigner struct {
	ids   contracts.MatchIDGenerator
	clock contracts.MatchClock
}

type metricsSink struct {
	ticks         atomic.Uint64
	overruns      atomic.Uint64
	skipped       atomic.Uint64
	dualBookings  atomic.Uint64
	emptyQueries  atomic.Uint64
	redisStatuses atomic.Uint64
}

type overrunLogger struct {
	warnings atomic.Uint64
}

type idGenerator struct {
	next atomic.Uint64
}

type matchCore struct {
	config  contracts.MatchCoreConfig
	clock   contracts.MatchClock
	ring    contracts.TicketRingBuffer
	store   contracts.RedisQueueStore
	keyer   contracts.RedisQueueKeyer
	codec   contracts.RedisScoreCodec
	drainer *ringDrainer
	ingress *queueIngress
	planner *candidatePlanner
	scorer  contracts.MatchFitnessScorer
	assign  *assigner
	metrics contracts.MatchMetricsSink
	logger  contracts.MatchOverrunLogger

	state atomicTickState

	drained    []contracts.MatchDrainedTicket
	entries    []contracts.RedisQueueEntry
	ranges     []contracts.RedisScoreRange
	candidates [][]contracts.RedisCandidate
	batch      contracts.RedisQueryBatch
	result     contracts.MatchResult
}

type atomicTickState struct {
	lastStarted        atomic.Int64
	lastFinished       atomic.Int64
	consecutiveOverrun atomic.Uint32
	totalOverruns      atomic.Uint64
	skippedTicks       atomic.Uint64
	skipNextTick       atomic.Bool
}

func productionConfig() contracts.MatchCoreConfig {
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

func newSystemClock() systemClock {
	return systemClock{}
}

func newToleranceCalculator() toleranceCalculator {
	return toleranceCalculator{}
}

func newRingDrainer(shards uint16, batchSize uint32) *ringDrainer {
	return &ringDrainer{
		shards:  shards,
		tickets: make([]*contracts.Ticket, batchSize),
	}
}

func newQueueIngress(batchSize uint32) *queueIngress {
	return &queueIngress{entries: make([]contracts.RedisQueueEntry, batchSize)}
}

func newCandidatePlanner(config contracts.MatchCoreConfig) *candidatePlanner {
	return &candidatePlanner{config: config, tolerance: toleranceCalculator{}}
}

func newBaselineScorer() baselineScorer {
	return baselineScorer{}
}

func newAssigner(ids contracts.MatchIDGenerator, clock contracts.MatchClock) *assigner {
	if clock == nil {
		clock = newSystemClock()
	}
	return &assigner{ids: ids, clock: clock}
}

func newMetricsSink() *metricsSink {
	return &metricsSink{}
}

func newIDGenerator(seed uint64) *idGenerator {
	g := &idGenerator{}
	g.next.Store(seed)
	return g
}

func newMatchCore(config contracts.MatchCoreConfig, shards uint16, ring contracts.TicketRingBuffer, store contracts.RedisQueueStore, keyer contracts.RedisQueueKeyer, codec contracts.RedisScoreCodec, clock contracts.MatchClock, metrics contracts.MatchMetricsSink, logger contracts.MatchOverrunLogger) (*matchCore, contracts.MatchCoreStatus) {
	if status := validateConfig(config); status != contracts.MatchCoreStatusOK {
		return nil, status
	}
	if shards == 0 || ring == nil || store == nil || keyer == nil || codec == nil {
		return nil, contracts.MatchCoreStatusInvalidConfig
	}
	if clock == nil {
		clock = systemClock{}
	}
	if metrics == nil {
		metrics = newMetricsSink()
	}
	if logger == nil {
		logger = &overrunLogger{}
	}
	drainer := newRingDrainer(shards, config.DrainBatchSize)
	core := &matchCore{
		config:     config,
		clock:      clock,
		ring:       ring,
		store:      store,
		keyer:      keyer,
		codec:      codec,
		drainer:    drainer,
		ingress:    newQueueIngress(config.DrainBatchSize),
		planner:    newCandidatePlanner(config),
		scorer:     baselineScorer{},
		assign:     newAssigner(newIDGenerator(0), clock),
		metrics:    metrics,
		logger:     logger,
		drained:    make([]contracts.MatchDrainedTicket, config.DrainBatchSize),
		entries:    make([]contracts.RedisQueueEntry, config.DrainBatchSize),
		ranges:     make([]contracts.RedisScoreRange, config.DrainBatchSize),
		candidates: make([][]contracts.RedisCandidate, config.DrainBatchSize),
	}
	for i := range core.candidates {
		core.candidates[i] = make([]contracts.RedisCandidate, config.CandidateLimit)
	}
	core.batch = contracts.RedisQueryBatch{Ranges: core.ranges, Results: core.candidates}
	return core, contracts.MatchCoreStatusOK
}

// NewMatchCore creates the production tick loop from contracted dependencies.
func NewMatchCore(config contracts.MatchCoreConfig, shards uint16, ring contracts.TicketRingBuffer, store contracts.RedisQueueStore, keyer contracts.RedisQueueKeyer, codec contracts.RedisScoreCodec, clock contracts.MatchClock, metrics contracts.MatchMetricsSink, logger contracts.MatchOverrunLogger) (contracts.MatchCoreLoop, contracts.MatchCoreStatus) {
	return newMatchCore(config, shards, ring, store, keyer, codec, clock, metrics, logger)
}

func validateConfig(config contracts.MatchCoreConfig) contracts.MatchCoreStatus {
	if config.TickIntervalNanos != contracts.MatchTickIntervalNanos {
		return contracts.MatchCoreStatusInvalidConfig
	}
	if config.HardBudgetNanos <= 0 || config.HardBudgetNanos > config.TickIntervalNanos {
		return contracts.MatchCoreStatusInvalidConfig
	}
	if config.BaseTolerance <= 0 || config.MaxTolerance < config.BaseTolerance {
		return contracts.MatchCoreStatusInvalidConfig
	}
	if config.ToleranceK < 0 || math.IsNaN(config.ToleranceK) || math.IsInf(config.ToleranceK, 0) {
		return contracts.MatchCoreStatusInvalidConfig
	}
	if config.OverrunWarnThreshold == 0 || config.DrainBatchSize == 0 {
		return contracts.MatchCoreStatusInvalidConfig
	}
	if config.CandidateLimit != contracts.MatchCandidateScratchLimit {
		return contracts.MatchCoreStatusInvalidConfig
	}
	return contracts.MatchCoreStatusOK
}

func (systemClock) NowUnixNano() int64 {
	return time.Now().UnixNano()
}

func (toleranceCalculator) ComputeTolerance(input contracts.MatchToleranceInput, out *contracts.MatchToleranceResult) contracts.MatchCoreStatus {
	if out == nil || input.BaseTolerance <= 0 || input.MaxTolerance < input.BaseTolerance || input.K < 0 || math.IsNaN(input.K) || math.IsInf(input.K, 0) {
		if out != nil {
			*out = contracts.MatchToleranceResult{Status: contracts.MatchCoreStatusInvalidConfig}
		}
		return contracts.MatchCoreStatusInvalidConfig
	}
	wait := input.NowUnixNano - input.EnqueuedAtUnixNano
	if wait < 0 {
		wait = 0
	}
	seconds := float64(wait) / 1_000_000_000
	product := input.K * seconds
	result := contracts.MatchToleranceResult{
		WaitNanos:   wait,
		WaitSeconds: seconds,
		Product:     product,
		Status:      contracts.MatchCoreStatusOK,
	}
	if product > contracts.MatchToleranceOverflowProduct {
		result.ToleranceTrophies = input.MaxTolerance
		result.Clamped = true
		*out = result
		return contracts.MatchCoreStatusOK
	}
	value := float64(input.BaseTolerance) * math.Exp(product)
	if value >= float64(input.MaxTolerance) {
		result.ToleranceTrophies = input.MaxTolerance
		result.Clamped = true
		*out = result
		return contracts.MatchCoreStatusOK
	}
	result.ToleranceTrophies = int32(value)
	if result.ToleranceTrophies < input.BaseTolerance {
		result.ToleranceTrophies = input.BaseTolerance
	}
	*out = result
	return contracts.MatchCoreStatusOK
}

func (d *ringDrainer) DrainRings(ring contracts.TicketRingBuffer, _ int64, dst []contracts.MatchDrainedTicket) (uint32, contracts.MatchCoreStatus) {
	if d == nil || ring == nil {
		return 0, contracts.MatchCoreStatusInvalidConfig
	}
	var count uint32
	for shard := uint16(0); shard < d.shards && int(count) < len(dst); shard++ {
		remaining := len(dst) - int(count)
		if remaining > len(d.tickets) {
			remaining = len(d.tickets)
		}
		result := ring.DrainShard(contracts.RingShardID(shard), d.tickets[:remaining])
		if result.Status == contracts.RingReadEmpty || result.Status == contracts.RingReadClosed {
			continue
		}
		if result.Status != contracts.RingReadOK {
			continue
		}
		for i := uint32(0); i < result.Count && int(count) < len(dst); i++ {
			dst[count] = contracts.MatchDrainedTicket{
				Ticket:  d.tickets[i],
				ShardID: result.ShardID,
			}
			d.tickets[i] = nil
			count++
		}
	}
	return count, contracts.MatchCoreStatusOK
}

func (q *queueIngress) EnqueueDrained(ctx context.Context, store contracts.RedisQueueStore, keyer contracts.RedisQueueKeyer, codec contracts.RedisScoreCodec, drained []contracts.MatchDrainedTicket) contracts.MatchCoreStatus {
	if q == nil || store == nil || keyer == nil || codec == nil {
		return contracts.MatchCoreStatusInvalidConfig
	}
	for i := range drained {
		if drained[i].Ticket == nil {
			continue
		}
		entry := &q.entries[i%len(q.entries)]
		status := buildQueueEntry(drained[i].Ticket, keyer, codec, entry)
		if status != contracts.RedisStatusOK {
			return redisStatusToMatchStatus(status)
		}
		result := store.Enqueue(ctx, entry)
		if result.Status != contracts.RedisStatusOK {
			return redisStatusToMatchStatus(result.Status)
		}
	}
	return contracts.MatchCoreStatusOK
}

func (p *candidatePlanner) BuildCandidateQueries(codec contracts.RedisScoreCodec, entries []contracts.RedisQueueEntry, nowUnixNano int64, batch *contracts.RedisQueryBatch) contracts.MatchCoreStatus {
	if p == nil || codec == nil || batch == nil || int(batch.Count) > len(batch.Ranges) || len(batch.Ranges) == 0 || len(batch.Results) == 0 {
		return contracts.MatchCoreStatusInvalidConfig
	}
	batch.Count = 0
	limit := len(batch.Ranges)
	if len(batch.Results) < limit {
		limit = len(batch.Results)
	}
	for i := range entries {
		if int(batch.Count) >= limit {
			break
		}
		entry := &entries[i]
		if entry.Ticket.PlayerID == 0 {
			continue
		}
		var tolerance contracts.MatchToleranceResult
		status := p.tolerance.ComputeTolerance(contracts.MatchToleranceInput{
			EnqueuedAtUnixNano: entry.Ticket.EnqueuedAt,
			NowUnixNano:        nowUnixNano,
			BaseTolerance:      p.config.BaseTolerance,
			K:                  p.config.ToleranceK,
			MaxTolerance:       p.config.MaxTolerance,
		}, &tolerance)
		if status != contracts.MatchCoreStatusOK {
			return status
		}
		rangeIndex := batch.Count
		redisStatus := codec.ScoreRange(entry.Ticket.Trophies, entry.Ticket.EnqueuedAt, tolerance.ToleranceTrophies, entry.Pool, &batch.Ranges[rangeIndex])
		if redisStatus != contracts.RedisStatusOK {
			return redisStatusToMatchStatus(redisStatus)
		}
		batch.Ranges[rangeIndex].Limit = int64(p.config.CandidateLimit)
		batch.Count++
	}
	if batch.Count == 0 {
		return contracts.MatchCoreStatusNoWork
	}
	return contracts.MatchCoreStatusOK
}

func (baselineScorer) ScoreCandidate(input contracts.MatchCandidateContext, out *contracts.MatchCandidateScore) contracts.MatchCoreStatus {
	if out == nil {
		return contracts.MatchCoreStatusInvalidCandidate
	}
	*out = contracts.MatchCandidateScore{Decision: contracts.MatchCandidateReject}
	if input.Anchor.PlayerID == 0 || input.Candidate.Ticket.PlayerID == 0 || input.Anchor.PlayerID == input.Candidate.Ticket.PlayerID {
		return contracts.MatchCoreStatusInvalidCandidate
	}
	if input.AnchorPool > contracts.RedisPoolMonetize || input.CandidatePool > contracts.RedisPoolMonetize {
		return contracts.MatchCoreStatusInvalidCandidate
	}
	delta := input.Anchor.Trophies - input.Candidate.Ticket.Trophies
	if delta < 0 {
		delta = -delta
	}
	if delta > input.ToleranceTrophies {
		out.TrophyDelta = delta
		return contracts.MatchCoreStatusInvalidCandidate
	}
	vectorDistance := float32(0)
	retention := input.Anchor.ChurnRisk + input.Candidate.Ticket.ChurnRisk
	monetization := input.Anchor.MonetizationP + input.Candidate.Ticket.MonetizationP
	fitness := 1_000_000 - float32(delta) - vectorDistance*100 + retention*10 + monetization
	*out = contracts.MatchCandidateScore{
		Fitness:         fitness,
		TrophyDelta:     delta,
		VectorDistance:  vectorDistance,
		RetentionWeight: retention,
		PredictedWinP:   0.5,
		Decision:        contracts.MatchCandidateReplaceBest,
	}
	return contracts.MatchCoreStatusOK
}

func (noOpVectorScorer) CosineSimilarity(a [8]float32, b [8]float32) float32 {
	var dot float32
	for i := 0; i < len(a); i++ {
		dot += a[i] * b[i]
	}
	return dot
}

func chooseCandidate(current contracts.MatchPair, candidate contracts.MatchPair, hasCurrent bool) (contracts.MatchPair, contracts.MatchCandidateDecision) {
	if !hasCurrent {
		candidate.Score.Decision = contracts.MatchCandidateReplaceBest
		return candidate, contracts.MatchCandidateReplaceBest
	}
	if candidate.Score.Fitness > current.Score.Fitness {
		candidate.Score.Decision = contracts.MatchCandidateReplaceBest
		return candidate, contracts.MatchCandidateReplaceBest
	}
	if candidate.Score.Fitness < current.Score.Fitness {
		return current, contracts.MatchCandidateKeepExisting
	}
	if candidate.Score.TrophyDelta < current.Score.TrophyDelta {
		candidate.Score.Decision = contracts.MatchCandidateReplaceBest
		return candidate, contracts.MatchCandidateReplaceBest
	}
	if candidate.Score.TrophyDelta > current.Score.TrophyDelta {
		return current, contracts.MatchCandidateKeepExisting
	}
	if candidate.PlayerB.Ticket.EnqueuedAt < current.PlayerB.Ticket.EnqueuedAt {
		candidate.Score.Decision = contracts.MatchCandidateReplaceBest
		return candidate, contracts.MatchCandidateReplaceBest
	}
	if candidate.PlayerB.Ticket.EnqueuedAt > current.PlayerB.Ticket.EnqueuedAt {
		return current, contracts.MatchCandidateKeepExisting
	}
	if candidate.PlayerB.Ticket.PlayerID < current.PlayerB.Ticket.PlayerID {
		candidate.Score.Decision = contracts.MatchCandidateReplaceBest
		return candidate, contracts.MatchCandidateReplaceBest
	}
	return current, contracts.MatchCandidateKeepExisting
}

func (a *assigner) AssignPair(ctx context.Context, store contracts.RedisQueueStore, pair contracts.MatchPair, out *contracts.MatchResult) contracts.MatchCoreStatus {
	status, _, _ := a.assignPair(ctx, store, pair, out)
	return status
}

func (a *assigner) assignPair(ctx context.Context, store contracts.RedisQueueStore, pair contracts.MatchPair, out *contracts.MatchResult) (contracts.MatchCoreStatus, contracts.RedisQueueStatus, int64) {
	if out != nil {
		*out = contracts.MatchResult{}
	}
	if a == nil || a.ids == nil || store == nil || out == nil {
		return contracts.MatchCoreStatusInvalidConfig, contracts.RedisStatusInvalidScore, 0
	}
	if pair.PlayerA.Ticket.PlayerID == 0 || pair.PlayerA.Ticket.PlayerID == pair.PlayerB.Ticket.PlayerID {
		return contracts.MatchCoreStatusInvalidCandidate, contracts.RedisStatusInvalidScore, 0
	}
	matchID := pair.MatchID
	if matchID == 0 {
		matchID = a.ids.NextMatchID()
	}
	if matchID == 0 {
		return contracts.MatchCoreStatusInvalidCandidate, contracts.RedisStatusInvalidScore, 0
	}
	result := store.AssignMatch(ctx, contracts.RedisAssignRequest{
		SourceA: pair.SourceA,
		SourceB: pair.SourceB,
		PlayerA: pair.PlayerA.Member,
		PlayerB: pair.PlayerB.Member,
		MatchID: matchID,
	})
	if result.Status != contracts.RedisStatusOK {
		return redisStatusToMatchStatus(result.Status), result.Status, result.ElapsedNanos
	}
	*out = contracts.MatchResult{
		MatchID:       matchID,
		PlayerA:       pair.PlayerA.Ticket.PlayerID,
		PlayerB:       pair.PlayerB.Ticket.PlayerID,
		PredictedWinP: pair.Score.PredictedWinP,
		PoolSource:    poolSource(pair.SourceA),
		AssignedAt:    a.clock.NowUnixNano(),
	}
	return contracts.MatchCoreStatusOK, result.Status, result.ElapsedNanos
}

func (m *metricsSink) RecordTick(_ contracts.MatchTickMetrics) {
	if m != nil {
		m.ticks.Add(1)
	}
}

func (m *metricsSink) RecordOverrun(_ uint64, _ int64, _ uint32) {
	if m != nil {
		m.overruns.Add(1)
	}
}

func (m *metricsSink) RecordSkippedTick(_ uint64) {
	if m != nil {
		m.skipped.Add(1)
	}
}

func (m *metricsSink) RecordDualBooking(_ uint64) {
	if m != nil {
		m.dualBookings.Add(1)
	}
}

func (m *metricsSink) RecordEmptyQuery(_ contracts.RedisQueuePool) {
	if m != nil {
		m.emptyQueries.Add(1)
	}
}

func (m *metricsSink) RecordRedisStatus(_ contracts.RedisQueueStatus, _ int64) {
	if m != nil {
		m.redisStatuses.Add(1)
	}
}

func (l *overrunLogger) WarnConsecutiveOverruns(_ uint64, _ uint32, _ int64) {
	if l != nil {
		l.warnings.Add(1)
	}
}

func (g *idGenerator) NextMatchID() uint64 {
	if g == nil {
		return 0
	}
	return g.next.Add(1)
}

func (m *matchCore) HandleTick(ctx context.Context, input contracts.MatchTickInput) contracts.MatchTickResult {
	start := input.StartedUnixNano
	if start == 0 {
		start = m.clock.NowUnixNano()
	}
	result := contracts.MatchTickResult{Status: contracts.MatchCoreStatusOK, TickID: input.TickID, StartedUnixNano: start}
	metrics := contracts.MatchTickMetrics{TickID: input.TickID}
	m.state.lastStarted.Store(start)
	if ctx.Err() != nil {
		return m.finishTick(result, metrics, start, start, contracts.MatchCoreStatusCanceled)
	}
	if m.state.skipNextTick.Load() {
		m.state.skipNextTick.Store(false)
		skipped := m.state.skippedTicks.Add(1)
		metrics.SkippedTicks = skipped
		if m.metrics != nil {
			m.metrics.RecordSkippedTick(input.TickID)
		}
		return m.finishTick(result, metrics, start, start, contracts.MatchCoreStatusSkipped)
	}

	status := contracts.MatchCoreStatusOK
	drained, drainStatus := m.drainer.DrainRings(m.ring, start, m.drained)
	status = maxStatus(status, drainStatus)
	metrics.DrainedTickets = drained
	entryCount, enqueueStatus := m.enqueueDrained(ctx, m.drained[:drained], &metrics)
	status = maxStatus(status, enqueueStatus)
	if entryCount == 0 && drained == 0 {
		status = maxStatus(status, contracts.MatchCoreStatusNoWork)
	}
	if entryCount > 0 {
		status = maxStatus(status, m.planner.BuildCandidateQueries(m.codec, m.entries[:entryCount], start, &m.batch))
		if m.batch.Count > 0 {
			metrics.CandidateQueries = uint32(m.batch.Count)
			redisResult := m.store.FetchCandidateBatch(ctx, &m.batch)
			if m.metrics != nil {
				m.metrics.RecordRedisStatus(redisResult.Status, redisResult.ElapsedNanos)
			}
			status = maxStatus(status, redisStatusToMatchStatus(redisResult.Status))
			if redisResult.Status == contracts.RedisStatusEmpty {
				for i := uint16(0); i < m.batch.Count; i++ {
					metrics.EmptyQueries++
					if m.metrics != nil {
						m.metrics.RecordEmptyQuery(m.batch.Ranges[i].Pool)
					}
				}
			} else {
				status = maxStatus(status, m.scoreAndAssign(ctx, entryCount, start, &metrics))
			}
		}
	}

	finish := m.clock.NowUnixNano()
	duration := finish - start
	if duration < 0 {
		duration = 0
	}
	metrics.DurationNanos = duration
	if duration > m.config.HardBudgetNanos {
		total := m.state.totalOverruns.Add(1)
		consecutive := m.state.consecutiveOverrun.Add(1)
		m.state.skipNextTick.Store(true)
		metrics.OverrunCount = total
		metrics.ConsecutiveOverruns = consecutive
		if m.metrics != nil {
			m.metrics.RecordOverrun(input.TickID, duration, consecutive)
		}
		if consecutive == m.config.OverrunWarnThreshold && m.logger != nil {
			m.logger.WarnConsecutiveOverruns(input.TickID, consecutive, duration)
		}
		if consecutive >= m.config.OverrunWarnThreshold {
			status = maxStatus(status, contracts.MatchCoreStatusWarnOverrun)
		} else {
			status = maxStatus(status, contracts.MatchCoreStatusOverrun)
		}
	} else {
		m.state.consecutiveOverrun.Store(0)
		m.state.skipNextTick.Store(false)
		metrics.OverrunCount = m.state.totalOverruns.Load()
	}
	return m.finishTick(result, metrics, start, finish, status)
}

func (m *matchCore) Run(ctx context.Context) contracts.MatchCoreStatus {
	if ctx.Err() != nil {
		return contracts.MatchCoreStatusCanceled
	}
	ticker := time.NewTicker(time.Duration(m.config.TickIntervalNanos))
	defer ticker.Stop()
	var tickID uint64
	for {
		select {
		case scheduled := <-ticker.C:
			tickID++
			start := m.clock.NowUnixNano()
			_ = m.HandleTick(ctx, contracts.MatchTickInput{
				TickID:            tickID,
				ScheduledUnixNano: scheduled.UnixNano(),
				StartedUnixNano:   start,
				DeadlineUnixNano:  start + m.config.HardBudgetNanos,
			})
		case <-ctx.Done():
			return contracts.MatchCoreStatusCanceled
		}
	}
}

func (m *matchCore) SnapshotTickState(out *contracts.MatchTickState) contracts.MatchCoreStatus {
	if m == nil || out == nil {
		return contracts.MatchCoreStatusInvalidConfig
	}
	*out = contracts.MatchTickState{
		LastStartedUnixNano:  m.state.lastStarted.Load(),
		LastFinishedUnixNano: m.state.lastFinished.Load(),
		ConsecutiveOverruns:  m.state.consecutiveOverrun.Load(),
		TotalOverruns:        m.state.totalOverruns.Load(),
		SkippedTicks:         m.state.skippedTicks.Load(),
		SkipNextTick:         m.state.skipNextTick.Load(),
	}
	return contracts.MatchCoreStatusOK
}

func (m *matchCore) finishTick(result contracts.MatchTickResult, metrics contracts.MatchTickMetrics, start int64, finish int64, status contracts.MatchCoreStatus) contracts.MatchTickResult {
	if finish < start {
		finish = start
	}
	metrics.DurationNanos = finish - start
	metrics.OverrunCount = m.state.totalOverruns.Load()
	metrics.ConsecutiveOverruns = m.state.consecutiveOverrun.Load()
	metrics.SkippedTicks = m.state.skippedTicks.Load()
	result.Status = status
	result.StartedUnixNano = start
	result.FinishedUnixNano = finish
	result.Metrics = metrics
	m.state.lastFinished.Store(finish)
	if m.metrics != nil {
		m.metrics.RecordTick(metrics)
	}
	return result
}

func (m *matchCore) enqueueDrained(ctx context.Context, drained []contracts.MatchDrainedTicket, metrics *contracts.MatchTickMetrics) (uint32, contracts.MatchCoreStatus) {
	var entries uint32
	status := contracts.MatchCoreStatusOK
	for i := range drained {
		if drained[i].Ticket == nil {
			continue
		}
		entry := &m.entries[entries]
		redisStatus := buildQueueEntry(drained[i].Ticket, m.keyer, m.codec, entry)
		if redisStatus != contracts.RedisStatusOK {
			if m.metrics != nil {
				m.metrics.RecordRedisStatus(redisStatus, 0)
			}
			status = maxStatus(status, redisStatusToMatchStatus(redisStatus))
			continue
		}
		result := m.store.Enqueue(ctx, entry)
		if m.metrics != nil {
			m.metrics.RecordRedisStatus(result.Status, result.ElapsedNanos)
		}
		status = maxStatus(status, redisStatusToMatchStatus(result.Status))
		if result.Status == contracts.RedisStatusTimeout {
			metrics.RedisTimeouts++
		}
		if result.Status == contracts.RedisStatusUnavailable {
			metrics.RedisUnavailable++
		}
		if result.Status == contracts.RedisStatusOK {
			entries++
		}
	}
	return entries, status
}

func (m *matchCore) scoreAndAssign(ctx context.Context, entryCount uint32, nowUnixNano int64, metrics *contracts.MatchTickMetrics) contracts.MatchCoreStatus {
	status := contracts.MatchCoreStatusOK
	for i := uint32(0); i < entryCount && i < uint32(m.batch.Count); i++ {
		anchor := m.entries[i]
		hasBest := false
		var best contracts.MatchPair
		for j := 0; j < len(m.candidates[i]); j++ {
			candidate := candidateEntry(m.candidates[i][j])
			if candidate.Ticket.PlayerID == 0 {
				continue
			}
			var score contracts.MatchCandidateScore
			scoreStatus := m.scorer.ScoreCandidate(contracts.MatchCandidateContext{
				Anchor:            anchor.Ticket,
				Candidate:         candidate,
				AnchorPool:        anchor.Pool,
				CandidatePool:     candidate.Pool,
				ToleranceTrophies: toleranceFromRange(anchor.Score, m.batch.Ranges[i]),
				NowUnixNano:       nowUnixNano,
			}, &score)
			if scoreStatus != contracts.MatchCoreStatusOK || score.Decision == contracts.MatchCandidateReject {
				continue
			}
			pair := contracts.MatchPair{
				PlayerA: anchor,
				PlayerB: candidate,
				SourceA: anchor.Pool,
				SourceB: candidate.Pool,
				Score:   score,
			}
			best, _ = chooseCandidate(best, pair, hasBest)
			hasBest = true
		}
		if !hasBest {
			metrics.EmptyQueries++
			if m.metrics != nil {
				m.metrics.RecordEmptyQuery(anchor.Pool)
			}
			continue
		}
		assignStatus, redisStatus, elapsedNanos := m.assign.assignPair(ctx, m.store, best, &m.result)
		if m.metrics != nil {
			m.metrics.RecordRedisStatus(redisStatus, elapsedNanos)
		}
		status = maxStatus(status, assignStatus)
		if assignStatus == contracts.MatchCoreStatusOK {
			metrics.MatchesMade++
		}
		if assignStatus == contracts.MatchCoreStatusDualBooking {
			metrics.DualBookings++
			if m.metrics != nil {
				m.metrics.RecordDualBooking(best.MatchID)
			}
		}
		if assignStatus == contracts.MatchCoreStatusRedisTimeout {
			metrics.RedisTimeouts++
		}
		if assignStatus == contracts.MatchCoreStatusRedisUnavailable {
			metrics.RedisUnavailable++
		}
	}
	return status
}

func buildQueueEntry(ticket *contracts.Ticket, keyer contracts.RedisQueueKeyer, codec contracts.RedisScoreCodec, out *contracts.RedisQueueEntry) contracts.RedisQueueStatus {
	if ticket == nil || out == nil {
		return contracts.RedisStatusInvalidScore
	}
	pool := poolForTicket(*ticket, keyer)
	if pool > contracts.RedisPoolMonetize {
		*out = contracts.RedisQueueEntry{}
		return contracts.RedisStatusInvalidSegment
	}
	*out = contracts.RedisQueueEntry{Ticket: *ticket, Pool: pool}
	if status := codec.EncodeMember(ticket.PlayerID, &out.Member); status != contracts.RedisStatusOK {
		*out = contracts.RedisQueueEntry{}
		return status
	}
	if status := codec.EncodeScore(ticket.Trophies, ticket.EnqueuedAt, &out.Score); status != contracts.RedisStatusOK {
		*out = contracts.RedisQueueEntry{}
		return status
	}
	return contracts.RedisStatusOK
}

func poolForTicket(ticket contracts.Ticket, keyer contracts.RedisQueueKeyer) contracts.RedisQueuePool {
	switch ticket.PoolTag {
	case contracts.PoolMainstream:
		return keyer.SegmentForTrophies(ticket.Trophies)
	case contracts.PoolLosers:
		return contracts.RedisPoolLosers
	case contracts.PoolRetention:
		return contracts.RedisPoolRetention
	case contracts.PoolMonetize:
		return contracts.RedisPoolMonetize
	default:
		return contracts.RedisQueuePool(math.MaxUint8)
	}
}

func candidateEntry(candidate contracts.RedisCandidate) contracts.RedisQueueEntry {
	if candidate.Member.PlayerID == 0 {
		return contracts.RedisQueueEntry{}
	}
	return contracts.RedisQueueEntry{
		Ticket: contracts.Ticket{
			PlayerID:   candidate.Member.PlayerID,
			EnqueuedAt: candidate.Score.EnqueuedAtMicros * 1_000,
			Trophies:   candidate.Score.Trophies,
			PoolTag:    poolTag(candidate.Pool),
		},
		Member: candidate.Member,
		Score:  candidate.Score,
		Pool:   candidate.Pool,
	}
}

func toleranceFromRange(score contracts.RedisScore, scoreRange contracts.RedisScoreRange) int32 {
	delta := scoreRange.Max - score.Value
	if delta < 0 {
		delta = 0
	}
	return int32(delta / float64(contracts.RedisScoreTrophyScale))
}

func poolSource(pool contracts.RedisQueuePool) contracts.TicketPoolTag {
	return poolTag(pool)
}

func poolTag(pool contracts.RedisQueuePool) contracts.TicketPoolTag {
	switch pool {
	case contracts.RedisPoolLosers:
		return contracts.PoolLosers
	case contracts.RedisPoolRetention:
		return contracts.PoolRetention
	case contracts.RedisPoolMonetize:
		return contracts.PoolMonetize
	default:
		return contracts.PoolMainstream
	}
}

func redisStatusToMatchStatus(status contracts.RedisQueueStatus) contracts.MatchCoreStatus {
	switch status {
	case contracts.RedisStatusOK:
		return contracts.MatchCoreStatusOK
	case contracts.RedisStatusEmpty:
		return contracts.MatchCoreStatusNoWork
	case contracts.RedisStatusTimeout:
		return contracts.MatchCoreStatusRedisTimeout
	case contracts.RedisStatusUnavailable, contracts.RedisStatusScriptNotLoaded, contracts.RedisStatusNoScript, contracts.RedisStatusPartial:
		return contracts.MatchCoreStatusRedisUnavailable
	case contracts.RedisStatusDualBooking:
		return contracts.MatchCoreStatusDualBooking
	case contracts.RedisStatusCanceled:
		return contracts.MatchCoreStatusCanceled
	case contracts.RedisStatusInvalidSegment, contracts.RedisStatusInvalidScore:
		return contracts.MatchCoreStatusInvalidCandidate
	default:
		return contracts.MatchCoreStatusRedisUnavailable
	}
}

func maxStatus(current contracts.MatchCoreStatus, next contracts.MatchCoreStatus) contracts.MatchCoreStatus {
	if next == contracts.MatchCoreStatusOK {
		return current
	}
	if current == contracts.MatchCoreStatusOK || severity(next) > severity(current) {
		return next
	}
	return current
}

func severity(status contracts.MatchCoreStatus) uint8 {
	switch status {
	case contracts.MatchCoreStatusCanceled:
		return 10
	case contracts.MatchCoreStatusInvalidConfig:
		return 9
	case contracts.MatchCoreStatusWarnOverrun:
		return 8
	case contracts.MatchCoreStatusOverrun:
		return 7
	case contracts.MatchCoreStatusRedisTimeout:
		return 6
	case contracts.MatchCoreStatusRedisUnavailable:
		return 5
	case contracts.MatchCoreStatusDualBooking:
		return 4
	case contracts.MatchCoreStatusInvalidCandidate:
		return 3
	case contracts.MatchCoreStatusSkipped:
		return 2
	case contracts.MatchCoreStatusNoWork:
		return 1
	default:
		return 0
	}
}
