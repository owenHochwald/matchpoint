// Package simulation implements deterministic macro-simulation state helpers.
package simulation

import (
	"math"
	"os"
	"strconv"
	"time"

	"matchpoint/contracts"
)

const (
	SimDefaultConcurrentPlayers     = contracts.SimDefaultConcurrentPlayers
	SimDefaultDurationNanos         = contracts.SimDefaultDurationNanos
	SimDefaultMatchMeanNanos        = contracts.SimDefaultMatchMeanNanos
	SimDefaultMatchStdNanos         = contracts.SimDefaultMatchStdNanos
	SimDefaultMaxSessionLosses      = contracts.SimDefaultMaxSessionLosses
	SimDefaultTickRateNanos         = contracts.SimDefaultTickRateNanos
	SimDeckMutationProbabilityScale = contracts.SimDeckMutationProbabilityScale
	SimDeckMutationDelta            = contracts.SimDeckMutationDelta
	SimTiltLossDelta                = contracts.SimTiltLossDelta
	SimTiltWinDelta                 = contracts.SimTiltWinDelta
)

const (
	SimStatusOK              = contracts.SimStatusOK
	SimStatusNoop            = contracts.SimStatusNoop
	SimStatusInvalidConfig   = contracts.SimStatusInvalidConfig
	SimStatusInvalidState    = contracts.SimStatusInvalidState
	SimStatusDropped         = contracts.SimStatusDropped
	SimStatusConvergenceFail = contracts.SimStatusConvergenceFail
)

const (
	SimPhaseQueued    = contracts.SimPhaseQueued
	SimPhaseWaiting   = contracts.SimPhaseWaiting
	SimPhaseMatched   = contracts.SimPhaseMatched
	SimPhasePlaying   = contracts.SimPhasePlaying
	SimPhasePostMatch = contracts.SimPhasePostMatch
	SimPhaseQuit      = contracts.SimPhaseQuit
)

type SimStatus = contracts.SimStatus
type SimPhase = contracts.SimPhase
type SimConfig = contracts.SimConfig
type SimPlayerState = contracts.SimPlayerState
type SimTickInput = contracts.SimTickInput
type SimTickOutput = contracts.SimTickOutput
type SimConvergenceInput = contracts.SimConvergenceInput
type SimConvergenceResult = contracts.SimConvergenceResult
type SimMetrics = contracts.SimMetrics
type SimulationEngine = contracts.SimulationEngine

type engine struct {
	config contracts.SimConfig
}

func productionConfig() contracts.SimConfig {
	config := contracts.SimConfig{
		ConcurrentPlayers: contracts.SimDefaultConcurrentPlayers,
		DurationNanos:     contracts.SimDefaultDurationNanos,
		MatchMeanNanos:    contracts.SimDefaultMatchMeanNanos,
		MatchStdNanos:     contracts.SimDefaultMatchStdNanos,
		MaxSessionLosses:  contracts.SimDefaultMaxSessionLosses,
		TickRateNanos:     contracts.SimDefaultTickRateNanos,
	}
	if value, ok := envUint32("MP_SIM_CONCURRENCY"); ok {
		config.ConcurrentPlayers = value
	}
	if value, ok := envDurationNanos("MP_SIM_DURATION"); ok {
		config.DurationNanos = value
	}
	if value, ok := envDurationNanos("MP_MATCH_DURATION_MEAN"); ok {
		config.MatchMeanNanos = value
	}
	if value, ok := envDurationNanos("MP_MATCH_DURATION_STD"); ok {
		config.MatchStdNanos = value
	}
	if value, ok := envUint16("MP_MAX_SESSION_LOSSES"); ok {
		config.MaxSessionLosses = value
	}
	if value, ok := envDurationNanos("MP_TICK_RATE"); ok {
		config.TickRateNanos = value
	}
	return config
}

// NewEngine creates a deterministic simulation engine with defaults applied.
func NewEngine(config contracts.SimConfig) (contracts.SimulationEngine, contracts.SimStatus) {
	return newEngine(config)
}

func newEngine(config contracts.SimConfig) (*engine, contracts.SimStatus) {
	config = fillDefaults(config)
	if status := validateConfig(config); status != contracts.SimStatusOK {
		return nil, status
	}
	return &engine{config: config}, contracts.SimStatusOK
}

func fillDefaults(config contracts.SimConfig) contracts.SimConfig {
	defaults := productionConfig()
	if config.ConcurrentPlayers == 0 {
		config.ConcurrentPlayers = defaults.ConcurrentPlayers
	}
	if config.DurationNanos == 0 {
		config.DurationNanos = defaults.DurationNanos
	}
	if config.MatchMeanNanos == 0 {
		config.MatchMeanNanos = defaults.MatchMeanNanos
	}
	if config.MatchStdNanos == 0 {
		config.MatchStdNanos = defaults.MatchStdNanos
	}
	if config.MaxSessionLosses == 0 {
		config.MaxSessionLosses = defaults.MaxSessionLosses
	}
	if config.TickRateNanos == 0 {
		config.TickRateNanos = defaults.TickRateNanos
	}
	return config
}

func validateConfig(config contracts.SimConfig) contracts.SimStatus {
	if config.ConcurrentPlayers == 0 || config.DurationNanos <= 0 ||
		config.MatchMeanNanos <= 0 || config.MatchStdNanos < 0 ||
		config.MaxSessionLosses == 0 || config.TickRateNanos <= 0 {
		return contracts.SimStatusInvalidConfig
	}
	return contracts.SimStatusOK
}

func (e *engine) SeedPopulation(dst []contracts.SimPlayerState, seed uint64) contracts.SimStatus {
	if e == nil {
		return contracts.SimStatusInvalidState
	}
	if seed == 0 {
		seed = 1
	}
	for i := range dst {
		id := uint64(i) + 1
		trophies := seedTrophies(i, len(dst), seed)
		vector := profileVector(i, seed)
		dst[i] = contracts.SimPlayerState{
			Ticket: contracts.Ticket{
				PlayerID:      id,
				EnqueuedAt:    0,
				DeckVector:    vector,
				Trophies:      trophies,
				ChurnRisk:     0.1,
				MonetizationP: 0.1,
				PoolTag:       contracts.PoolMainstream,
			},
			TrophyFloor: tierFloor(trophies),
			Phase:       contracts.SimPhaseQueued,
		}
	}
	return contracts.SimStatusOK
}

func (e *engine) SimPlayerTick(input contracts.SimTickInput, state *contracts.SimPlayerState, out *contracts.SimTickOutput) contracts.SimStatus {
	if e == nil || state == nil || out == nil || state.Ticket.PlayerID == 0 {
		return contracts.SimStatusInvalidState
	}
	*out = contracts.SimTickOutput{}
	switch state.Phase {
	case contracts.SimPhaseQueued:
		state.Ticket.EnqueuedAt = input.NowUnixNano
		state.Phase = contracts.SimPhaseWaiting
		out.PublishedTicket = true
	case contracts.SimPhaseWaiting:
		if !input.HasResult {
			out.Phase = state.Phase
			return contracts.SimStatusNoop
		}
		state.LastMatchID = input.Result.MatchID
		state.LastPredictedWinP = clamp01(input.Result.PredictedWinP)
		state.Phase = contracts.SimPhaseMatched
	case contracts.SimPhaseMatched:
		duration := e.matchDuration(input.OutcomeRoll)
		state.MatchEndsAt = input.NowUnixNano + duration
		state.Phase = contracts.SimPhasePlaying
		out.MatchDurationNanos = duration
	case contracts.SimPhasePlaying:
		if input.NowUnixNano < state.MatchEndsAt {
			out.Phase = state.Phase
			return contracts.SimStatusNoop
		}
		state.LastWon = clamp01(input.OutcomeRoll) <= state.LastPredictedWinP
		state.Phase = contracts.SimPhasePostMatch
		out.CompletedMatch = true
	case contracts.SimPhasePostMatch:
		out.MutatedDeck = e.applyPostMatch(input, state)
		out.QuitSession = state.Phase == contracts.SimPhaseQuit
	case contracts.SimPhaseQuit:
		out.Phase = state.Phase
		return contracts.SimStatusNoop
	default:
		return contracts.SimStatusInvalidState
	}
	out.Phase = state.Phase
	return contracts.SimStatusOK
}

func (e *engine) DeliverResult(mailbox chan<- contracts.MatchResult, result contracts.MatchResult, metrics contracts.SimMetrics) contracts.SimStatus {
	if e == nil || mailbox == nil {
		return contracts.SimStatusInvalidState
	}
	select {
	case mailbox <- result:
		return contracts.SimStatusOK
	default:
		if metrics != nil {
			metrics.RecordSimDrop(result.PlayerA)
		}
		return contracts.SimStatusDropped
	}
}

func (e *engine) CheckConvergence(input contracts.SimConvergenceInput, out *contracts.SimConvergenceResult) contracts.SimStatus {
	if e == nil || out == nil {
		return contracts.SimStatusInvalidState
	}
	*out = contracts.SimConvergenceResult{}
	if input.WarmupElapsedNanos < 60_000_000_000 {
		out.FirstFailedGate = 1
		return contracts.SimStatusConvergenceFail
	}
	for i := 0; i < len(input.SegmentDepths); i++ {
		if input.SegmentDepths[i] < 50 {
			out.FirstFailedGate = 2
			return contracts.SimStatusConvergenceFail
		}
	}
	if input.LoserPoolDepth < 100 {
		out.FirstFailedGate = 3
		return contracts.SimStatusConvergenceFail
	}
	if input.RetentionPoolDepth < 200 {
		out.FirstFailedGate = 4
		return contracts.SimStatusConvergenceFail
	}
	if input.ConsecutiveNonZeroMatchTicks < 30 {
		out.FirstFailedGate = 5
		return contracts.SimStatusConvergenceFail
	}
	if input.StableHeapTicks < 60 {
		out.FirstFailedGate = 6
		return contracts.SimStatusConvergenceFail
	}
	out.Converged = true
	return contracts.SimStatusOK
}

func (e *engine) matchDuration(sample float32) int64 {
	normalized := (clamp01(sample) * 2) - 1
	duration := e.config.MatchMeanNanos + int64(float32(e.config.MatchStdNanos)*normalized)
	if duration < 0 {
		return 0
	}
	return duration
}

func (e *engine) applyPostMatch(input contracts.SimTickInput, state *contracts.SimPlayerState) bool {
	mutated := false
	if state.LastWon {
		state.SessionWins++
		if state.TiltFactor > contracts.SimTiltWinDelta {
			state.TiltFactor -= contracts.SimTiltWinDelta
		} else {
			state.TiltFactor = 0
		}
		state.Ticket.ConsecWins++
		state.Ticket.ConsecLosses = 0
		state.Ticket.Trophies = clampTrophies(state.Ticket.Trophies + 30)
	} else {
		state.SessionLosses++
		state.TiltFactor = clamp01(state.TiltFactor + contracts.SimTiltLossDelta)
		state.Ticket.ConsecLosses--
		state.Ticket.ConsecWins = 0
		state.Ticket.Trophies = clampTrophies(state.Ticket.Trophies - 20)
		if state.Ticket.Trophies < state.TrophyFloor {
			state.Ticket.Trophies = state.TrophyFloor
		}
	}
	if floor := tierFloor(state.Ticket.Trophies); floor > state.TrophyFloor {
		state.TrophyFloor = floor
	}
	state.Ticket.ChurnRisk = clamp01(0.7*state.Ticket.ChurnRisk + 0.3*state.TiltFactor)
	if input.MutationRoll < state.TiltFactor*contracts.SimDeckMutationProbabilityScale {
		mutated = mutateDeck(input, state)
	}
	if state.SessionLosses >= e.config.MaxSessionLosses || input.OutcomeRoll < state.Ticket.ChurnRisk {
		state.Phase = contracts.SimPhaseQuit
		return mutated
	}
	state.Phase = contracts.SimPhaseQueued
	return mutated
}

func mutateDeck(input contracts.SimTickInput, state *contracts.SimPlayerState) bool {
	dim := int(input.MutationDim % contracts.VectorDimensionCount)
	v := state.Ticket.DeckVector
	if input.MutationSign < 0 {
		v[dim] -= contracts.SimDeckMutationDelta
		if v[dim] < 0 {
			v[dim] = 0
		}
	} else {
		v[dim] += contracts.SimDeckMutationDelta
	}
	if !normalize(&v) {
		v = [8]float32{}
		v[dim] = 1
	}
	state.Ticket.DeckVector = v
	state.LastMutatedAt = input.NowUnixNano
	return true
}

func normalize(v *[8]float32) bool {
	var sum float32
	for i := 0; i < len(v); i++ {
		sum += v[i] * v[i]
	}
	if sum < contracts.VectorZeroThreshold*contracts.VectorZeroThreshold {
		return false
	}
	inv := float32(1.0 / math.Sqrt(float64(sum)))
	for i := 0; i < len(v); i++ {
		v[i] *= inv
	}
	return true
}

func seedTrophies(index int, total int, seed uint64) int32 {
	if total <= 0 {
		return 0
	}
	pct := (index * 100) / total
	var min, max, mean int32
	switch {
	case pct < 15:
		min, max, mean = 0, 999, 500
	case pct < 60:
		min, max, mean = 1000, 2999, 1800
	case pct < 85:
		min, max, mean = 3000, 5999, 4200
	case pct < 95:
		min, max, mean = 6000, 8999, 7200
	case pct < 99:
		min, max, mean = 9000, 11999, 10200
	default:
		min, max, mean = 12000, 15000, 12800
	}
	offset := int32((mix(seed+uint64(index)) % 801)) - 400
	trophies := mean + offset
	if trophies < min {
		return min
	}
	if trophies > max {
		return max
	}
	return trophies
}

func profileVector(index int, seed uint64) [8]float32 {
	profile := int(mix(seed+uint64(index*17)) % 100)
	var v [8]float32
	switch {
	case profile < 18:
		v[0], v[5] = 1, 1
	case profile < 34:
		v[2], v[7] = 1, 1
	case profile < 48:
		v[3], v[4] = 1, 1
	case profile < 60:
		v[6], v[0] = 1, 1
	case profile < 72:
		v[5], v[1] = 1, 1
	case profile < 82:
		v[1], v[3] = 1, 1
	case profile < 92:
		v[2], v[5] = 1, 1
	default:
		v[0], v[3], v[5], v[7] = 1, 1, 1, 1
	}
	normalize(&v)
	return v
}

func tierFloor(trophies int32) int32 {
	switch {
	case trophies >= 12000:
		return 12000
	case trophies >= 9000:
		return 9000
	case trophies >= 6000:
		return 6000
	case trophies >= 3000:
		return 3000
	case trophies >= 1000:
		return 1000
	default:
		return 0
	}
}

func clampTrophies(v int32) int32 {
	if v < 0 {
		return 0
	}
	if v > 100000 {
		return 100000
	}
	return v
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

func envUint32(key string) (uint32, bool) {
	raw := os.Getenv(key)
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(value), true
}

func envUint16(key string) (uint16, bool) {
	raw := os.Getenv(key)
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseUint(raw, 10, 16)
	if err != nil {
		return 0, false
	}
	return uint16(value), true
}

func envDurationNanos(key string) (int64, bool) {
	raw := os.Getenv(key)
	if raw == "" {
		return 0, false
	}
	if duration, err := time.ParseDuration(raw); err == nil {
		return int64(duration), true
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func mix(v uint64) uint64 {
	v ^= v >> 30
	v *= 0xbf58476d1ce4e5b9
	v ^= v >> 27
	v *= 0x94d049bb133111eb
	v ^= v >> 31
	return v
}
