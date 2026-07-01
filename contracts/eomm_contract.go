// Package contracts defines the Planner-owned public contract for MatchPoint
// modules. This file is intentionally declarative: no implementation logic
// belongs here.
package contracts

import "context"

// EOMMDefaultTrophyWeight is w1 for normalized trophy proximity penalty.
const EOMMDefaultTrophyWeight float32 = 0.4

// EOMMDefaultVectorWeight is w2 for cosine-distance penalty.
const EOMMDefaultVectorWeight float32 = 0.3

// EOMMDefaultRetentionWeight is w3 for retention and monetization modifiers.
const EOMMDefaultRetentionWeight float32 = 0.3

// EOMMWeightSum is the required total for all configured score weights.
const EOMMWeightSum float32 = 1.0

// EOMMWeightEpsilon is the accepted floating-point tolerance for weight sums.
const EOMMWeightEpsilon float32 = 0.0001

// EOMMHighChurnThreshold routes high-risk players into retention handling.
const EOMMHighChurnThreshold float32 = 0.75

// EOMMHighMonetizationThreshold routes high-spend-propensity winners.
const EOMMHighMonetizationThreshold float32 = 0.80

// EOMMLoserBaseTolerance is the independent loser's pool tolerance in trophies.
const EOMMLoserBaseTolerance int32 = 200

// EOMMLoserStarvationTicks is the strict tick-count threshold for evacuation.
const EOMMLoserStarvationTicks uint16 = 10

// EOMMRetentionTrophyOffset is the preferred easier-opponent trophy gap.
const EOMMRetentionTrophyOffset int32 = 100

// EOMMRetentionTargetWinP is the simulation target for retention matches.
const EOMMRetentionTargetWinP float32 = 0.70

// EOMMMonetizeTargetWinP is the simulation target for monetization trigger matches.
const EOMMMonetizeTargetWinP float32 = 0.30

// EOMMCounterSimilarityThreshold identifies structural counter archetypes.
const EOMMCounterSimilarityThreshold float32 = 0.20

// EOMMSimilarSimilarityThreshold identifies stylistically similar decks.
const EOMMSimilarSimilarityThreshold float32 = 0.75

// EOMMSpikeWindowTicks is the rolling window used for adaptive spike detection.
const EOMMSpikeWindowTicks uint8 = 10

// EOMMSpikeWinRateThreshold routes sudden low-win-rate churn spikes.
const EOMMSpikeWinRateThreshold float32 = 0.30

// EOMMStatus is the stable non-allocating status taxonomy for EOMM operations.
type EOMMStatus uint8

const (
	// EOMMStatusOK means the operation completed successfully.
	EOMMStatusOK EOMMStatus = 0
	// EOMMStatusNoop means no routing or spike action was required.
	EOMMStatusNoop EOMMStatus = 1
	// EOMMStatusInvalidConfig means immutable EOMM config is not legal.
	EOMMStatusInvalidConfig EOMMStatus = 2
	// EOMMStatusWeightMismatch means score weights do not sum to EOMMWeightSum.
	EOMMStatusWeightMismatch EOMMStatus = 3
	// EOMMStatusInvalidTicket means required ticket or queue metadata is malformed.
	EOMMStatusInvalidTicket EOMMStatus = 4
	// EOMMStatusRedisTimeout maps RedisStatusTimeout from RedisQueueStore.
	EOMMStatusRedisTimeout EOMMStatus = 5
	// EOMMStatusRedisUnavailable maps unavailable, NOSCRIPT, partial, or unknown Redis statuses.
	EOMMStatusRedisUnavailable EOMMStatus = 6
	// EOMMStatusCanceled means caller context cancellation prevented completion.
	EOMMStatusCanceled EOMMStatus = 7
)

// EOMMRouteReason identifies why a ticket is routed or left unchanged.
type EOMMRouteReason uint8

const (
	// EOMMRouteMainstream means no special-pool condition matched.
	EOMMRouteMainstream EOMMRouteReason = 0
	// EOMMRouteLoser means ConsecLosses <= -2 selected the loser's pool.
	EOMMRouteLoser EOMMRouteReason = 1
	// EOMMRouteRetention means churn risk and a loss selected the retention pool.
	EOMMRouteRetention EOMMRouteReason = 2
	// EOMMRouteMonetize means monetization probability and win streak selected monetization.
	EOMMRouteMonetize EOMMRouteReason = 3
	// EOMMRouteStarvationEvacuate means a starving loser-pool ticket returns to mainstream.
	EOMMRouteStarvationEvacuate EOMMRouteReason = 4
	// EOMMRouteWinEvacuate means a completed win returns loser or retention tickets to mainstream.
	EOMMRouteWinEvacuate EOMMRouteReason = 5
	// EOMMRouteCompleteEvacuate means a completed monetization match returns to mainstream.
	EOMMRouteCompleteEvacuate EOMMRouteReason = 6
)

// EOMMConfig defines immutable routing and scoring parameters.
type EOMMConfig struct {
	// TrophyWeight is w1 and must be non-negative.
	TrophyWeight float32
	// VectorWeight is w2 and must be non-negative.
	VectorWeight float32
	// RetentionWeight is w3 and must be non-negative.
	RetentionWeight float32
	// HighChurnThreshold defaults to EOMMHighChurnThreshold.
	HighChurnThreshold float32
	// HighMonetizationThreshold defaults to EOMMHighMonetizationThreshold.
	HighMonetizationThreshold float32
	// CounterSimilarityThreshold defaults to EOMMCounterSimilarityThreshold.
	CounterSimilarityThreshold float32
	// SimilarSimilarityThreshold defaults to EOMMSimilarSimilarityThreshold.
	SimilarSimilarityThreshold float32
	// LoserBaseTolerance defaults to EOMMLoserBaseTolerance.
	LoserBaseTolerance int32
	// LoserStarvationTicks defaults to EOMMLoserStarvationTicks.
	LoserStarvationTicks uint16
	// SpikeWindowTicks defaults to EOMMSpikeWindowTicks.
	SpikeWindowTicks uint8
	// SpikeWinRateThreshold defaults to EOMMSpikeWinRateThreshold.
	SpikeWinRateThreshold float32
}

// EOMMRoutingInput describes one ticket's current queue state.
type EOMMRoutingInput struct {
	// Entry is copied from Redis queue metadata and must not be retained.
	Entry RedisQueueEntry
	// CurrentPool is the Redis pool currently containing Entry.
	CurrentPool RedisQueuePool
	// MainstreamPool is the segment pool selected by RedisQueueKeyer.
	MainstreamPool RedisQueuePool
	// WaitTicks is the number of matchcore ticks since entry enqueue or last route.
	WaitTicks uint16
	// OtherLoserPoolPlayers is the count of other currently queued loser-pool players.
	OtherLoserPoolPlayers uint16
}

// EOMMRouteDecision describes one routing or evacuation action.
type EOMMRouteDecision struct {
	// Move is true when RedisQueueStore.MovePool must be called.
	Move bool
	// Member identifies the player moved between Redis pools.
	Member RedisMember
	// From is the currently expected source pool.
	From RedisQueuePool
	// To is the target pool after routing.
	To RedisQueuePool
	// PoolTag is the Ticket.PoolTag corresponding to To.
	PoolTag TicketPoolTag
	// Reason is the first mutually exclusive condition that selected To.
	Reason EOMMRouteReason
	// Score preserves the Redis ZSET score for MovePool.
	Score RedisScore
}

// EOMMMatchOutcome describes one completed player result for pool evacuation.
type EOMMMatchOutcome struct {
	// Entry is the queue entry that was matched.
	Entry RedisQueueEntry
	// SourcePool is the pool used for the match.
	SourcePool RedisQueuePool
	// MainstreamPool is the segment pool to return to when evacuation applies.
	MainstreamPool RedisQueuePool
	// Won is true when this player won the completed match.
	Won bool
}

// EOMMSpikeInput is one sample for adaptive churn spike detection.
type EOMMSpikeInput struct {
	// PlayerID identifies the rolling window slot owner and must be non-zero.
	PlayerID uint64
	// Won is the latest match outcome.
	Won bool
	// PreviousChurnRisk is the prior server-derived churn estimate.
	PreviousChurnRisk float32
	// CurrentChurnRisk is the latest server-derived churn estimate.
	CurrentChurnRisk float32
}

// EOMMSpikeState is the fixed-size rolling state for one player.
type EOMMSpikeState struct {
	// PlayerID identifies the window owner.
	PlayerID uint64
	// Outcomes stores the last EOMMSpikeWindowTicks samples as 1 for win and 0 for loss.
	Outcomes [10]uint8
	// Index is the next write offset in Outcomes.
	Index uint8
	// Count is the number of valid samples and is capped at EOMMSpikeWindowTicks.
	Count uint8
	// Wins is the number of wins in the valid window.
	Wins uint8
}

// EOMMChurnAlertEvent is emitted when adaptive spike detection routes retention.
type EOMMChurnAlertEvent struct {
	// PlayerID identifies the affected player.
	PlayerID uint64
	// RollingWinRate is Wins/Count after the latest sample.
	RollingWinRate float32
	// PreviousChurnRisk is the prior risk value.
	PreviousChurnRisk float32
	// CurrentChurnRisk is the risk value crossing the high-churn threshold.
	CurrentChurnRisk float32
	// PoolTag is always PoolRetention for emitted events.
	PoolTag TicketPoolTag
}

// EOMMScoreBreakdown exposes scoring terms for tests and telemetry.
type EOMMScoreBreakdown struct {
	// TrophyPenalty is abs(delta)/MatchMaxTolerance clamped to [0, 1].
	TrophyPenalty float32
	// VectorDistance is 1-CosineSimilarity clamped to [0, 2].
	VectorDistance float32
	// RetentionModifier is the signed retention or monetization term.
	RetentionModifier float32
	// PredictedWinP is simulation-only P(PlayerA wins).
	PredictedWinP float32
	// Total is the final fitness where higher is better.
	Total float32
}

// EOMMRouter computes mutually exclusive routing decisions.
type EOMMRouter interface {
	// RouteTicket computes loser, retention, monetization, mainstream, or starvation routing.
	RouteTicket(input EOMMRoutingInput, out *EOMMRouteDecision) EOMMStatus
	// RouteMatchOutcome computes post-match evacuation for special-pool players.
	RouteMatchOutcome(input EOMMMatchOutcome, out *EOMMRouteDecision) EOMMStatus
}

// EOMMPoolMover applies route decisions through the Redis queue boundary.
type EOMMPoolMover interface {
	// ApplyRoute calls RedisQueueStore.MovePool only when decision.Move is true.
	ApplyRoute(ctx context.Context, store RedisQueueStore, decision EOMMRouteDecision) EOMMStatus
}

// EOMMFitnessScorer scores candidates and satisfies MatchFitnessScorer.
type EOMMFitnessScorer interface {
	// ScoreCandidate fills MatchCandidateScore without mutating either ticket.
	ScoreCandidate(input MatchCandidateContext, out *MatchCandidateScore) MatchCoreStatus
	// ScoreBreakdown fills the EOMM-specific score terms.
	ScoreBreakdown(input MatchCandidateContext, out *EOMMScoreBreakdown) EOMMStatus
}

// EOMMSpikeDetector tracks rolling win-rate drops and churn threshold crossings.
type EOMMSpikeDetector interface {
	// RecordOutcome updates caller-owned rolling state and optionally emits a churn alert.
	RecordOutcome(input EOMMSpikeInput, state *EOMMSpikeState, out *EOMMChurnAlertEvent) EOMMStatus
}
