# MatchPoint

MatchPoint is a Go matchmaking backend prototype for a single-node, game-style
queueing system. The project focuses on contracted internal modules for ticket
intake, lock-free ring-buffer handoff, Redis-backed priority queues, and a
deterministic match core loop.

## Project Layout

- `contracts/` defines cross-module interfaces and shared data contracts.
- `internal/ticket/` implements ticket parsing, validation, pooling, and deck
  vector construction.
- `internal/ringbuffer/` implements the ingestion handoff ring buffers.
- `internal/redisqueue/` owns Redis ZSET and Lua queue boundaries.
- `internal/matchcore/` coordinates one matchmaking tick at a time.
- `docs/` contains the system and domain specifications.
- `tasks/` and `reports/` capture implementation plans and checker reports.

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
