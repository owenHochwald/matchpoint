# MatchPoint

MatchPoint is a Go matchmaking backend prototype for a single-node, game-style
queueing system. The project focuses on internal modules for ticket intake,
lock-free ring-buffer handoff, Redis-backed priority queues, deterministic match
core behavior, player simulation, and telemetry.

## Project Layout

- `docs/FEATURES.md` is the system specification and completion checklist.
- `docs/SESSIONS.md` tracks current in-progress agent handoff state.
- `contracts/` contains historical cross-module interfaces and shared data
  contracts from the earlier workflow.
- `internal/ticket/` implements ticket parsing, validation, pooling, and deck
  vector construction.
- `internal/ringbuffer/` implements the ingestion handoff ring buffers.
- `internal/redisqueue/` owns Redis ZSET and Lua queue boundaries.
- `internal/matchcore/` coordinates one matchmaking tick at a time.
- `internal/eomm/` implements pool routing and retention-oriented match scoring.
- `internal/vectorarch/` implements archetype vectors and similarity scoring.
- `internal/simulation/` contains the player state machine harness.
- `docs/` contains the system and domain specifications.
- `tasks/` and `reports/` contain historical implementation plans and checker
  reports.

## Requirements

- Go 1.22+
- Redis is only needed for live Redis integration tests; the default test suite
  uses fakes.

## Development

Run the default test suite:

```sh
go test ./...
```

Live Redis integration is gated behind:

```sh
MP_REDIS_INTEGRATION=1 go test ./internal/redisqueue
```
