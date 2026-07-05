package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"matchpoint/contracts"
	"matchpoint/internal/simulation"
)

type counters struct {
	queued    atomic.Uint64
	completed atomic.Uint64
	mutated   atomic.Uint64
	quit      atomic.Uint64
}

func main() {
	var (
		players = flag.Uint("players", uint(contracts.SimDefaultConcurrentPlayers), "simulated player goroutines")
		rounds  = flag.Uint("rounds", 16, "post-match rounds per player")
		seed    = flag.Uint64("seed", 42, "population seed")
	)
	flag.Parse()

	engine, status := simulation.NewEngine(contracts.SimConfig{ConcurrentPlayers: uint32(*players)})
	if status != contracts.SimStatusOK {
		fmt.Fprintf(os.Stderr, "simulation init failed: %d\n", status)
		os.Exit(1)
	}

	states := make([]contracts.SimPlayerState, int(*players))
	if status := engine.SeedPopulation(states, *seed); status != contracts.SimStatusOK {
		fmt.Fprintf(os.Stderr, "seed population failed: %d\n", status)
		os.Exit(1)
	}

	start := time.Now()
	var metrics counters
	var wg sync.WaitGroup
	wg.Add(len(states))
	for i := range states {
		i := i
		go func() {
			defer wg.Done()
			runPlayer(engine, &states[i], uint32(*rounds), uint64(i)+*seed, &metrics)
		}()
	}
	wg.Wait()

	input := contracts.SimConvergenceInput{
		SegmentDepths:                segmentDepths(states),
		LoserPoolDepth:               100,
		RetentionPoolDepth:           200,
		ConsecutiveNonZeroMatchTicks: uint32(minUint64(metrics.completed.Load(), 30)),
		StableHeapTicks:              60,
		WarmupElapsedNanos:           60_000_000_000,
	}
	var convergence contracts.SimConvergenceResult
	convergenceStatus := engine.CheckConvergence(input, &convergence)

	fmt.Printf("players=%d rounds=%d goroutines=%d elapsed=%s\n", *players, *rounds, runtime.NumGoroutine(), time.Since(start).Round(time.Millisecond))
	fmt.Printf("queued=%d completed=%d mutated=%d quit=%d\n", metrics.queued.Load(), metrics.completed.Load(), metrics.mutated.Load(), metrics.quit.Load())
	fmt.Printf("convergence_status=%d converged=%t failed_gate=%d segment_depths=%v\n", convergenceStatus, convergence.Converged, convergence.FirstFailedGate, input.SegmentDepths)
}

func runPlayer(engine contracts.SimulationEngine, state *contracts.SimPlayerState, rounds uint32, seed uint64, metrics *counters) {
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
			PredictedWinP: predictedWin(seed, round),
			AssignedAt:    now + 1,
		}
		_ = engine.SimPlayerTick(contracts.SimTickInput{NowUnixNano: now + 1, HasResult: true, Result: result}, state, &out)
		_ = engine.SimPlayerTick(contracts.SimTickInput{NowUnixNano: now + 2, OutcomeRoll: roll(seed, round, 3)}, state, &out)
		_ = engine.SimPlayerTick(contracts.SimTickInput{NowUnixNano: state.MatchEndsAt, OutcomeRoll: roll(seed, round, 5)}, state, &out)
		if status := engine.SimPlayerTick(contracts.SimTickInput{
			NowUnixNano:  state.MatchEndsAt + 1,
			OutcomeRoll:  roll(seed, round, 7),
			MutationRoll: roll(seed, round, 11),
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

func predictedWin(seed uint64, round uint32) float32 {
	return 0.35 + 0.3*roll(seed, round, 13)
}

func roll(seed uint64, round uint32, salt uint64) float32 {
	x := seed + uint64(round)*0x9e3779b97f4a7c15 + salt
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return float32(x%10_000) / 10_000
}

func segmentDepths(states []contracts.SimPlayerState) [6]uint32 {
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
