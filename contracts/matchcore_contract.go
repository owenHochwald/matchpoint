// Package contracts defines the Planner-owned public contract for MatchPoint
// modules. This file is intentionally declarative: no implementation logic
// belongs here.
package contracts

import "context"

// MatchTickIntervalNanos is the production master ticker period: 200ms.
const MatchTickIntervalNanos int64 = 200_000_000

// MatchTickHardBudgetNanos is the maximum allowed handler duration before an overrun is recorded.
const MatchTickHardBudgetNanos int64 = MatchTickIntervalNanos

// MatchRingDrainBudgetNanos is the approximate 5ms per-tick ring drain budget.
const MatchRingDrainBudgetNanos int64 = 5_000_000

// MatchRedisQueryBudgetNanos is the approximate 40ms per-tick candidate query and scoring budget.
const MatchRedisQueryBudgetNanos int64 = 40_000_000

// MatchFitnessBudgetNanos is the approximate 80ms per-tick EOMM/vector scoring budget.
const MatchFitnessBudgetNanos int64 = 80_000_000

// MatchAssignBudgetNanos is the approximate 20ms per-tick Lua assignment budget.
const MatchAssignBudgetNanos int64 = 20_000_000

// MatchTelemetryBudgetNanos is the approximate 5ms per-tick metrics emission budget.
const MatchTelemetryBudgetNanos int64 = 5_000_000

// MatchOverrunWarnThreshold is the consecutive-overrun count that requires a WARN log.
const MatchOverrunWarnThreshold uint32 = 3

// MatchDefaultBaseTolerance is the default mainstream tolerance in trophies.
const MatchDefaultBaseTolerance int32 = 50

// MatchMaxTolerance is the hard maximum tolerance in trophies.
const MatchMaxTolerance int32 = 2000

// MatchDefaultToleranceK is the default exponential expansion coefficient.
const MatchDefaultToleranceK float64 = 0.15

// MatchToleranceOverflowProduct is the k*t guard that forces MaxTolerance.
const MatchToleranceOverflowProduct float64 = 10.0

// MatchCandidateScratchLimit is the maximum candidate count requested per query.
const MatchCandidateScratchLimit uint16 = uint16(RedisCandidateLimit)

// MatchCoreStatus is the stable non-allocating status taxonomy for matchcore operations.
type MatchCoreStatus uint8

const (
	// MatchCoreStatusOK means the operation completed without warnings.
	MatchCoreStatusOK MatchCoreStatus = 0
	// MatchCoreStatusNoWork means the tick had no drained tickets and no candidates to process.
	MatchCoreStatusNoWork MatchCoreStatus = 1
	// MatchCoreStatusSkipped means this tick was intentionally skipped after a prior overrun.
	MatchCoreStatusSkipped MatchCoreStatus = 2
	// MatchCoreStatusOverrun means the tick handler exceeded MatchTickHardBudgetNanos.
	MatchCoreStatusOverrun MatchCoreStatus = 3
	// MatchCoreStatusWarnOverrun means at least MatchOverrunWarnThreshold consecutive overruns occurred.
	MatchCoreStatusWarnOverrun MatchCoreStatus = 4
	// MatchCoreStatusRedisTimeout means a Redis boundary returned RedisStatusTimeout.
	MatchCoreStatusRedisTimeout MatchCoreStatus = 5
	// MatchCoreStatusRedisUnavailable means a Redis boundary returned RedisStatusUnavailable.
	MatchCoreStatusRedisUnavailable MatchCoreStatus = 6
	// MatchCoreStatusDualBooking means AssignMatch reported RedisStatusDualBooking.
	MatchCoreStatusDualBooking MatchCoreStatus = 7
	// MatchCoreStatusInvalidConfig means immutable config violates contracted bounds.
	MatchCoreStatusInvalidConfig MatchCoreStatus = 8
	// MatchCoreStatusInvalidCandidate means candidate selection produced a self-match or malformed pair.
	MatchCoreStatusInvalidCandidate MatchCoreStatus = 9
	// MatchCoreStatusCanceled means the caller context was canceled before the operation completed.
	MatchCoreStatusCanceled MatchCoreStatus = 10
)

// MatchCandidateDecision identifies the selection result for one candidate comparison.
type MatchCandidateDecision uint8

const (
	// MatchCandidateReject means the candidate pair is invalid for this tick.
	MatchCandidateReject MatchCandidateDecision = 0
	// MatchCandidateKeepExisting means an existing best candidate remains preferable.
	MatchCandidateKeepExisting MatchCandidateDecision = 1
	// MatchCandidateReplaceBest means this candidate becomes the current best pair.
	MatchCandidateReplaceBest MatchCandidateDecision = 2
)

// MatchCoreConfig defines immutable matchcore construction parameters.
type MatchCoreConfig struct {
	// TickIntervalNanos must be MatchTickIntervalNanos in production.
	TickIntervalNanos int64
	// HardBudgetNanos must be less than or equal to TickIntervalNanos.
	HardBudgetNanos int64
	// BaseTolerance is the mainstream starting tolerance and must be positive.
	BaseTolerance int32
	// ToleranceK is the exponential expansion coefficient and must be finite and non-negative.
	ToleranceK float64
	// MaxTolerance is the hard trophy clamp and must be greater than or equal to BaseTolerance.
	MaxTolerance int32
	// OverrunWarnThreshold must be MatchOverrunWarnThreshold in production.
	OverrunWarnThreshold uint32
	// DrainBatchSize is the fixed caller-owned ring-drain scratch capacity and must be greater than zero.
	DrainBatchSize uint32
	// CandidateLimit must be MatchCandidateScratchLimit for Redis ZRANGEBYSCORE queries.
	CandidateLimit uint16
}

// MatchTickInput is one externally supplied tick signal.
type MatchTickInput struct {
	// TickID is monotonically increasing for attempted ticks.
	TickID uint64
	// ScheduledUnixNano is the nominal ticker timestamp for drift accounting.
	ScheduledUnixNano int64
	// StartedUnixNano is the observed handler start timestamp.
	StartedUnixNano int64
	// DeadlineUnixNano is StartedUnixNano plus the hard tick budget.
	DeadlineUnixNano int64
}

// MatchTickState is the mutable scheduler state owned by the matchcore goroutine.
type MatchTickState struct {
	// LastStartedUnixNano is the most recent handler start timestamp.
	LastStartedUnixNano int64
	// LastFinishedUnixNano is the most recent handler finish timestamp.
	LastFinishedUnixNano int64
	// ConsecutiveOverruns is reset to zero after a non-overrun tick.
	ConsecutiveOverruns uint32
	// TotalOverruns is monotonically increasing for all over-budget ticks.
	TotalOverruns uint64
	// SkippedTicks is monotonically increasing for ticks skipped after overruns.
	SkippedTicks uint64
	// SkipNextTick is true only between an overrun and the immediately following tick signal.
	SkipNextTick bool
}

// MatchToleranceInput describes one tolerance calculation.
type MatchToleranceInput struct {
	// EnqueuedAtUnixNano is copied from Ticket.EnqueuedAt and must not be mutated.
	EnqueuedAtUnixNano int64
	// NowUnixNano is the current tick timestamp used for wait-time calculation.
	NowUnixNano int64
	// BaseTolerance is the configured starting tolerance in trophies.
	BaseTolerance int32
	// K is the configured exponential expansion coefficient.
	K float64
	// MaxTolerance is the hard trophy clamp.
	MaxTolerance int32
}

// MatchToleranceResult is the observable output of Tolerance(t)=BaseTolerance*exp(k*t).
type MatchToleranceResult struct {
	// WaitNanos is max(0, NowUnixNano-EnqueuedAtUnixNano).
	WaitNanos int64
	// WaitSeconds is WaitNanos converted to seconds for the exponent.
	WaitSeconds float64
	// Product is K*WaitSeconds and is never passed to exp when greater than MatchToleranceOverflowProduct.
	Product float64
	// ToleranceTrophies is the final integer tolerance passed to RedisScoreCodec.ScoreRange.
	ToleranceTrophies int32
	// Clamped is true when MaxTolerance was selected by overflow guard or math.Min clamp.
	Clamped bool
	// Status is MatchCoreStatusOK or MatchCoreStatusInvalidConfig.
	Status MatchCoreStatus
}

// MatchDrainedTicket represents one ticket transferred from ringbuffer into Redis ownership.
type MatchDrainedTicket struct {
	// Ticket is the drained pointer returned by TicketRingBuffer and owned by matchcore until copied into RedisQueueEntry.
	Ticket *Ticket
	// ShardID identifies the ring shard that produced Ticket.
	ShardID RingShardID
	// Sequence is the ring slot generation consumed by this drain.
	Sequence RingSequence
}

// MatchCandidateContext is the scoring input for one anchor ticket and one Redis candidate.
type MatchCandidateContext struct {
	// Anchor is the active ticket being matched during this tick.
	Anchor Ticket
	// Candidate is the Redis candidate reconstructed by the queue store boundary.
	Candidate RedisQueueEntry
	// AnchorPool is the source Redis pool containing Anchor.
	AnchorPool RedisQueuePool
	// CandidatePool is the source Redis pool containing Candidate.
	CandidatePool RedisQueuePool
	// ToleranceTrophies is the tolerance used to find Candidate.
	ToleranceTrophies int32
	// NowUnixNano is the current tick timestamp.
	NowUnixNano int64
}

// MatchCandidateScore describes a candidate pair after future EOMM/vector scoring.
type MatchCandidateScore struct {
	// Fitness is the total score from the contracted scorer; higher is better for selection.
	Fitness float32
	// TrophyDelta is abs(A.Trophies-B.Trophies) and must be within ToleranceTrophies.
	TrophyDelta int32
	// VectorDistance is 1.0-CosineSimilarity from the future vector interface.
	VectorDistance float32
	// RetentionWeight is the future EOMM retention or monetization term.
	RetentionWeight float32
	// PredictedWinP is the simulation-only predicted win probability for PlayerA.
	PredictedWinP float32
	// Decision is reject, keep existing, or replace best.
	Decision MatchCandidateDecision
}

// MatchPair is the selected pair passed to Redis Lua assignment.
type MatchPair struct {
	// PlayerA is the anchor Redis entry.
	PlayerA RedisQueueEntry
	// PlayerB is the selected candidate Redis entry.
	PlayerB RedisQueueEntry
	// SourceA is the Redis pool containing PlayerA.
	SourceA RedisQueuePool
	// SourceB is the Redis pool containing PlayerB and may equal SourceA.
	SourceB RedisQueuePool
	// Score is the selected fitness score.
	Score MatchCandidateScore
	// MatchID is generated before RedisAssignRequest construction.
	MatchID uint64
}

// MatchResult is the fixed-size assignment result emitted after Redis commit.
type MatchResult struct {
	// MatchID is unique for the process lifetime.
	MatchID uint64
	// PlayerA is the first assigned player ID.
	PlayerA uint64
	// PlayerB is the second assigned player ID.
	PlayerB uint64
	// PredictedWinP is P(PlayerA wins) and is simulation-only client output.
	PredictedWinP float32
	// PoolSource identifies the pool family used for the pair.
	PoolSource TicketPoolTag
	// AssignedAt is the Unix nanosecond timestamp after successful Redis assignment.
	AssignedAt int64
}

// MatchTickMetrics is the allocation-free counter snapshot produced by one tick.
type MatchTickMetrics struct {
	// TickID is the attempted tick ID.
	TickID uint64
	// DurationNanos is FinishedUnixNano-StartedUnixNano.
	DurationNanos int64
	// DrainedTickets is the number of ring tickets copied into Redis entries.
	DrainedTickets uint32
	// CandidateQueries is the number of Redis score ranges issued.
	CandidateQueries uint32
	// EmptyQueries is the number of Redis candidate queries returning RedisStatusEmpty.
	EmptyQueries uint32
	// MatchesMade is the number of successful Redis assignments.
	MatchesMade uint32
	// DualBookings is the number of RedisStatusDualBooking assignment attempts.
	DualBookings uint32
	// RedisTimeouts is the number of RedisStatusTimeout results observed in this tick.
	RedisTimeouts uint32
	// RedisUnavailable is the number of RedisStatusUnavailable results observed in this tick.
	RedisUnavailable uint32
	// OverrunCount is the total overrun counter after this tick.
	OverrunCount uint64
	// ConsecutiveOverruns is the consecutive overrun counter after this tick.
	ConsecutiveOverruns uint32
	// SkippedTicks is the total skipped tick counter after this tick.
	SkippedTicks uint64
}

// MatchTickResult is the observable outcome of one attempted tick.
type MatchTickResult struct {
	// Status is OK, skipped, overrun, warning, or a boundary error status.
	Status MatchCoreStatus
	// TickID is the attempted tick ID.
	TickID uint64
	// StartedUnixNano is copied from MatchTickInput.
	StartedUnixNano int64
	// FinishedUnixNano is captured at handler completion.
	FinishedUnixNano int64
	// Metrics contains the complete per-tick counter snapshot.
	Metrics MatchTickMetrics
}

// MatchClock provides monotonic-like timestamps without coupling contracts to time.Ticker.
type MatchClock interface {
	// NowUnixNano returns the current server timestamp in Unix nanoseconds.
	NowUnixNano() int64
}

// MatchToleranceCalculator computes exponential wait tolerance for one ticket.
type MatchToleranceCalculator interface {
	// ComputeTolerance computes Tolerance(t)=BaseTolerance*exp(k*t) with overflow guard and MaxTolerance clamp.
	ComputeTolerance(input MatchToleranceInput, out *MatchToleranceResult) MatchCoreStatus
}

// MatchRingDrainer drains ingestion shards into caller-owned scratch storage.
type MatchRingDrainer interface {
	// DrainRings reads all shards through TicketRingBuffer.DrainShard without allocating or blocking.
	DrainRings(ring TicketRingBuffer, nowUnixNano int64, dst []MatchDrainedTicket) (uint32, MatchCoreStatus)
}

// MatchQueueIngress copies drained tickets into RedisQueueEntry values and enqueues them.
type MatchQueueIngress interface {
	// EnqueueDrained writes drained tickets to Redis through RedisQueueStore.Enqueue and does not retain ticket pointers.
	EnqueueDrained(ctx context.Context, store RedisQueueStore, keyer RedisQueueKeyer, codec RedisScoreCodec, drained []MatchDrainedTicket) MatchCoreStatus
}

// MatchCandidatePlanner prepares Redis score ranges for one tick.
type MatchCandidatePlanner interface {
	// BuildCandidateQueries computes tolerance and RedisScoreRange entries into caller-owned batch storage.
	BuildCandidateQueries(codec RedisScoreCodec, entries []RedisQueueEntry, nowUnixNano int64, batch *RedisQueryBatch) MatchCoreStatus
}

// MatchFitnessScorer is the future EOMM/vector scoring boundary consumed by matchcore.
type MatchFitnessScorer interface {
	// ScoreCandidate evaluates one anchor/candidate pair without mutating either ticket.
	ScoreCandidate(input MatchCandidateContext, out *MatchCandidateScore) MatchCoreStatus
}

// MatchVectorScorer is the future vectorarch cosine-similarity boundary used by EOMM scoring.
type MatchVectorScorer interface {
	// CosineSimilarity returns dot(a,b) for already-normalized 8-dimensional vectors.
	CosineSimilarity(a [8]float32, b [8]float32) float32
}

// MatchAssigner commits selected pairs through Redis Lua assignment.
type MatchAssigner interface {
	// AssignPair builds RedisAssignRequest, rejects self-matches, calls RedisQueueStore.AssignMatch, and maps dual booking.
	AssignPair(ctx context.Context, store RedisQueueStore, pair MatchPair, out *MatchResult) MatchCoreStatus
}

// MatchMetricsSink records allocation-free tick, Redis, and overrun counters.
type MatchMetricsSink interface {
	// RecordTick records one completed, overrun, or skipped tick.
	RecordTick(metrics MatchTickMetrics)
	// RecordOverrun records a tick that exceeded MatchTickHardBudgetNanos.
	RecordOverrun(tickID uint64, durationNanos int64, consecutive uint32)
	// RecordSkippedTick records an intentionally skipped tick after an overrun.
	RecordSkippedTick(tickID uint64)
	// RecordDualBooking records one RedisStatusDualBooking assignment result.
	RecordDualBooking(matchID uint64)
	// RecordEmptyQuery records one candidate query with no Redis members.
	RecordEmptyQuery(pool RedisQueuePool)
	// RecordRedisStatus records timeout, unavailable, partial, canceled, or OK Redis outcomes.
	RecordRedisStatus(status RedisQueueStatus, elapsedNanos int64)
}

// MatchOverrunLogger is the non-hot-path warning boundary for three consecutive overruns.
type MatchOverrunLogger interface {
	// WarnConsecutiveOverruns logs once when consecutive reaches the configured warning threshold.
	WarnConsecutiveOverruns(tickID uint64, consecutive uint32, durationNanos int64)
}

// MatchIDGenerator provides process-unique match identifiers without allocation.
type MatchIDGenerator interface {
	// NextMatchID returns a non-zero unique match identifier.
	NextMatchID() uint64
}

// MatchCoreLoop owns the 200ms tick orchestration boundary.
type MatchCoreLoop interface {
	// HandleTick processes one tick signal and never queues missed ticks.
	HandleTick(ctx context.Context, input MatchTickInput) MatchTickResult
	// Run consumes ticker signals until ctx cancellation and invokes HandleTick for each non-skipped tick.
	Run(ctx context.Context) MatchCoreStatus
	// SnapshotTickState copies scheduler state for tests and telemetry.
	SnapshotTickState(out *MatchTickState) MatchCoreStatus
}
