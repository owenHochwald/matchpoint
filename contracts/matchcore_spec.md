# Matchcore Planner Specification

Module: `matchcore`  
Planner: Project MatchPoint Planner Agent  
Date: 2026-06-28  
Artifacts:
- `contracts/matchcore_contract.go`
- `contracts/matchcore_spec.md`

## Scope

`matchcore` owns the 200ms master tick loop, ringbuffer drain, Redis queue
orchestration, exponential tolerance expansion, candidate selection, atomic
assignment boundary, and per-tick metrics. It depends only on upstream
contracts:

- `TicketRingBuffer`
- `RedisQueueStore`
- `RedisQueueKeyer`
- `RedisScoreCodec`
- `Ticket`
- `RedisQueueEntry`

Future EOMM/vector modules are represented only by `MatchFitnessScorer` and
`MatchVectorScorer`; no downstream concrete package may be imported.

## Public Symbol Coverage

Constants covered by behaviours and allocation table:

| Symbol | Coverage |
| --- | --- |
| `MatchTickIntervalNanos` | `B-MATCHCORE-1`, `B-MATCHCORE-2` |
| `MatchTickHardBudgetNanos` | `B-MATCHCORE-3`, `B-MATCHCORE-4` |
| `MatchRingDrainBudgetNanos` | `B-MATCHCORE-10`, `B-MATCHCORE-11` |
| `MatchRedisQueryBudgetNanos` | `B-MATCHCORE-18`, `B-MATCHCORE-19` |
| `MatchFitnessBudgetNanos` | `B-MATCHCORE-23`, `B-MATCHCORE-24` |
| `MatchAssignBudgetNanos` | `B-MATCHCORE-28`, `B-MATCHCORE-30` |
| `MatchTelemetryBudgetNanos` | `B-MATCHCORE-34`, `B-MATCHCORE-35` |
| `MatchOverrunWarnThreshold` | `B-MATCHCORE-5` |
| `MatchDefaultBaseTolerance` | `B-MATCHCORE-13` |
| `MatchMaxTolerance` | `B-MATCHCORE-15`, `B-MATCHCORE-16` |
| `MatchDefaultToleranceK` | `B-MATCHCORE-14` |
| `MatchToleranceOverflowProduct` | `B-MATCHCORE-16` |
| `MatchCandidateScratchLimit` | `B-MATCHCORE-20` |

Types covered by behaviours:

| Symbol | Coverage |
| --- | --- |
| `MatchCoreStatus` and constants | `B-MATCHCORE-1` through `B-MATCHCORE-38` |
| `MatchCandidateDecision` and constants | `B-MATCHCORE-24` through `B-MATCHCORE-27` |
| `MatchCoreConfig` | `B-MATCHCORE-6` through `B-MATCHCORE-9` |
| `MatchTickInput` | `B-MATCHCORE-1` through `B-MATCHCORE-5` |
| `MatchTickState` | `B-MATCHCORE-1` through `B-MATCHCORE-5`, `B-MATCHCORE-38` |
| `MatchToleranceInput` | `B-MATCHCORE-13` through `B-MATCHCORE-17` |
| `MatchToleranceResult` | `B-MATCHCORE-13` through `B-MATCHCORE-17` |
| `MatchDrainedTicket` | `B-MATCHCORE-10` through `B-MATCHCORE-12` |
| `MatchCandidateContext` | `B-MATCHCORE-23` through `B-MATCHCORE-27` |
| `MatchCandidateScore` | `B-MATCHCORE-23` through `B-MATCHCORE-27` |
| `MatchPair` | `B-MATCHCORE-28` through `B-MATCHCORE-32` |
| `MatchResult` | `B-MATCHCORE-29`, `B-MATCHCORE-31` |
| `MatchTickMetrics` | `B-MATCHCORE-33` through `B-MATCHCORE-37` |
| `MatchTickResult` | `B-MATCHCORE-1` through `B-MATCHCORE-5`, `B-MATCHCORE-37` |
| `MatchClock` | `B-MATCHCORE-1`, `B-MATCHCORE-13` |
| `MatchToleranceCalculator` | `B-MATCHCORE-13` through `B-MATCHCORE-17` |
| `MatchRingDrainer` | `B-MATCHCORE-10` through `B-MATCHCORE-12` |
| `MatchQueueIngress` | `B-MATCHCORE-18`, `B-MATCHCORE-36` |
| `MatchCandidatePlanner` | `B-MATCHCORE-19` through `B-MATCHCORE-22` |
| `MatchFitnessScorer` | `B-MATCHCORE-23` through `B-MATCHCORE-27` |
| `MatchVectorScorer` | `B-MATCHCORE-23` |
| `MatchAssigner` | `B-MATCHCORE-28` through `B-MATCHCORE-32` |
| `MatchMetricsSink` | `B-MATCHCORE-33` through `B-MATCHCORE-37` |
| `MatchOverrunLogger` | `B-MATCHCORE-5` |
| `MatchIDGenerator` | `B-MATCHCORE-29`, `B-MATCHCORE-31` |
| `MatchCoreLoop` | `B-MATCHCORE-1` through `B-MATCHCORE-5`, `B-MATCHCORE-38` |

## Behaviour Specification

B-MATCHCORE-1: Given `MatchCoreLoop.Run` receives 200ms ticker signals and context is active, when a tick signal arrives, then `HandleTick` is invoked with a monotonically increasing `MatchTickInput.TickID` and a `StartedUnixNano` from `MatchClock.NowUnixNano`.

B-MATCHCORE-2: Given multiple ticker signals arrive while the previous handler is still executing, when the previous handler has not completed, then matchcore must not queue catch-up work and must process at most one active tick at a time.

B-MATCHCORE-3: Given `HandleTick` finishes in `<= MatchTickHardBudgetNanos`, when it updates `MatchTickState`, then `ConsecutiveOverruns` is reset to zero, `SkipNextTick` is false, and the result status is not an overrun status.

B-MATCHCORE-4: Given `HandleTick` finishes in `> MatchTickHardBudgetNanos`, when it updates `MatchTickState`, then `TotalOverruns` and `ConsecutiveOverruns` increment, `SkipNextTick` becomes true, and the returned status is `MatchCoreStatusOverrun` unless the warning threshold is reached.

B-MATCHCORE-5: Given `MatchTickState.ConsecutiveOverruns` reaches `MatchOverrunWarnThreshold`, when the overrun tick result is produced, then `MatchOverrunLogger.WarnConsecutiveOverruns` is called once and the returned status is `MatchCoreStatusWarnOverrun`.

B-MATCHCORE-6: Given `MatchCoreConfig.TickIntervalNanos` is not `MatchTickIntervalNanos` in production configuration, when construction or validation occurs, then matchcore returns `MatchCoreStatusInvalidConfig`.

B-MATCHCORE-7: Given `MatchCoreConfig.BaseTolerance <= 0`, `MaxTolerance < BaseTolerance`, negative `ToleranceK`, non-finite `ToleranceK`, zero `DrainBatchSize`, or mismatched `CandidateLimit`, when config validation occurs, then `MatchCoreStatusInvalidConfig` is returned before tick processing starts.

B-MATCHCORE-8: Given `HardBudgetNanos > TickIntervalNanos` or `OverrunWarnThreshold == 0`, when config validation occurs, then matchcore returns `MatchCoreStatusInvalidConfig` and no goroutine is launched.

B-MATCHCORE-9: Given config validation succeeds, when `Run` starts, then the only long-lived matchcore goroutine is the master tick goroutine and it owns all mutation of `MatchTickState`.

B-MATCHCORE-10: Given one or more ring shards contain tickets, when `MatchRingDrainer.DrainRings` runs, then it calls `TicketRingBuffer.DrainShard` for configured shards into caller-owned scratch storage without allocating and records each ticket as `MatchDrainedTicket`.

B-MATCHCORE-11: Given a shard is empty or closed, when `DrainRings` observes `RingReadEmpty` or `RingReadClosed`, then it does not treat that shard as an error and continues draining the remaining shards.

B-MATCHCORE-12: Given the destination scratch slice fills before all shards are empty, when `DrainRings` returns, then the returned count is bounded by `len(dst)` and unread tickets remain in the ring for a later tick.

B-MATCHCORE-13: Given `MatchToleranceInput.NowUnixNano == EnqueuedAtUnixNano`, when `ComputeTolerance` runs with default base tolerance, then `ToleranceTrophies == MatchDefaultBaseTolerance`, `WaitNanos == 0`, and `Clamped == false`.

B-MATCHCORE-14: Given a positive wait time `t`, when `ComputeTolerance` runs with valid `BaseTolerance`, `K`, and `MaxTolerance`, then it computes `BaseTolerance * exp(K*tSeconds)` and converts the clamped result into trophy units for Redis queries.

B-MATCHCORE-15: Given the exponential tolerance result exceeds `MatchMaxTolerance` while `K*t <= MatchToleranceOverflowProduct`, when `ComputeTolerance` runs, then `ToleranceTrophies == MatchMaxTolerance` and `Clamped == true`.

B-MATCHCORE-16: Given `K*t > MatchToleranceOverflowProduct`, when `ComputeTolerance` runs, then it must not call `exp` on the unbounded product and must return `ToleranceTrophies == MaxTolerance` with `Clamped == true`.

B-MATCHCORE-17: Given `NowUnixNano < EnqueuedAtUnixNano` because of clock skew or malformed input, when `ComputeTolerance` runs, then wait time is clamped to zero and tolerance equals `BaseTolerance`.

B-MATCHCORE-18: Given `MatchQueueIngress.EnqueueDrained` receives drained tickets, when it writes to Redis, then each `Ticket` is copied into a `RedisQueueEntry`, encoded through `RedisScoreCodec`, assigned a pool through `RedisQueueKeyer`, and passed to `RedisQueueStore.Enqueue` without retaining the pooled `*Ticket` pointer.

B-MATCHCORE-19: Given Redis enqueue returns `RedisStatusTimeout` or `RedisStatusUnavailable`, when `EnqueueDrained` maps the result, then it returns `MatchCoreStatusRedisTimeout` or `MatchCoreStatusRedisUnavailable` and records the Redis status through `MatchMetricsSink`.

B-MATCHCORE-20: Given queued entries are available for matching, when `MatchCandidatePlanner.BuildCandidateQueries` runs, then it computes one `RedisScoreRange` per eligible anchor with `Limit == MatchCandidateScratchLimit`.

B-MATCHCORE-21: Given a ticket tolerance is `N` trophies, when `BuildCandidateQueries` calls `RedisScoreCodec.ScoreRange`, then it passes `N` as trophy tolerance and leaves score scaling by `RedisScoreTrophyScale` to the codec.

B-MATCHCORE-22: Given no eligible entries or every candidate query returns `RedisStatusEmpty`, when a tick completes, then `MatchTickMetrics.EmptyQueries` increments for empty query results and the tick may return `MatchCoreStatusNoWork` if no other work occurred.

B-MATCHCORE-23: Given `MatchFitnessScorer.ScoreCandidate` receives `MatchCandidateContext`, when future vector scoring is needed, then it uses only the `MatchVectorScorer` interface for cosine similarity and never imports a downstream vector package.

B-MATCHCORE-24: Given a candidate pair has `PlayerID` equality, a trophy delta greater than `ToleranceTrophies`, or malformed pool metadata, when scored, then `MatchCandidateScore.Decision == MatchCandidateReject`.

B-MATCHCORE-25: Given two valid candidates have different `Fitness` values, when best-candidate selection compares them, then the higher `Fitness` candidate replaces the lower one.

B-MATCHCORE-26: Given two valid candidates have equal `Fitness`, when best-candidate selection compares them, then tie-breaking is deterministic by lower `TrophyDelta`, then lower `Candidate.Ticket.EnqueuedAt`, then lower `Candidate.Ticket.PlayerID`.

B-MATCHCORE-27: Given future EOMM returns retention or monetization weights, when `ScoreCandidate` fills `MatchCandidateScore`, then matchcore treats the values as scoring data only and does not mutate `Ticket.PoolTag` in memory.

B-MATCHCORE-28: Given a selected `MatchPair` has the same player on both sides, when `MatchAssigner.AssignPair` is called, then it returns `MatchCoreStatusInvalidCandidate` and does not call `RedisQueueStore.AssignMatch`.

B-MATCHCORE-29: Given a selected `MatchPair` has two distinct players, when `AssignPair` is called, then it obtains a non-zero ID from `MatchIDGenerator`, builds a `RedisAssignRequest`, and calls `RedisQueueStore.AssignMatch`.

B-MATCHCORE-30: Given Redis Lua assignment returns `RedisStatusDualBooking`, when `AssignPair` maps the result, then it returns `MatchCoreStatusDualBooking`, increments dual-booking metrics, and does not emit a committed `MatchResult`.

B-MATCHCORE-31: Given Redis Lua assignment returns `RedisStatusOK`, when `AssignPair` completes, then it fills `MatchResult` with both player IDs, match ID, predicted win probability, pool source, and assignment timestamp.

B-MATCHCORE-32: Given Redis assignment returns timeout, unavailable, canceled, or script-related statuses, when `AssignPair` maps the status, then it returns the matching `MatchCoreStatus` where defined or a non-OK status without retrying outside the Redisqueue contract.

B-MATCHCORE-33: Given a tick drains tickets, executes candidate queries, and commits matches, when `MatchTickMetrics` is produced, then `DrainedTickets`, `CandidateQueries`, `MatchesMade`, and `DurationNanos` reflect observed work for that tick.

B-MATCHCORE-34: Given an overrun tick completes, when metrics are emitted, then `RecordOverrun` and `RecordTick` receive the same tick ID and duration without string formatting on the hot path.

B-MATCHCORE-35: Given a tick is skipped because `SkipNextTick` was true, when metrics are emitted, then `RecordSkippedTick` is called, `SkippedTicks` increments, and no ring or Redis operation is performed for that skipped signal.

B-MATCHCORE-36: Given Redis operations report timeout, unavailable, partial, canceled, empty, or OK statuses, when matchcore records metrics, then it calls `RecordRedisStatus` with the original `RedisQueueStatus` and elapsed nanos.

B-MATCHCORE-37: Given `HandleTick` completes, when it returns `MatchTickResult`, then `FinishedUnixNano >= StartedUnixNano`, `Metrics.TickID == TickID`, and status reflects the highest-severity outcome observed in the tick.

B-MATCHCORE-38: Given telemetry or tests call `SnapshotTickState`, when the method copies state into caller-owned storage, then it performs no allocation and returns a coherent snapshot of last start, last finish, overrun counters, skipped ticks, and skip flag.

## Allocation Budget Table

| Hot-path function or method | Max heap allocation | Justification |
| --- | ---: | --- |
| `MatchClock.NowUnixNano` | `0 B/op` | Timestamp read only. |
| `MatchToleranceCalculator.ComputeTolerance` | `0 B/op` | Pure math over scalar inputs. |
| `MatchRingDrainer.DrainRings` | `0 B/op` | Uses caller-owned `[]MatchDrainedTicket` and upstream ring scratch. |
| `MatchQueueIngress.EnqueueDrained` helper work before Redis call | `0 B/op` | Copies tickets into caller-owned/preallocated entries; Redis client allocation is charged to Redis boundary. |
| `RedisQueueStore.Enqueue` as invoked by matchcore | `<= 256 B/op` | Network command boundary inherited from redisqueue. |
| `MatchCandidatePlanner.BuildCandidateQueries` | `0 B/op` | Uses caller-owned `RedisQueryBatch.Ranges` and result slots. |
| `RedisQueueStore.FetchCandidateBatch` as invoked by matchcore | `< 512 B/op` | Pipeline boundary inherited from redisqueue and FEATURES §10.1 `TickHandler`. |
| `MatchFitnessScorer.ScoreCandidate` baseline implementation | `0 B/op` | Stack-only scoring; future EOMM/vector must preserve hot-path budget. |
| `MatchVectorScorer.CosineSimilarity` | `0 B/op` | Fixed `[8]float32` dot product. |
| Best-candidate tie-breaking | `0 B/op` | Scalar comparisons only. |
| `MatchAssigner.AssignPair` helper work before Redis call | `0 B/op` | Builds `RedisAssignRequest` and validates IDs in stack/caller-owned storage. |
| `RedisQueueStore.AssignMatch` as invoked by matchcore | `<= 256 B/op` | EVALSHA command boundary inherited from redisqueue. |
| `MatchMetricsSink.RecordTick` | `0 B/op` | Atomic counters or fixed-size telemetry event only. |
| `MatchMetricsSink.RecordOverrun` | `0 B/op` | Atomic counters; logging is only at warning threshold. |
| `MatchMetricsSink.RecordSkippedTick` | `0 B/op` | Atomic counter only. |
| `MatchMetricsSink.RecordDualBooking` | `0 B/op` | Atomic counter only. |
| `MatchMetricsSink.RecordEmptyQuery` | `0 B/op` | Atomic counter only. |
| `MatchMetricsSink.RecordRedisStatus` | `0 B/op` | Atomic counters/histogram buckets only. |
| `MatchOverrunLogger.WarnConsecutiveOverruns` | `<= 512 B/op` | Non-steady-state warning path; must not run on every tick. |
| `MatchIDGenerator.NextMatchID` | `0 B/op` | Atomic counter or equivalent scalar generator. |
| `MatchCoreLoop.HandleTick` steady-state helper work | `< 512 B/op` | Matches FEATURES §10.1 `TickHandler`; Redis pipeline allocation is the only justified non-zero source. |
| `MatchCoreLoop.Run` per tick | `< 512 B/op` | Delegates to `HandleTick`; ticker creation is startup cost, not per-tick hot path. |
| `MatchCoreLoop.SnapshotTickState` | `0 B/op` | Caller-owned output struct. |

## Edge Case Register

| Edge case | Contracted handling |
| --- | --- |
| Tick drift | `StartedUnixNano` and `ScheduledUnixNano` remain observable in `MatchTickInput`; drift does not queue catch-up ticks. |
| Tick handler exceeds 200ms | Overrun counters increment and `SkipNextTick` is set. |
| Next tick after overrun | The tick is skipped, `SkippedTicks` increments, and no ring or Redis work occurs. |
| Three consecutive overruns | `MatchOverrunLogger.WarnConsecutiveOverruns` fires once for the threshold crossing. |
| Zero or negative wait | Wait is clamped to zero and tolerance is exactly `BaseTolerance`. |
| `k*t > 10.0` | Return `MaxTolerance` without computing an unbounded exponential. |
| Exponential result exceeds max | Clamp with `math.Min` semantics to `MaxTolerance`. |
| Invalid tolerance config | Return `MatchCoreStatusInvalidConfig`; no tick processing starts. |
| Empty ring shards | Not an error; continue other shards. |
| Drain scratch exhaustion | Stop at caller-owned slice capacity and leave unread ring tickets for a later tick. |
| Redis enqueue timeout | Map to `MatchCoreStatusRedisTimeout` and record Redis status. |
| Redis unavailable | Map to `MatchCoreStatusRedisUnavailable` and avoid local retries outside redisqueue. |
| Redis candidate empty result | Increment `EmptyQueries`; proceed with other ranges. |
| Redis candidate pipeline partial | Record original Redis status and preserve successful result slots. |
| Candidate self-match | Reject before scoring or assignment. |
| Candidate trophy delta exceeds tolerance | Reject candidate as invalid for the current anchor. |
| Candidate tie | Break deterministically by lower trophy delta, older enqueue time, then lower player ID. |
| Lua dual booking | Return `MatchCoreStatusDualBooking`, record metric, and do not emit `MatchResult`. |
| Assignment timeout unknown commit state | Treat as Redis boundary status; do not assume match committed unless `RedisStatusOK` is returned. |
| Script cache/NOSCRIPT status | Do not implement script reload in matchcore; defer to redisqueue contract. |
| `Ticket.PoolTag` future EOMM mutation | Matchcore reads copied ticket data and does not perform in-memory atomic pool-tag mutation. |
| Future vectorarch integration | Matchcore calls `MatchVectorScorer`; no downstream package import or concrete type leak. |
| Future EOMM integration | Matchcore calls `MatchFitnessScorer`; routing remains owned by future EOMM contracts. |
| Metrics sink unavailable in tests | Implementations may use a no-op sink that still satisfies `0 B/op`. |
| Context cancellation | Return `MatchCoreStatusCanceled` and do not launch replacement goroutines. |
| `Run` shutdown | Context cancellation exits after the active tick returns; no goroutine retains an undrained channel reference. |

## Formula Coverage

| Formula or invariant | Behaviour |
| --- | --- |
| 200ms ticker interval | `B-MATCHCORE-1`, `B-MATCHCORE-2` |
| Overrun skip next tick | `B-MATCHCORE-4`, `B-MATCHCORE-5`, `B-MATCHCORE-35` |
| `Tolerance(t)=BaseTolerance*exp(k*t)` | `B-MATCHCORE-13`, `B-MATCHCORE-14` |
| `k*t > 10.0` overflow guard | `B-MATCHCORE-16` |
| `MaxTolerance=2000` clamp | `B-MATCHCORE-15`, `B-MATCHCORE-16` |
| Redis score range in trophy tolerance units | `B-MATCHCORE-20`, `B-MATCHCORE-21` |
| `ZRANGEBYSCORE ... LIMIT 0 8` candidate limit | `B-MATCHCORE-20` |
| EOMM/vector scoring boundary | `B-MATCHCORE-23`, `B-MATCHCORE-27` |
| Atomic Lua assignment boundary | `B-MATCHCORE-28` through `B-MATCHCORE-32` |
| Metrics: duration, overruns, skipped, matches, drained, dual bookings, empty queries | `B-MATCHCORE-33` through `B-MATCHCORE-37` |

## Planner Assumptions

- Redisqueue owns score scaling by `RedisScoreTrophyScale`; matchcore passes
  tolerance in trophies to `RedisScoreCodec.ScoreRange`.
- Matchcore copies drained ticket data before Redis enqueue and does not retain
  pooled `*Ticket` pointers across Redis boundaries.
- Matchcore owns selection order and assignment orchestration, while future EOMM
  and vectorarch modules provide scoring interfaces only.
- `MATCH_SPEC.md` wins on intake trophy range `[0, 15000]`; matchcore does not
  enforce tier floor protection because that is post-match simulation logic.
- Redis timeout and unknown-commit semantics are represented by typed statuses;
  matchcore does not retry or reinterpret Redisqueue results.

## Signature

Signed-off Planner Agent: Project MatchPoint Planner  
Status: Planner contract complete for `matchcore`
