# Redisqueue Module

`redisqueue` owns the Redis ZSET and Lua boundary for MatchPoint queue state.
It depends only on `matchpoint/contracts` for cross-module data and keeps Redis
client details behind an internal adapter.

## Implementation Notes

- Fixed key constants are returned directly; invalid pools never synthesize
  dynamic keys.
- ZSET scores use `Trophies*1e6 + EnqueuedAt/1000` with an exact `float64`
  precision guard.
- `RedisQueueEntry` copies `contracts.Ticket`, so pooled ticket pointers are not
  retained across the Redis boundary.
- The script cache stores the 40-byte SHA in fixed storage and clears it on
  `NOSCRIPT`.
- Default tests use a deterministic fake Redis adapter. Live Redis integration
  remains gated behind `MP_REDIS_INTEGRATION=1`.
- Real `go-redis/v9` operations receive a per-command context deadline. Fake
  adapter benchmarks use elapsed-time classification so the benchmark measures
  queue wrapper overhead rather than `context.WithTimeout` timer allocation.
- Redis-bound methods use non-zero allocation budgets because `go-redis/v9`
  command construction and network boundaries allocate. Pure keyer, codec,
  script-cache lookup, script-cache clear, and metrics paths are benchmarked at
  `0 B/op`.

## Concurrency

Queue state is serialized by Redis. Local mutable state is limited to:

- `scriptCache`, protected by `sync.RWMutex` because SHA load and invalidation
  are rare control-plane events.
- `queueMetrics`, implemented with atomics.

No `internal/ticket` or `internal/ringbuffer` dependency is used.
