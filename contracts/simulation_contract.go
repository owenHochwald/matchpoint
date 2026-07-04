// Package contracts defines the Planner-owned public contract for MatchPoint
// modules. This file is intentionally declarative: no implementation logic
// belongs here.
package contracts

// Simulation defaults from FEATURES.md section 7.5.
const (
	SimDefaultConcurrentPlayers     uint32  = 100_000
	SimDefaultDurationNanos         int64   = 600_000_000_000
	SimDefaultMatchMeanNanos        int64   = 180_000_000_000
	SimDefaultMatchStdNanos         int64   = 30_000_000_000
	SimDefaultMaxSessionLosses      uint16  = 10
	SimDefaultTickRateNanos         int64   = MatchTickIntervalNanos
	SimDeckMutationProbabilityScale float32 = 0.3
	SimDeckMutationDelta            float32 = 0.1
	SimTiltLossDelta                float32 = 0.15
	SimTiltWinDelta                 float32 = 0.10
)

// SimStatus is the stable non-allocating status taxonomy for simulation.
type SimStatus uint8

const (
	// SimStatusOK means the operation completed successfully.
	SimStatusOK SimStatus = 0
	// SimStatusNoop means the state did not need to advance.
	SimStatusNoop SimStatus = 1
	// SimStatusInvalidConfig means immutable config is not legal.
	SimStatusInvalidConfig SimStatus = 2
	// SimStatusInvalidState means a player state is malformed.
	SimStatusInvalidState SimStatus = 3
	// SimStatusDropped means non-blocking result delivery found a full mailbox.
	SimStatusDropped SimStatus = 4
	// SimStatusConvergenceFail means the warm-up convergence gates are not met.
	SimStatusConvergenceFail SimStatus = 5
)

// SimPhase identifies one player state-machine phase.
type SimPhase uint8

const (
	// SimPhaseQueued means the player is ready to publish a queue ticket.
	SimPhaseQueued SimPhase = 0
	// SimPhaseWaiting means the player is waiting for a match assignment.
	SimPhaseWaiting SimPhase = 1
	// SimPhaseMatched means an assignment has been received.
	SimPhaseMatched SimPhase = 2
	// SimPhasePlaying means the simulated match is in progress.
	SimPhasePlaying SimPhase = 3
	// SimPhasePostMatch means outcome mutation should run.
	SimPhasePostMatch SimPhase = 4
	// SimPhaseQuit means the player has churned out of the session.
	SimPhaseQuit SimPhase = 5
)

// SimConfig defines immutable simulation parameters.
type SimConfig struct {
	// ConcurrentPlayers defaults to SimDefaultConcurrentPlayers.
	ConcurrentPlayers uint32
	// DurationNanos defaults to SimDefaultDurationNanos.
	DurationNanos int64
	// MatchMeanNanos defaults to SimDefaultMatchMeanNanos.
	MatchMeanNanos int64
	// MatchStdNanos defaults to SimDefaultMatchStdNanos.
	MatchStdNanos int64
	// MaxSessionLosses defaults to SimDefaultMaxSessionLosses.
	MaxSessionLosses uint16
	// TickRateNanos defaults to SimDefaultTickRateNanos.
	TickRateNanos int64
}

// SimPlayerState is the hot per-player state owned by one simulation goroutine.
type SimPlayerState struct {
	// Ticket embeds the current queue ticket for in-place mutation.
	Ticket Ticket
	// LastMatchID is the current or most recent assignment.
	LastMatchID uint64
	// LastMutatedAt is Unix nanoseconds of the latest deck mutation.
	LastMutatedAt int64
	// MatchEndsAt is Unix nanoseconds when PLAYING may advance to POST_MATCH.
	MatchEndsAt int64
	// TrophyFloor is the highest reached tier floor.
	TrophyFloor int32
	// TiltFactor is clamped to [0, 1].
	TiltFactor float32
	// LastPredictedWinP is copied from the assignment for outcome resolution.
	LastPredictedWinP float32
	// SessionWins counts wins in the current session.
	SessionWins uint16
	// SessionLosses counts losses in the current session.
	SessionLosses uint16
	// Phase is the current state-machine phase.
	Phase SimPhase
	// LastWon is the resolved outcome waiting for POST_MATCH mutation.
	LastWon bool
}

// SimTickInput describes one deterministic player-state tick.
type SimTickInput struct {
	// NowUnixNano is the simulation clock.
	NowUnixNano int64
	// Result is consumed only when HasResult is true in WAITING.
	Result MatchResult
	// HasResult reports whether Result is available.
	HasResult bool
	// OutcomeRoll is in [0, 1] and is compared with predicted win probability.
	OutcomeRoll float32
	// MutationRoll is in [0, 1] and is compared with TiltFactor*0.3.
	MutationRoll float32
	// MutationDim selects the deck vector dimension to shift.
	MutationDim uint8
	// MutationSign selects +0.1 when non-negative and -0.1 when negative.
	MutationSign int8
}

// SimTickOutput describes one observed state transition.
type SimTickOutput struct {
	// PublishedTicket is true for QUEUED -> WAITING.
	PublishedTicket bool
	// CompletedMatch is true for PLAYING -> POST_MATCH.
	CompletedMatch bool
	// MutatedDeck is true when POST_MATCH deck mutation changed the vector.
	MutatedDeck bool
	// QuitSession is true for POST_MATCH -> QUIT.
	QuitSession bool
	// Phase is the resulting phase.
	Phase SimPhase
	// MatchDurationNanos is the deterministic duration selected at MATCHED.
	MatchDurationNanos int64
}

// SimConvergenceInput contains the acceptance gates checked after warm-up.
type SimConvergenceInput struct {
	// SegmentDepths contains the six trophy-tier depths used by MATCH_SPEC.
	SegmentDepths [6]uint32
	// LoserPoolDepth must be at least 100.
	LoserPoolDepth uint32
	// RetentionPoolDepth must be at least 200.
	RetentionPoolDepth uint32
	// ConsecutiveNonZeroMatchTicks must be at least 30.
	ConsecutiveNonZeroMatchTicks uint32
	// StableHeapTicks must be at least 60.
	StableHeapTicks uint32
	// WarmupElapsedNanos must be at least 60 seconds.
	WarmupElapsedNanos int64
}

// SimConvergenceResult reports whether macro-simulation data is meaningful.
type SimConvergenceResult struct {
	// Converged is true only when all gates pass.
	Converged bool
	// FirstFailedGate is zero when converged, otherwise a stable gate number.
	FirstFailedGate uint8
}

// SimMetrics records non-blocking delivery drops without coupling to telemetry.
type SimMetrics interface {
	// RecordSimDrop records a full player result mailbox.
	RecordSimDrop(playerID uint64)
}

// SimulationEngine owns deterministic state transitions and population seeding.
type SimulationEngine interface {
	// SeedPopulation fills caller-owned player state storage with deterministic defaults.
	SeedPopulation(dst []SimPlayerState, seed uint64) SimStatus
	// SimPlayerTick advances one player state without allocation or timers.
	SimPlayerTick(input SimTickInput, state *SimPlayerState, out *SimTickOutput) SimStatus
	// DeliverResult performs a non-blocking send to a capacity-one player mailbox.
	DeliverResult(mailbox chan<- MatchResult, result MatchResult, metrics SimMetrics) SimStatus
	// CheckConvergence validates macro-simulation warm-up gates.
	CheckConvergence(input SimConvergenceInput, out *SimConvergenceResult) SimStatus
}
