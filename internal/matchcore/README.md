# Matchcore

`matchcore` owns one synchronous matchmaking tick at a time. The production
`Run` method consumes 200ms ticker signals, but the implementation keeps the
actual work in `HandleTick` so tests and benchmarks can drive deterministic
single-tick execution without sleeping.

## Invariants

- Cross-module dependencies use `matchpoint/contracts` only. The package does
  not import concrete `internal/ticket`, `internal/ringbuffer`, or
  `internal/redisqueue` packages.
- Ring drains, Redis queue entries, query ranges, candidate result slots, and
  assignment output are preallocated on `matchCore` construction and reused per
  tick.
- Tolerance remains in trophy units in matchcore. `RedisScoreCodec.ScoreRange`
  owns conversion to Redis score units.
- Drained ticket pointers are copied into `RedisQueueEntry` values before Redis
  enqueue. Matchcore does not retain pooled `*Ticket` pointers across Redis
  boundaries.
- Candidate scoring is an interface boundary. The baseline scorer rejects
  self-matches, out-of-tolerance trophies, and malformed pools without mutating
  `Ticket.PoolTag`.
- Assignment is delegated to `RedisQueueStore.AssignMatch`. Matchcore maps typed
  Redis statuses to typed matchcore statuses and does not retry Lua/script
  outcomes locally.
- Overrun handling is stateful: an over-budget tick sets `SkipNextTick`, the
  next signal records a skipped tick and performs no ring or Redis work, and the
  warning logger fires once when the consecutive-overrun threshold is reached.

## Benchmark Shape

`BenchmarkMatchLoop` uses fake in-memory ring and Redis dependencies. It avoids
network, Docker, live timers, goroutines, and dynamic setup in the timed loop so
the CPU profile reflects matchcore orchestration rather than external systems.

`GOMAXPROCS` variants save and restore the previous runtime setting to avoid
cross-benchmark hygiene warnings.
