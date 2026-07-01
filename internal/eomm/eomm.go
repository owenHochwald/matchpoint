// Package eomm implements engagement-optimized routing and scoring.
package eomm

import (
	"context"
	"math"

	"matchpoint/contracts"
)

const (
	EOMMDefaultTrophyWeight                = contracts.EOMMDefaultTrophyWeight
	EOMMDefaultVectorWeight                = contracts.EOMMDefaultVectorWeight
	EOMMDefaultRetentionWeight             = contracts.EOMMDefaultRetentionWeight
	EOMMWeightSum                          = contracts.EOMMWeightSum
	EOMMWeightEpsilon                      = contracts.EOMMWeightEpsilon
	EOMMHighChurnThreshold                 = contracts.EOMMHighChurnThreshold
	EOMMHighMonetizationThreshold          = contracts.EOMMHighMonetizationThreshold
	EOMMLoserBaseTolerance                 = contracts.EOMMLoserBaseTolerance
	EOMMLoserStarvationTicks               = contracts.EOMMLoserStarvationTicks
	EOMMRetentionTrophyOffset              = contracts.EOMMRetentionTrophyOffset
	EOMMRetentionTargetWinP                = contracts.EOMMRetentionTargetWinP
	EOMMMonetizeTargetWinP                 = contracts.EOMMMonetizeTargetWinP
	EOMMCounterSimilarityThreshold         = contracts.EOMMCounterSimilarityThreshold
	EOMMSimilarSimilarityThreshold         = contracts.EOMMSimilarSimilarityThreshold
	EOMMSpikeWindowTicks                   = contracts.EOMMSpikeWindowTicks
	EOMMSpikeWinRateThreshold              = contracts.EOMMSpikeWinRateThreshold
	retentionBoost                 float32 = -0.5
	monetizeCounterPenalty         float32 = 0.4
)

const (
	EOMMStatusOK               = contracts.EOMMStatusOK
	EOMMStatusNoop             = contracts.EOMMStatusNoop
	EOMMStatusInvalidConfig    = contracts.EOMMStatusInvalidConfig
	EOMMStatusWeightMismatch   = contracts.EOMMStatusWeightMismatch
	EOMMStatusInvalidTicket    = contracts.EOMMStatusInvalidTicket
	EOMMStatusRedisTimeout     = contracts.EOMMStatusRedisTimeout
	EOMMStatusRedisUnavailable = contracts.EOMMStatusRedisUnavailable
	EOMMStatusCanceled         = contracts.EOMMStatusCanceled
)

const (
	EOMMRouteMainstream         = contracts.EOMMRouteMainstream
	EOMMRouteLoser              = contracts.EOMMRouteLoser
	EOMMRouteRetention          = contracts.EOMMRouteRetention
	EOMMRouteMonetize           = contracts.EOMMRouteMonetize
	EOMMRouteStarvationEvacuate = contracts.EOMMRouteStarvationEvacuate
	EOMMRouteWinEvacuate        = contracts.EOMMRouteWinEvacuate
	EOMMRouteCompleteEvacuate   = contracts.EOMMRouteCompleteEvacuate
)

type EOMMStatus = contracts.EOMMStatus
type EOMMRouteReason = contracts.EOMMRouteReason
type EOMMConfig = contracts.EOMMConfig
type EOMMRoutingInput = contracts.EOMMRoutingInput
type EOMMRouteDecision = contracts.EOMMRouteDecision
type EOMMMatchOutcome = contracts.EOMMMatchOutcome
type EOMMSpikeInput = contracts.EOMMSpikeInput
type EOMMSpikeState = contracts.EOMMSpikeState
type EOMMChurnAlertEvent = contracts.EOMMChurnAlertEvent
type EOMMScoreBreakdown = contracts.EOMMScoreBreakdown
type EOMMRouter = contracts.EOMMRouter
type EOMMPoolMover = contracts.EOMMPoolMover
type EOMMFitnessScorer = contracts.EOMMFitnessScorer
type EOMMSpikeDetector = contracts.EOMMSpikeDetector

type engine struct {
	config contracts.EOMMConfig
}

func productionConfig() contracts.EOMMConfig {
	return contracts.EOMMConfig{
		TrophyWeight:               contracts.EOMMDefaultTrophyWeight,
		VectorWeight:               contracts.EOMMDefaultVectorWeight,
		RetentionWeight:            contracts.EOMMDefaultRetentionWeight,
		HighChurnThreshold:         contracts.EOMMHighChurnThreshold,
		HighMonetizationThreshold:  contracts.EOMMHighMonetizationThreshold,
		CounterSimilarityThreshold: contracts.EOMMCounterSimilarityThreshold,
		SimilarSimilarityThreshold: contracts.EOMMSimilarSimilarityThreshold,
		LoserBaseTolerance:         contracts.EOMMLoserBaseTolerance,
		LoserStarvationTicks:       contracts.EOMMLoserStarvationTicks,
		SpikeWindowTicks:           contracts.EOMMSpikeWindowTicks,
		SpikeWinRateThreshold:      contracts.EOMMSpikeWinRateThreshold,
	}
}

func newEngine(config contracts.EOMMConfig) (*engine, contracts.EOMMStatus) {
	config = fillDefaults(config)
	if status := validateConfig(config); status != contracts.EOMMStatusOK {
		return nil, status
	}
	return &engine{config: config}, contracts.EOMMStatusOK
}

func fillDefaults(config contracts.EOMMConfig) contracts.EOMMConfig {
	if config.TrophyWeight == 0 && config.VectorWeight == 0 && config.RetentionWeight == 0 {
		config.TrophyWeight = contracts.EOMMDefaultTrophyWeight
		config.VectorWeight = contracts.EOMMDefaultVectorWeight
		config.RetentionWeight = contracts.EOMMDefaultRetentionWeight
	}
	if config.HighChurnThreshold == 0 {
		config.HighChurnThreshold = contracts.EOMMHighChurnThreshold
	}
	if config.HighMonetizationThreshold == 0 {
		config.HighMonetizationThreshold = contracts.EOMMHighMonetizationThreshold
	}
	if config.CounterSimilarityThreshold == 0 {
		config.CounterSimilarityThreshold = contracts.EOMMCounterSimilarityThreshold
	}
	if config.SimilarSimilarityThreshold == 0 {
		config.SimilarSimilarityThreshold = contracts.EOMMSimilarSimilarityThreshold
	}
	if config.LoserBaseTolerance == 0 {
		config.LoserBaseTolerance = contracts.EOMMLoserBaseTolerance
	}
	if config.LoserStarvationTicks == 0 {
		config.LoserStarvationTicks = contracts.EOMMLoserStarvationTicks
	}
	if config.SpikeWindowTicks == 0 {
		config.SpikeWindowTicks = contracts.EOMMSpikeWindowTicks
	}
	if config.SpikeWinRateThreshold == 0 {
		config.SpikeWinRateThreshold = contracts.EOMMSpikeWinRateThreshold
	}
	return config
}

func validateConfig(config contracts.EOMMConfig) contracts.EOMMStatus {
	if !finiteNonNegative(config.TrophyWeight) || !finiteNonNegative(config.VectorWeight) || !finiteNonNegative(config.RetentionWeight) {
		return contracts.EOMMStatusWeightMismatch
	}
	sum := config.TrophyWeight + config.VectorWeight + config.RetentionWeight
	if abs32(sum-contracts.EOMMWeightSum) > contracts.EOMMWeightEpsilon {
		return contracts.EOMMStatusWeightMismatch
	}
	if !finiteThreshold(config.HighChurnThreshold) || !finiteThreshold(config.HighMonetizationThreshold) ||
		!finiteThreshold(config.CounterSimilarityThreshold) || !finiteThreshold(config.SimilarSimilarityThreshold) ||
		!finiteThreshold(config.SpikeWinRateThreshold) || config.LoserBaseTolerance <= 0 ||
		config.LoserStarvationTicks == 0 || config.SpikeWindowTicks == 0 || config.SpikeWindowTicks > contracts.EOMMSpikeWindowTicks {
		return contracts.EOMMStatusInvalidConfig
	}
	return contracts.EOMMStatusOK
}

func (e *engine) RouteTicket(input contracts.EOMMRoutingInput, out *contracts.EOMMRouteDecision) contracts.EOMMStatus {
	if e == nil || out == nil || input.Entry.Ticket.PlayerID == 0 || input.Entry.Member.PlayerID == 0 || !validPool(input.CurrentPool) || !validSegment(input.MainstreamPool) {
		return contracts.EOMMStatusInvalidTicket
	}
	target, tag, reason := e.routeTarget(input)
	*out = contracts.EOMMRouteDecision{
		Move:    target != input.CurrentPool,
		Member:  input.Entry.Member,
		From:    input.CurrentPool,
		To:      target,
		PoolTag: tag,
		Reason:  reason,
		Score:   input.Entry.Score,
	}
	return contracts.EOMMStatusOK
}

func (e *engine) routeTarget(input contracts.EOMMRoutingInput) (contracts.RedisQueuePool, contracts.TicketPoolTag, contracts.EOMMRouteReason) {
	ticket := input.Entry.Ticket
	if input.CurrentPool == contracts.RedisPoolLosers && input.WaitTicks > e.config.LoserStarvationTicks && input.OtherLoserPoolPlayers < 2 {
		return input.MainstreamPool, contracts.PoolMainstream, contracts.EOMMRouteStarvationEvacuate
	}
	if ticket.ConsecLosses <= -2 {
		return contracts.RedisPoolLosers, contracts.PoolLosers, contracts.EOMMRouteLoser
	}
	if ticket.ChurnRisk > e.config.HighChurnThreshold && ticket.ConsecLosses <= -1 {
		return contracts.RedisPoolRetention, contracts.PoolRetention, contracts.EOMMRouteRetention
	}
	if ticket.MonetizationP > e.config.HighMonetizationThreshold && ticket.ConsecWins >= 2 {
		return contracts.RedisPoolMonetize, contracts.PoolMonetize, contracts.EOMMRouteMonetize
	}
	return input.MainstreamPool, contracts.PoolMainstream, contracts.EOMMRouteMainstream
}

func (e *engine) RouteMatchOutcome(input contracts.EOMMMatchOutcome, out *contracts.EOMMRouteDecision) contracts.EOMMStatus {
	if e == nil || out == nil || input.Entry.Ticket.PlayerID == 0 || input.Entry.Member.PlayerID == 0 || !validPool(input.SourcePool) || !validSegment(input.MainstreamPool) {
		return contracts.EOMMStatusInvalidTicket
	}
	reason := contracts.EOMMRouteMainstream
	move := false
	switch input.SourcePool {
	case contracts.RedisPoolLosers, contracts.RedisPoolRetention:
		move = input.Won
		reason = contracts.EOMMRouteWinEvacuate
	case contracts.RedisPoolMonetize:
		move = true
		reason = contracts.EOMMRouteCompleteEvacuate
	}
	*out = contracts.EOMMRouteDecision{
		Move:    move,
		Member:  input.Entry.Member,
		From:    input.SourcePool,
		To:      input.MainstreamPool,
		PoolTag: contracts.PoolMainstream,
		Reason:  reason,
		Score:   input.Entry.Score,
	}
	return contracts.EOMMStatusOK
}

func (e *engine) ApplyRoute(ctx context.Context, store contracts.RedisQueueStore, decision contracts.EOMMRouteDecision) contracts.EOMMStatus {
	if e == nil || store == nil || decision.Member.PlayerID == 0 {
		return contracts.EOMMStatusInvalidTicket
	}
	if !decision.Move || decision.From == decision.To {
		return contracts.EOMMStatusNoop
	}
	result := store.MovePool(ctx, contracts.RedisMoveRequest{
		Member: decision.Member,
		From:   decision.From,
		To:     decision.To,
		Score:  decision.Score,
	})
	return redisStatusToEOMMStatus(result.Status)
}

func (e *engine) ScoreBreakdown(input contracts.MatchCandidateContext, out *contracts.EOMMScoreBreakdown) contracts.EOMMStatus {
	if e == nil || out == nil || !validCandidate(input) {
		if out != nil {
			*out = contracts.EOMMScoreBreakdown{}
		}
		return contracts.EOMMStatusInvalidTicket
	}
	delta := input.Anchor.Trophies - input.Candidate.Ticket.Trophies
	if delta < 0 {
		delta = -delta
	}
	trophyPenalty := float32(delta) / float32(contracts.MatchMaxTolerance)
	if trophyPenalty > 1 {
		trophyPenalty = 1
	}
	similarity := cosine(input.Anchor.DeckVector, input.Candidate.Ticket.DeckVector)
	vectorDistance := 1 - similarity
	if vectorDistance < 0 {
		vectorDistance = 0
	}
	if vectorDistance > 2 {
		vectorDistance = 2
	}
	retention := float32(0)
	predicted := float32(0.5)
	if input.Anchor.ChurnRisk > e.config.HighChurnThreshold && input.Candidate.Ticket.Trophies <= input.Anchor.Trophies-contracts.EOMMRetentionTrophyOffset {
		retention += retentionBoost
		predicted = contracts.EOMMRetentionTargetWinP
	}
	if input.AnchorPool == contracts.RedisPoolMonetize && similarity < e.config.CounterSimilarityThreshold {
		retention += monetizeCounterPenalty
		predicted = contracts.EOMMMonetizeTargetWinP
	}
	totalPenalty := e.config.TrophyWeight*trophyPenalty + e.config.VectorWeight*vectorDistance + e.config.RetentionWeight*retention
	*out = contracts.EOMMScoreBreakdown{
		TrophyPenalty:     trophyPenalty,
		VectorDistance:    vectorDistance,
		RetentionModifier: retention,
		PredictedWinP:     predicted,
		Total:             -totalPenalty,
	}
	return contracts.EOMMStatusOK
}

func (e *engine) ScoreCandidate(input contracts.MatchCandidateContext, out *contracts.MatchCandidateScore) contracts.MatchCoreStatus {
	if out != nil {
		*out = contracts.MatchCandidateScore{Decision: contracts.MatchCandidateReject}
	}
	if out == nil || !validCandidate(input) {
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
	var breakdown contracts.EOMMScoreBreakdown
	if status := e.ScoreBreakdown(input, &breakdown); status != contracts.EOMMStatusOK {
		return contracts.MatchCoreStatusInvalidCandidate
	}
	*out = contracts.MatchCandidateScore{
		Fitness:         breakdown.Total,
		TrophyDelta:     delta,
		VectorDistance:  breakdown.VectorDistance,
		RetentionWeight: breakdown.RetentionModifier,
		PredictedWinP:   breakdown.PredictedWinP,
		Decision:        contracts.MatchCandidateReplaceBest,
	}
	return contracts.MatchCoreStatusOK
}

func (e *engine) RecordOutcome(input contracts.EOMMSpikeInput, state *contracts.EOMMSpikeState, out *contracts.EOMMChurnAlertEvent) contracts.EOMMStatus {
	if e == nil || state == nil || out == nil || input.PlayerID == 0 {
		return contracts.EOMMStatusInvalidTicket
	}
	*out = contracts.EOMMChurnAlertEvent{}
	if state.PlayerID == 0 {
		state.PlayerID = input.PlayerID
	}
	if state.PlayerID != input.PlayerID || state.Index >= e.config.SpikeWindowTicks || state.Count > e.config.SpikeWindowTicks {
		return contracts.EOMMStatusInvalidTicket
	}
	if state.Count == e.config.SpikeWindowTicks {
		evicted := state.Outcomes[state.Index]
		if evicted == 1 && state.Wins > 0 {
			state.Wins--
		}
	} else {
		state.Count++
	}
	sample := uint8(0)
	if input.Won {
		sample = 1
		state.Wins++
	}
	state.Outcomes[state.Index] = sample
	state.Index++
	if state.Index >= e.config.SpikeWindowTicks {
		state.Index = 0
	}
	if state.Count < e.config.SpikeWindowTicks {
		return contracts.EOMMStatusNoop
	}
	winRate := float32(state.Wins) / float32(state.Count)
	crossed := input.PreviousChurnRisk <= e.config.HighChurnThreshold && input.CurrentChurnRisk > e.config.HighChurnThreshold
	if winRate < e.config.SpikeWinRateThreshold && crossed {
		*out = contracts.EOMMChurnAlertEvent{
			PlayerID:          input.PlayerID,
			RollingWinRate:    winRate,
			PreviousChurnRisk: input.PreviousChurnRisk,
			CurrentChurnRisk:  input.CurrentChurnRisk,
			PoolTag:           contracts.PoolRetention,
		}
		return contracts.EOMMStatusOK
	}
	return contracts.EOMMStatusNoop
}

func validCandidate(input contracts.MatchCandidateContext) bool {
	return input.Anchor.PlayerID != 0 &&
		input.Candidate.Ticket.PlayerID != 0 &&
		input.Anchor.PlayerID != input.Candidate.Ticket.PlayerID &&
		validPool(input.AnchorPool) &&
		validPool(input.CandidatePool) &&
		input.ToleranceTrophies >= 0
}

func validPool(pool contracts.RedisQueuePool) bool {
	return pool <= contracts.RedisPoolMonetize
}

func validSegment(pool contracts.RedisQueuePool) bool {
	return pool <= contracts.RedisPoolSegment4
}

func cosine(a [8]float32, b [8]float32) float32 {
	var dot float32
	for i := 0; i < len(a); i++ {
		dot += a[i] * b[i]
	}
	if dot < -1 {
		return -1
	}
	if dot > 1 {
		return 1
	}
	return dot
}

func redisStatusToEOMMStatus(status contracts.RedisQueueStatus) contracts.EOMMStatus {
	switch status {
	case contracts.RedisStatusOK:
		return contracts.EOMMStatusOK
	case contracts.RedisStatusTimeout:
		return contracts.EOMMStatusRedisTimeout
	case contracts.RedisStatusCanceled:
		return contracts.EOMMStatusCanceled
	case contracts.RedisStatusUnavailable, contracts.RedisStatusNoScript, contracts.RedisStatusScriptNotLoaded, contracts.RedisStatusPartial:
		return contracts.EOMMStatusRedisUnavailable
	default:
		return contracts.EOMMStatusRedisUnavailable
	}
}

func finiteNonNegative(value float32) bool {
	return value >= 0 && !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

func finiteThreshold(value float32) bool {
	return value >= 0 && value <= 1 && !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

func abs32(value float32) float32 {
	if value < 0 {
		return -value
	}
	return value
}
