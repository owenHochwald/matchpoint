# Redisqueue Contract Specification

Module: `redisqueue`  
Phase: Planner  
Status: Signed Planner contract for Module 3.

## Scope

`redisqueue` owns the Redis 7.0+ single-instance queue boundary used by the
200ms match tick. The implementation uses `go-redis/v9`, but downstream modules
depend only on `contracts.RedisQueueStore`, `contracts.RedisScriptCache`,
`contracts.RedisQueueKeyer`, and `contracts.RedisScoreCodec`.

Redis queue state is external shared state. Correctness is enforced by Redis
serialization and the contracted Lua assignment script; Go implementations must
not introduce mutable cross-module state.

## Public Symbol Coverage

- Constants: `RedisCommandTimeoutNanos`, `RedisScoreTrophyScale`,
  `RedisCandidateLimit`, `RedisMatchTTLSeconds`, `RedisQueueSegmentCount`,
  every `Redis*Key`, `Redis*KeyPrefix`, and `RedisAssignMatchLua`.
- Types: `RedisQueuePool`, `RedisQueueStatus`, `RedisScriptKind`,
  `RedisQueueConfig`, `RedisSegmentRange`, `RedisScriptSHA`, `RedisMember`,
  `RedisScore`, `RedisScoreRange`, `RedisQueueEntry`, `RedisCandidate`,
  `RedisQueryBatch`, `RedisMoveRequest`, `RedisAssignRequest`,
  `RedisOperationResult`, and `RedisAssignResult`.
- Interfaces: `RedisQueueMetrics`, `RedisQueueKeyer`, `RedisScoreCodec`,
  `RedisScriptCache`, and `RedisQueueStore`.

## Behaviour Specification

B-REDISQUEUE-1: Given a production Redis queue config is constructed, when its fields are inspected, then `CommandTimeoutNanos` is `5_000_000`, pool size is `runtime.NumCPU()*4`, candidate limit is `8`, and match TTL is `3600` seconds.

B-REDISQUEUE-2: Given trophies `0`, `1000`, `1001`, `3000`, `3001`, `6000`, `6001`, `10000`, and `10001`, when `SegmentForTrophies` is called, then it returns respectively segment pools `0`, `0`, `1`, `1`, `2`, `2`, `3`, `3`, and `4`.

B-REDISQUEUE-3: Given negative trophies or a pool outside the contracted segment and special-pool range, when the keyer maps the value, then it returns `RedisStatusInvalidSegment` and does not synthesize a dynamic key.

B-REDISQUEUE-4: Given any valid `RedisQueuePool`, when `KeyForPool` is called, then the returned key is exactly one of `mq:seg:0`, `mq:seg:1`, `mq:seg:2`, `mq:seg:3`, `mq:seg:4`, `mq:losers`, `mq:retention`, or `mq:monetize`.

B-REDISQUEUE-5: Given a mainstream segment pool, when `SegmentRange` is called, then its inclusive boundaries match the queue segment table and segment `4` reports `MaxTrophies == -1`.

B-REDISQUEUE-6: Given a non-zero player ID, when `EncodeMember` writes into caller-owned storage, then `RedisMember.Bytes[:Len]` contains the base-10 player ID, `Len` is in `[1,20]`, and no heap allocation is required on the steady-state path.

B-REDISQUEUE-7: Given player ID `0`, when `EncodeMember` is called, then it returns `RedisStatusInvalidScore` and leaves a zero-length member.

B-REDISQUEUE-8: Given trophies and `EnqueuedAt` nanoseconds, when `EncodeScore` is called, then the score equals `Trophies * 1e6 + EnqueuedAt_microseconds_truncated`.

B-REDISQUEUE-9: Given an `EnqueuedAt` value with a non-zero nanosecond remainder below one microsecond, when `EncodeScore` is called, then the remainder is truncated rather than rounded.

B-REDISQUEUE-10: Given an encoded score for trophies in the MatchPoint intake range, when the value is represented as a Redis `float64`, then it preserves the integer score exactly for the expected range.

B-REDISQUEUE-11: Given score encoding would overflow the exact integer precision allowed by the contract, when `EncodeScore` is called, then it returns `RedisStatusInvalidScore`.

B-REDISQUEUE-12: Given trophies, enqueue timestamp, tolerance trophies, and a valid pool, when `ScoreRange` is called, then it writes inclusive bounds `[score - tolerance*1e6, score + tolerance*1e6]` and limit `8`.

B-REDISQUEUE-13: Given a drained `Ticket` from the ringbuffer, when a `RedisQueueEntry` is built for enqueue, then the ticket is copied into the entry, the member encodes `Ticket.PlayerID`, the score uses `Ticket.Trophies` and `Ticket.EnqueuedAt`, and the pool is selected from the ticket's current pool tag.

B-REDISQUEUE-14: Given an entry in a mainstream segment, when `Enqueue` succeeds, then Redis receives a `ZADD` for the exact segment key and a metadata update for `mq:player:<id>`.

B-REDISQUEUE-15: Given an entry in `RedisPoolLosers`, `RedisPoolRetention`, or `RedisPoolMonetize`, when `Enqueue` succeeds, then Redis receives a `ZADD` for only that special-pool key and not for a mainstream segment.

B-REDISQUEUE-16: Given a Redis command exceeds `5ms`, when any store operation returns, then the result status is `RedisStatusTimeout`, elapsed nanoseconds are recorded, and `RedisQueueMetrics.IncLatency` is called.

B-REDISQUEUE-17: Given the caller context is canceled before or during a Redis command, when any store operation returns, then the status is `RedisStatusCanceled` and no dynamic error formatting occurs on the hot path.

B-REDISQUEUE-18: Given Redis is unreachable or returns a connection-level error, when any store operation returns, then the status is `RedisStatusUnavailable`.

B-REDISQUEUE-19: Given a member is present in a pool, when `Remove` succeeds, then the member is removed from exactly that ZSET and `Count` reports one affected member.

B-REDISQUEUE-20: Given a member is absent from a pool, when `Remove` is called, then the operation is idempotent and returns `RedisStatusOK` with `Count == 0`.

B-REDISQUEUE-21: Given a score range with matching candidates, when `FetchCandidates` is called with caller-owned destination storage of length `N`, then it writes at most `min(N, RedisCandidateLimit)` candidates and returns `RedisStatusOK`.

B-REDISQUEUE-22: Given a score range with no matching candidates, when `FetchCandidates` is called, then it returns `RedisStatusEmpty` and writes no candidate entries.

B-REDISQUEUE-23: Given a tick has multiple range queries, when `FetchCandidateBatch` is called, then the implementation pipelines all valid `ZRANGEBYSCORE` commands in one Redis round-trip boundary and stores each result in the matching `Results` slot.

B-REDISQUEUE-24: Given a pipeline has at least one successful range query and at least one failed query, when `FetchCandidateBatch` returns, then its status is `RedisStatusPartial` and `RedisQueueMetrics.IncPipelinePartial` is called.

B-REDISQUEUE-25: Given `RedisQueryBatch.Count` exceeds the lengths of `Ranges` or `Results`, when `FetchCandidateBatch` is called, then it returns `RedisStatusInvalidSegment` and does not execute a partial caller-bounds-unsafe pipeline.

B-REDISQUEUE-26: Given a member exists in a source pool and not in the destination pool, when `MovePool` succeeds, then the source ZSET no longer contains the member, the destination ZSET contains the same member at the exact same score, and the result count is one.

B-REDISQUEUE-27: Given `MovePool` is repeated for the same member, source, destination, and score, when the second call runs, then it is idempotent and does not create duplicate destination entries.

B-REDISQUEUE-28: Given a special-pool evacuation condition from the EOMM contract, when `MovePool` moves a member back to the mainstream segment, then the destination segment is derived from the member ticket trophies and the preserved score.

B-REDISQUEUE-29: Given startup begins before match ticks are processed, when `LoadScripts` is called, then it issues `SCRIPT LOAD` for `RedisAssignMatchLua` and stores a 40-byte SHA for `RedisScriptAssignMatch`.

B-REDISQUEUE-30: Given no SHA is cached for `RedisScriptAssignMatch`, when `ScriptSHA` is called, then it returns `RedisStatusScriptNotLoaded`.

B-REDISQUEUE-31: Given Redis returns `NOSCRIPT` for an assignment, when `MarkNoScript` is called, then the cached SHA is cleared, `RedisQueueMetrics.IncScriptReload` is called by the implementation path, and the next startup/reload path can reload the script.

B-REDISQUEUE-32: Given two queued players are both still present in their source ZSETs, when `AssignMatch` executes `EVALSHA`, then the Lua script removes both members, writes `mq:match:<id>` with `playerA` and `playerB`, sets TTL `3600`, and returns `RedisStatusOK`.

B-REDISQUEUE-33: Given `SourceA` and `SourceB` are the same ZSET, when `AssignMatch` succeeds, then both players are removed atomically from the single ZSET and no double-removal error leaks to the caller.

B-REDISQUEUE-34: Given either player has already been removed from its source ZSET, when `AssignMatch` executes the Lua script, then the script returns `0`, the result status is `RedisStatusDualBooking`, and `RedisQueueMetrics.IncDualBooking` is called.

B-REDISQUEUE-35: Given `EVALSHA` is called before script load, when `AssignMatch` observes missing SHA state, then it returns `RedisStatusScriptNotLoaded` without falling back to dynamic `EVAL` on the hot path.

B-REDISQUEUE-36: Given Redis replies `NOSCRIPT` to `EVALSHA`, when `AssignMatch` returns, then status is `RedisStatusNoScript`, the cached SHA is invalidated, and the caller must route through the script reload path before retry.

B-REDISQUEUE-37: Given `RedisAssignMatchLua` source is inspected, when compared to the storage contract, then it uses `ZSCORE` guards, returns `0` on missing players, `ZREM`s both players, writes `mq:match:<id>`, sets `EXPIRE 3600`, and returns `1` on success.

B-REDISQUEUE-38: Given the loser's pool contains fewer than two other players after more than 10 ticks of wait, when EOMM requests evacuation, then `MovePool` supports moving the player out of `mq:losers` without changing score or match state.

B-REDISQUEUE-39: Given `RedisQueueMetrics` receives timeout, script reload, dual-booking, or partial-pipeline notifications, when counters are incremented, then each increment is typed by status or event and requires no formatted error string.

B-REDISQUEUE-40: Given implementations benchmark hot-path Redis boundary methods with a fake or local Redis response path, when benchmark allocation output is inspected, then codec, keyer, script cache lookup, and non-network result construction paths report `0 B/op`.

## Allocation Budget Table

| Hot-path function | Budget (B/op) | Justification |
| --- | ---: | --- |
| `RedisQueueKeyer.SegmentForTrophies` | 0 | Pure integer boundary mapping. |
| `RedisQueueKeyer.KeyForPool` | 0 | Returns stable key constants only. |
| `RedisQueueKeyer.SegmentRange` | 0 | Returns fixed struct by value. |
| `RedisScoreCodec.EncodeMember` | 0 | Writes decimal player ID into caller-owned `[20]byte`. |
| `RedisScoreCodec.EncodeScore` | 0 | Pure integer/float arithmetic. |
| `RedisScoreCodec.ScoreRange` | 0 | Writes range into caller-owned struct. |
| `RedisScriptCache.ScriptSHA` | 0 | Copies fixed `[40]byte` SHA into caller-owned storage. |
| `RedisScriptCache.MarkNoScript` | 0 | Clears cached fixed-size SHA state. |
| `RedisQueueStore.Enqueue` | <= 256 | Redis client command object/network boundary; no dynamic error strings on hot path. |
| `RedisQueueStore.Remove` | <= 128 | Redis client command object/network boundary. |
| `RedisQueueStore.FetchCandidates` | <= 256 | Redis response decoding into caller-owned slice; client command overhead allowed. |
| `RedisQueueStore.FetchCandidateBatch` | < 512 | Contracted Redis pipeline boundary for one tick; matches FEATURES `TickHandler` Redis budget. |
| `RedisQueueStore.MovePool` | <= 256 | Redis transaction/script or pipeline boundary; no caller-owned data retention. |
| `RedisQueueStore.AssignMatch` | <= 256 | EVALSHA command boundary inside the 20ms Lua assignment budget. |
| `RedisQueueMetrics.*` | 0 | Atomic counters only. |

## Edge Case Register

| Edge case | Contracted handling |
| --- | --- |
| Redis command timeout | Return `RedisStatusTimeout`, record elapsed nanos, increment latency metrics, and avoid dynamic error formatting. |
| Context cancellation | Return `RedisStatusCanceled`; do not retry after caller cancellation. |
| Redis unavailability | Return `RedisStatusUnavailable`; caller decides whether to skip tick or retry later. |
| Script cache miss | `ScriptSHA` and `AssignMatch` return `RedisStatusScriptNotLoaded`; hot path must not invoke raw `EVAL`. |
| Redis `NOSCRIPT` | Return `RedisStatusNoScript`, clear cached SHA, increment script reload metric, and require reload before retry. |
| Lua dual booking | Lua return `0` maps to `RedisStatusDualBooking`; both Go and Redis state must remain deterministic. |
| Same-source ZSET assignment | Lua must support `KEYS[1] == KEYS[2]` and remove both distinct members atomically. |
| PlayerA equals PlayerB | Implementation must reject before EVALSHA with a typed non-OK status; self-match is never valid. |
| ZSET score precision | Scores must remain exact for MatchPoint trophy and timestamp bounds; lossy encodings return `RedisStatusInvalidScore`. |
| Enqueued timestamp sub-microsecond remainder | Remainder is truncated, never rounded. |
| Segment boundary trophies | Boundary values map exactly to the segment table; negative trophies are invalid for Redis queueing. |
| Empty query results | Return `RedisStatusEmpty`; leave caller-owned candidate storage untouched beyond count zero. |
| Destination candidate slice too short | Write at most `len(dst)` candidates and report the actual count. |
| Pipeline partial errors | Return `RedisStatusPartial`, preserve successful result slots, and increment partial-pipeline metric. |
| Invalid batch dimensions | Return `RedisStatusInvalidSegment`; do not execute a partially unsafe pipeline. |
| Special-pool movement idempotency | Repeated `MovePool` calls must not duplicate members and must preserve score. |
| Loser's pool starvation evacuation | `MovePool` supports evacuation to segment ZSET after EOMM detects >10 ticks with insufficient peers. |
| Match record TTL | `AssignMatch` must set `EXPIRE mq:match:<id> 3600` on success. |
| Metadata hash consistency | `Enqueue` updates `mq:player:<id>` metadata with ticket-derived churn, monetization, and streak state. |
| Redis key synthesis | Only fixed keys or documented prefixes may be used; invalid pools never generate arbitrary keys. |

## Cross-Module Boundaries

- Upstream data enters as `contracts.Ticket` copied into `RedisQueueEntry`.
- No `internal/ticket` or `internal/ringbuffer` package may be imported.
- `matchcore` consumes only the queue/store interfaces and owns tolerance
  computation; `redisqueue` only receives score ranges and assignment requests.
- `eomm` owns routing decisions and calls `MovePool`; `redisqueue` only enforces
  the key/score/movement semantics.

## Planner Assumptions

- `MATCH_SPEC.md` trophy intake range remains `[0, 15000]`; Redis segment `4`
  covers all trophies `>= 10001`, including Diamond and Champion players.
- Redis is single-instance, not cluster mode, so multi-key Lua assignment is
  legal for all contracted queue keys.
- The implementation may use `go-redis/v9` internally without exposing
  go-redis concrete types in the contract package.
- The Lua script source is deliberately simple and mirrors `FEATURES.md`; any
  future lock-set usage for `mq:locks` requires Planner recertification.

## Checker WARN Annotations

Appended by Orchestrator after `reports/redisqueue_checker_report.md` returned
`CHECKER: WARN` on 2026-06-28.

- `CPU_WARN`: the redisqueue CPU profile benchmark completed, but this Go
  toolchain lacks `go tool pprof`; fallback `go tool preprofile` output
  contained syscall/GC-related sampled frames including `runtime.netpoll`,
  `runtime.kevent`, and `runtime.mallocgc`.
- `BENCH_WARN`: the fresh benchmark run emitted Go test warnings that several
  `GOMAXPROCS1` benchmarks left `GOMAXPROCS` set to `1`. Allocation values
  remained within contract.
- `COVERAGE_WARN`: behaviour markers `B-REDISQUEUE-1` through
  `B-REDISQUEUE-40` are all present, but some multi-clause behaviours are
  covered through representative fake-store paths rather than every
  Redis-bound method variant.
