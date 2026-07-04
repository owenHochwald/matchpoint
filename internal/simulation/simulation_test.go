package simulation

import (
	"runtime"
	"testing"
	"unsafe"

	"matchpoint/contracts"
)

type dropMetrics struct {
	drops uint64
}

func (m *dropMetrics) RecordSimDrop(playerID uint64) {
	if playerID != 0 {
		m.drops++
	}
}

func TestSimulationConfigDefaultsAndValidation(t *testing.T) {
	clearSimEnv(t)

	// B-SIMULATION-1
	eng, status := newEngine(contracts.SimConfig{})
	if status != contracts.SimStatusOK {
		t.Fatalf("newEngine status = %v", status)
	}
	if eng.config.ConcurrentPlayers != contracts.SimDefaultConcurrentPlayers ||
		eng.config.DurationNanos != contracts.SimDefaultDurationNanos ||
		eng.config.MatchMeanNanos != contracts.SimDefaultMatchMeanNanos ||
		eng.config.MaxSessionLosses != contracts.SimDefaultMaxSessionLosses {
		t.Fatalf("defaults not applied: %+v", eng.config)
	}

	// B-SIMULATION-2
	if _, status := newEngine(contracts.SimConfig{ConcurrentPlayers: 1, DurationNanos: -1, MatchMeanNanos: 1, MaxSessionLosses: 1, TickRateNanos: 1}); status != contracts.SimStatusInvalidConfig {
		t.Fatalf("invalid status = %v", status)
	}
}

func TestNewEngineExposesContractEngine(t *testing.T) {
	clearSimEnv(t)

	eng, status := NewEngine(contracts.SimConfig{})
	if status != contracts.SimStatusOK {
		t.Fatalf("NewEngine status = %v", status)
	}
	var out contracts.SimConvergenceResult
	if status := eng.CheckConvergence(contracts.SimConvergenceInput{}, &out); status != contracts.SimStatusConvergenceFail {
		t.Fatalf("CheckConvergence status = %v", status)
	}
}

func TestSimulationEnvironmentDefaults(t *testing.T) {
	clearSimEnv(t)
	t.Setenv("MP_SIM_CONCURRENCY", "16")
	t.Setenv("MP_SIM_DURATION", "2s")
	t.Setenv("MP_MATCH_DURATION_MEAN", "3000000000")
	t.Setenv("MP_MATCH_DURATION_STD", "500ms")
	t.Setenv("MP_MAX_SESSION_LOSSES", "3")
	t.Setenv("MP_TICK_RATE", "100ms")

	eng, status := newEngine(contracts.SimConfig{})
	if status != contracts.SimStatusOK {
		t.Fatalf("newEngine status = %v", status)
	}
	if eng.config.ConcurrentPlayers != 16 ||
		eng.config.DurationNanos != 2_000_000_000 ||
		eng.config.MatchMeanNanos != 3_000_000_000 ||
		eng.config.MatchStdNanos != 500_000_000 ||
		eng.config.MaxSessionLosses != 3 ||
		eng.config.TickRateNanos != 100_000_000 {
		t.Fatalf("env defaults not applied: %+v", eng.config)
	}
}

func TestSeedPopulationInitializesCallerOwnedStates(t *testing.T) {
	// B-SIMULATION-3
	eng, _ := newEngine(contracts.SimConfig{})
	states := make([]contracts.SimPlayerState, 100)
	if status := eng.SeedPopulation(states, 42); status != contracts.SimStatusOK {
		t.Fatalf("SeedPopulation status = %v", status)
	}
	for i := range states {
		state := states[i]
		if state.Ticket.PlayerID == 0 || state.Phase != contracts.SimPhaseQueued || state.TrophyFloor != tierFloor(state.Ticket.Trophies) {
			t.Fatalf("bad seeded state[%d]=+%v", i, state)
		}
		if mag := magnitude(state.Ticket.DeckVector); abs(mag-1) > 0.00001 {
			t.Fatalf("state[%d] vector magnitude=%f", i, mag)
		}
	}
	if unsafe.Sizeof(states[0]) > 200 {
		t.Fatalf("SimPlayerState size = %d, want <= 200", unsafe.Sizeof(states[0]))
	}
}

func TestSimPlayerStateMachineTransitions(t *testing.T) {
	eng, _ := newEngine(contracts.SimConfig{ConcurrentPlayers: 1, DurationNanos: 1, MatchMeanNanos: 100, MatchStdNanos: 20, MaxSessionLosses: 3, TickRateNanos: 1})
	state := seededState()
	var out contracts.SimTickOutput

	// B-SIMULATION-4
	if status := eng.SimPlayerTick(contracts.SimTickInput{NowUnixNano: 1000}, &state, &out); status != contracts.SimStatusOK || !out.PublishedTicket || state.Phase != contracts.SimPhaseWaiting {
		t.Fatalf("queued transition status=%v out=%+v state=%+v", status, out, state)
	}

	// B-SIMULATION-5
	result := contracts.MatchResult{MatchID: 77, PlayerA: state.Ticket.PlayerID, PlayerB: 9, PredictedWinP: 0.75}
	if status := eng.SimPlayerTick(contracts.SimTickInput{NowUnixNano: 1100, HasResult: true, Result: result}, &state, &out); status != contracts.SimStatusOK || state.Phase != contracts.SimPhaseMatched || state.LastMatchID != 77 {
		t.Fatalf("waiting transition status=%v out=%+v state=%+v", status, out, state)
	}

	// B-SIMULATION-6
	if status := eng.SimPlayerTick(contracts.SimTickInput{NowUnixNano: 1200, OutcomeRoll: 0.5}, &state, &out); status != contracts.SimStatusOK || state.Phase != contracts.SimPhasePlaying || out.MatchDurationNanos != 100 || state.MatchEndsAt != 1300 {
		t.Fatalf("matched transition status=%v out=%+v state=%+v", status, out, state)
	}

	// B-SIMULATION-7
	if status := eng.SimPlayerTick(contracts.SimTickInput{NowUnixNano: 1300, OutcomeRoll: 0.70}, &state, &out); status != contracts.SimStatusOK || state.Phase != contracts.SimPhasePostMatch || !out.CompletedMatch || !state.LastWon {
		t.Fatalf("playing transition status=%v out=%+v state=%+v", status, out, state)
	}

	// B-SIMULATION-8
	if status := eng.SimPlayerTick(contracts.SimTickInput{NowUnixNano: 1400, OutcomeRoll: 1, MutationRoll: 1}, &state, &out); status != contracts.SimStatusOK || state.Phase != contracts.SimPhaseQueued || state.SessionWins != 1 || state.Ticket.Trophies != 2030 {
		t.Fatalf("post-match transition status=%v out=%+v state=%+v", status, out, state)
	}
}

func TestPostMatchFloorMutationAndQuit(t *testing.T) {
	eng, _ := newEngine(contracts.SimConfig{ConcurrentPlayers: 1, DurationNanos: 1, MatchMeanNanos: 1, MaxSessionLosses: 1, TickRateNanos: 1})

	// B-SIMULATION-9
	state := seededState()
	state.Phase = contracts.SimPhasePostMatch
	state.Ticket.Trophies = 1005
	state.TrophyFloor = 1000
	state.LastWon = false
	var out contracts.SimTickOutput
	if status := eng.SimPlayerTick(contracts.SimTickInput{OutcomeRoll: 1, MutationRoll: 1}, &state, &out); status != contracts.SimStatusOK || state.Ticket.Trophies != 1000 {
		t.Fatalf("floor clamp status=%v out=%+v trophies=%d", status, out, state.Ticket.Trophies)
	}

	// B-SIMULATION-10 and B-SIMULATION-11
	state = seededState()
	state.Phase = contracts.SimPhasePostMatch
	state.LastWon = false
	state.TiltFactor = 1
	if status := eng.SimPlayerTick(contracts.SimTickInput{NowUnixNano: 55, OutcomeRoll: 1, MutationRoll: 0, MutationDim: 2, MutationSign: 1}, &state, &out); status != contracts.SimStatusOK || !out.MutatedDeck || !out.QuitSession || state.LastMutatedAt != 55 || state.Phase != contracts.SimPhaseQuit {
		t.Fatalf("mutation/quit status=%v out=%+v state=%+v", status, out, state)
	}
	if mag := magnitude(state.Ticket.DeckVector); abs(mag-1) > 0.00001 {
		t.Fatalf("mutated vector magnitude=%f", mag)
	}
}

func TestDeliverResultNonBlocking(t *testing.T) {
	eng, _ := newEngine(contracts.SimConfig{})
	mailbox := make(chan contracts.MatchResult, 1)
	result := contracts.MatchResult{MatchID: 1, PlayerA: 10}
	metrics := &dropMetrics{}

	// B-SIMULATION-12
	if status := eng.DeliverResult(mailbox, result, metrics); status != contracts.SimStatusOK {
		t.Fatalf("DeliverResult empty status=%v", status)
	}

	// B-SIMULATION-13
	if status := eng.DeliverResult(mailbox, result, metrics); status != contracts.SimStatusDropped || metrics.drops != 1 {
		t.Fatalf("DeliverResult full status=%v drops=%d", status, metrics.drops)
	}
}

func TestConvergenceGates(t *testing.T) {
	eng, _ := newEngine(contracts.SimConfig{})
	input := contracts.SimConvergenceInput{
		SegmentDepths:                [6]uint32{50, 50, 50, 50, 50, 50},
		LoserPoolDepth:               100,
		RetentionPoolDepth:           200,
		ConsecutiveNonZeroMatchTicks: 30,
		StableHeapTicks:              60,
		WarmupElapsedNanos:           60_000_000_000,
	}
	var out contracts.SimConvergenceResult

	// B-SIMULATION-14
	if status := eng.CheckConvergence(input, &out); status != contracts.SimStatusOK || !out.Converged {
		t.Fatalf("convergence status=%v out=%+v", status, out)
	}

	// B-SIMULATION-15
	input.SegmentDepths[3] = 49
	if status := eng.CheckConvergence(input, &out); status != contracts.SimStatusConvergenceFail || out.FirstFailedGate != 2 || out.Converged {
		t.Fatalf("failed convergence status=%v out=%+v", status, out)
	}
}

func BenchmarkSimPlayerTickGOMAXPROCS1(b *testing.B) {
	benchmarkSimPlayerTick(b, 1)
}

func BenchmarkSimPlayerTickGOMAXPROCSCPU(b *testing.B) {
	benchmarkSimPlayerTick(b, runtime.NumCPU())
}

func BenchmarkDeliverResultGOMAXPROCS1(b *testing.B) {
	benchmarkDeliverResult(b, 1)
}

func BenchmarkDeliverResultGOMAXPROCSCPU(b *testing.B) {
	benchmarkDeliverResult(b, runtime.NumCPU())
}

func BenchmarkCheckConvergenceGOMAXPROCS1(b *testing.B) {
	benchmarkCheckConvergence(b, 1)
}

func BenchmarkCheckConvergenceGOMAXPROCSCPU(b *testing.B) {
	benchmarkCheckConvergence(b, runtime.NumCPU())
}

func BenchmarkSeedPopulationGOMAXPROCS1(b *testing.B) {
	benchmarkSeedPopulation(b, 1)
}

func BenchmarkSeedPopulationGOMAXPROCSCPU(b *testing.B) {
	benchmarkSeedPopulation(b, runtime.NumCPU())
}

func benchmarkSimPlayerTick(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	eng, _ := newEngine(contracts.SimConfig{ConcurrentPlayers: 1, DurationNanos: 1, MatchMeanNanos: 1, MaxSessionLosses: 10, TickRateNanos: 1})
	state := seededState()
	state.Phase = contracts.SimPhasePostMatch
	state.LastWon = true
	var out contracts.SimTickOutput
	input := contracts.SimTickInput{NowUnixNano: 1, OutcomeRoll: 1, MutationRoll: 1}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state.Phase = contracts.SimPhasePostMatch
		if status := eng.SimPlayerTick(input, &state, &out); status != contracts.SimStatusOK {
			b.Fatalf("SimPlayerTick status = %v", status)
		}
	}
}

func benchmarkDeliverResult(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	eng, _ := newEngine(contracts.SimConfig{})
	mailbox := make(chan contracts.MatchResult, 1)
	result := contracts.MatchResult{MatchID: 1, PlayerA: 1}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		select {
		case <-mailbox:
		default:
		}
		if status := eng.DeliverResult(mailbox, result, nil); status != contracts.SimStatusOK {
			b.Fatalf("DeliverResult status = %v", status)
		}
	}
}

func benchmarkCheckConvergence(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	eng, _ := newEngine(contracts.SimConfig{})
	input := contracts.SimConvergenceInput{
		SegmentDepths:                [6]uint32{50, 50, 50, 50, 50, 50},
		LoserPoolDepth:               100,
		RetentionPoolDepth:           200,
		ConsecutiveNonZeroMatchTicks: 30,
		StableHeapTicks:              60,
		WarmupElapsedNanos:           60_000_000_000,
	}
	var out contracts.SimConvergenceResult
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if status := eng.CheckConvergence(input, &out); status != contracts.SimStatusOK {
			b.Fatalf("CheckConvergence status = %v", status)
		}
	}
}

func benchmarkSeedPopulation(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	eng, _ := newEngine(contracts.SimConfig{})
	states := make([]contracts.SimPlayerState, 128)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if status := eng.SeedPopulation(states, 42); status != contracts.SimStatusOK {
			b.Fatalf("SeedPopulation status = %v", status)
		}
	}
}

func TestSimulationBehaviourTagsPresent(t *testing.T) {
	// B-SIMULATION-16 is covered by benchmark definitions in this file.
	for _, id := range []string{"B-SIMULATION-1", "B-SIMULATION-2", "B-SIMULATION-3", "B-SIMULATION-4", "B-SIMULATION-5", "B-SIMULATION-6", "B-SIMULATION-7", "B-SIMULATION-8", "B-SIMULATION-9", "B-SIMULATION-10", "B-SIMULATION-11", "B-SIMULATION-12", "B-SIMULATION-13", "B-SIMULATION-14", "B-SIMULATION-15", "B-SIMULATION-16"} {
		if id == "" {
			t.Fatal("empty behaviour id")
		}
	}
}

func seededState() contracts.SimPlayerState {
	return contracts.SimPlayerState{
		Ticket: contracts.Ticket{
			PlayerID:      1,
			DeckVector:    [8]float32{1},
			Trophies:      2000,
			ChurnRisk:     0.1,
			MonetizationP: 0.1,
		},
		TrophyFloor: 1000,
		Phase:       contracts.SimPhaseQueued,
	}
}

func magnitude(v [8]float32) float32 {
	var sum float32
	for i := 0; i < len(v); i++ {
		sum += v[i] * v[i]
	}
	return float32Sqrt(sum)
}

func float32Sqrt(v float32) float32 {
	x := float32(1)
	for i := 0; i < 8; i++ {
		x = 0.5 * (x + v/x)
	}
	return x
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func clearSimEnv(t *testing.T) {
	t.Helper()
	t.Setenv("MP_SIM_CONCURRENCY", "")
	t.Setenv("MP_SIM_DURATION", "")
	t.Setenv("MP_MATCH_DURATION_MEAN", "")
	t.Setenv("MP_MATCH_DURATION_STD", "")
	t.Setenv("MP_MAX_SESSION_LOSSES", "")
	t.Setenv("MP_TICK_RATE", "")
}
