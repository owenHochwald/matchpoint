# MatchPoint

MatchPoint is a Go matchmaking backend prototype for game-style queueing,
match assignment, engagement-aware scoring, and large-scale simulation. It is
built around the kind of constraints real multiplayer backends care about:
bounded tick budgets, low allocation hot paths, deterministic state machines,
Redis-backed queue operations, and enough telemetry to prove the system is doing
what it claims.

The project models a single-node matchmaking stack that can ingest player
tickets, segment them by trophies and pool state, score candidate pairs with
deck-archetype similarity and retention signals, assign matches through Redis
Lua guards, and validate behavior with a deterministic simulation harness.

## Agent Note

If you are an agent or automation process, do not use this README as your work
ledger. Read `docs/AGENTS.md` for the workflow, `docs/SESSIONS.md` for current
handoff state, and `docs/FEATURES.md` for the authoritative system checklist.

## What It Does

- Accepts queue tickets with validated player identity, trophies, deck data, and
  server-derived behavioral signals.
- Hands tickets from ingestion to matchmaking through lock-free ring-buffer
  shards.
- Stores matchmaking queues in Redis sorted sets with score ranges and Lua
  scripts for atomic match assignment.
- Runs a deterministic 200ms match-core tick loop with bounded drain, Redis,
  scoring, assignment, and telemetry budgets.
- Scores player pairs with engagement-optimized matchmaking inputs such as
  trophy distance, churn risk, monetization probability, losing streaks, and
  deck archetype similarity.
- Represents decks as normalized 8-dimensional archetype vectors and computes
  match-time cosine similarity with a tiny fixed-width dot product.
- Simulates player sessions, match outcomes, tilt, churn, deck mutation, trophy
  movement, and convergence gates for large population tests.

## Technical Highlights

- **Language:** Go 1.22+
- **Queue storage:** Redis 7 sorted sets and Lua assignment scripts
- **Concurrency model:** goroutines, atomics, bounded channels, and lock-free
  SPSC-style handoff where appropriate
- **Performance posture:** explicit `0 B/op` hot-path targets for ticket
  parsing, ring-buffer reads/writes, vector scoring, and simulation ticks
- **Testing:** focused unit tests, race runs, allocation benchmarks, and fake
  Redis boundaries by default
- **Simulation:** deterministic player state machine designed to scale toward
  100k concurrent simulated players

## Features

- Ticket validation with fixed-card roster rules and normalized deck vectors.
- Duplicate queue protection and bounded ring-buffer backpressure.
- Redis score encoding for trophy segments, loser pools, retention pools, and
  monetization pools.
- Lua-backed dual-booking protection during assignment.
- Exponential tolerance expansion for long-waiting tickets.
- EOMM routing for mainstream, loser, retention, and monetization paths.
- Churn spike detection with retention alert events.
- Archetype vector classification for structural counters and stylistic mirrors.
- Simulation of player tilt, churn, trophy floors, deck mutation, and session
  exits.
- Async telemetry ring buffers with a lightweight WebSocket visualizer.

## Why It Is Interesting

MatchPoint is intentionally overbuilt in the places matchmaking systems usually
fail: state ownership, timing budgets, allocation discipline, and adversarial
edge cases. The code treats matchmaking as a systems problem, not just a pairing
function. The result is a compact backend prototype that can reason about
fairness, retention, monetization pressure, queue health, and performance in one
coherent loop.

## Project Layout

- `internal/ticket/` validates queue joins and builds tickets.
- `internal/ringbuffer/` implements ingestion handoff buffers.
- `internal/redisqueue/` owns Redis keys, scores, scripts, and queue operations.
- `internal/matchcore/` coordinates the 200ms matchmaking tick.
- `internal/eomm/` routes pools and computes retention-oriented fitness.
- `internal/vectorarch/` normalizes deck vectors and scores similarity.
- `internal/simulation/` drives deterministic player-state simulation.
- `docs/` contains the detailed system, workflow, and domain specifications.
- `contracts/`, `tasks/`, and `reports/` preserve historical planning artifacts.

## Requirements

- Go 1.22+
- Redis 7+ for live Redis integration tests

The default test suite uses deterministic fakes and does not require Redis.

## Development

Run the default test suite:

```sh
go test ./...
```

Run live Redis integration tests:

```sh
MP_REDIS_INTEGRATION=1 go test ./internal/redisqueue
```

Run focused allocation benchmarks for a module:

```sh
go test ./internal/vectorarch -bench=. -benchmem -run='^$'
```
