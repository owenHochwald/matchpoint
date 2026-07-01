# EOMM Planner Specification

Module: `eomm`  
Planner: Project MatchPoint Planner Agent  
Date: 2026-07-01  
Artifacts:
- `contracts/eomm_contract.go`
- `contracts/eomm_spec.md`

## Scope

`eomm` owns special-pool routing, special-pool evacuation, engagement-aware
fitness scoring, and adaptive churn spike detection. It depends only on
contracted upstream types:

- `Ticket`
- `RedisQueueEntry`
- `RedisQueueStore`
- `RedisMoveRequest`
- `MatchCandidateContext`
- `MatchCandidateScore`

`eomm` must not import concrete internal packages. Vector scoring remains pure
Go over the normalized `[8]float32` ticket vectors until `vectorarch` replaces
that boundary.

## Public Symbol Coverage

Constants:

| Symbol | Coverage |
| --- | --- |
| `EOMMDefaultTrophyWeight` | `B-EOMM-16`, `B-EOMM-17` |
| `EOMMDefaultVectorWeight` | `B-EOMM-16`, `B-EOMM-17` |
| `EOMMDefaultRetentionWeight` | `B-EOMM-16`, `B-EOMM-17` |
| `EOMMWeightSum`, `EOMMWeightEpsilon` | `B-EOMM-7` |
| `EOMMHighChurnThreshold` | `B-EOMM-2`, `B-EOMM-24` |
| `EOMMHighMonetizationThreshold` | `B-EOMM-3` |
| `EOMMLoserBaseTolerance` | `B-EOMM-8` |
| `EOMMLoserStarvationTicks` | `B-EOMM-9` |
| `EOMMRetentionTrophyOffset` | `B-EOMM-18` |
| `EOMMRetentionTargetWinP` | `B-EOMM-19` |
| `EOMMMonetizeTargetWinP` | `B-EOMM-22` |
| `EOMMCounterSimilarityThreshold` | `B-EOMM-20`, `B-EOMM-21` |
| `EOMMSimilarSimilarityThreshold` | `B-EOMM-23` |
| `EOMMSpikeWindowTicks`, `EOMMSpikeWinRateThreshold` | `B-EOMM-24`, `B-EOMM-25` |

Types and interfaces:

| Symbol | Coverage |
| --- | --- |
| `EOMMStatus` | `B-EOMM-1` through `B-EOMM-28` |
| `EOMMRouteReason` | `B-EOMM-1` through `B-EOMM-6`, `B-EOMM-9` through `B-EOMM-12` |
| `EOMMConfig` | `B-EOMM-7`, `B-EOMM-16`, `B-EOMM-17` |
| `EOMMRoutingInput`, `EOMMRouteDecision`, `EOMMRouter` | `B-EOMM-1` through `B-EOMM-12` |
| `EOMMMatchOutcome` | `B-EOMM-10` through `B-EOMM-12` |
| `EOMMPoolMover` | `B-EOMM-13` through `B-EOMM-15` |
| `EOMMFitnessScorer`, `EOMMScoreBreakdown` | `B-EOMM-16` through `B-EOMM-23`, `B-EOMM-28` |
| `EOMMSpikeInput`, `EOMMSpikeState`, `EOMMChurnAlertEvent`, `EOMMSpikeDetector` | `B-EOMM-24` through `B-EOMM-27` |

## Behaviour Specification

B-EOMM-1: Given a mainstream ticket has `ConsecLosses <= -2`, when `RouteTicket` evaluates routing, then the decision moves it to `RedisPoolLosers`, sets `PoolTag == PoolLosers`, and reports `EOMMRouteLoser`.

B-EOMM-2: Given a ticket has `ChurnRisk > EOMMHighChurnThreshold` and `ConsecLosses <= -1` but does not satisfy loser routing, when `RouteTicket` evaluates routing, then the decision moves it to `RedisPoolRetention` and reports `EOMMRouteRetention`.

B-EOMM-3: Given a ticket has `MonetizationP > EOMMHighMonetizationThreshold` and `ConsecWins >= 2` but does not satisfy earlier routing, when `RouteTicket` evaluates routing, then the decision moves it to `RedisPoolMonetize` and reports `EOMMRouteMonetize`.

B-EOMM-4: Given multiple routing conditions are true, when `RouteTicket` evaluates routing, then it selects the first condition in loser, retention, monetization, mainstream order.

B-EOMM-5: Given no special routing condition is true and the ticket is already in its mainstream segment, when `RouteTicket` runs, then `Move == false`, `To == MainstreamPool`, and reason is `EOMMRouteMainstream`.

B-EOMM-6: Given no special routing condition is true and the ticket is currently in a special pool, when `RouteTicket` runs, then it moves the ticket back to `MainstreamPool`.

B-EOMM-7: Given configured weights are negative, non-finite, or do not sum to `EOMMWeightSum` within `EOMMWeightEpsilon`, when scoring construction or validation occurs, then `EOMMStatusWeightMismatch` or `EOMMStatusInvalidConfig` is returned before scoring.

B-EOMM-8: Given a loser-pool ticket is matched against candidates, when EOMM prepares scoring metadata, then it uses `EOMMLoserBaseTolerance` and never routes loser-pool tickets into non-loser candidate pools for peer matching.

B-EOMM-9: Given a loser-pool ticket has waited more than `EOMMLoserStarvationTicks` and fewer than two other loser-pool players are present, when `RouteTicket` runs, then the decision evacuates it to `MainstreamPool` with `EOMMRouteStarvationEvacuate`.

B-EOMM-10: Given a loser-pool player wins a completed match, when `RouteMatchOutcome` runs, then the decision moves it to its mainstream segment with `EOMMRouteWinEvacuate`.

B-EOMM-11: Given a retention-pool player wins a completed match, when `RouteMatchOutcome` runs, then the decision moves it to its mainstream segment with `EOMMRouteWinEvacuate`.

B-EOMM-12: Given a monetization-pool player completes a match regardless of outcome, when `RouteMatchOutcome` runs, then the decision moves it to its mainstream segment with `EOMMRouteCompleteEvacuate`.

B-EOMM-13: Given `EOMMRouteDecision.Move == false`, when `ApplyRoute` is called, then it does not call Redis and returns `EOMMStatusNoop`.

B-EOMM-14: Given `EOMMRouteDecision.Move == true`, when `ApplyRoute` is called, then it calls `RedisQueueStore.MovePool` with `Member`, `From`, `To`, and score preserved from the decision.

B-EOMM-15: Given Redis movement returns timeout, unavailable, partial, or canceled status, when `ApplyRoute` maps the result, then it returns the corresponding typed EOMM status and does not retry locally.

B-EOMM-16: Given valid candidate context, when `ScoreBreakdown` runs, then it computes `0.4*TrophyPenalty + 0.3*VectorDistance + 0.3*RetentionModifier` terms using configured weights.

B-EOMM-17: Given lower penalty means better match quality, when `ScoreCandidate` fills `MatchCandidateScore`, then higher `Fitness` represents better pairs by negating the weighted penalty.

B-EOMM-18: Given Player A is high churn and Player B is at least `EOMMRetentionTrophyOffset` trophies below A, when scoring the pair, then `RetentionModifier` improves the score by the contracted retention boost.

B-EOMM-19: Given Player A is routed through retention handling, when scoring fills simulation metadata, then `PredictedWinP` moves toward `EOMMRetentionTargetWinP`.

B-EOMM-20: Given Player A is in monetization handling and candidate B is a structural counter with cosine similarity below `EOMMCounterSimilarityThreshold`, when scoring the pair, then `RetentionModifier` worsens the score by the contracted monetization counter penalty.

B-EOMM-21: Given cosine similarity is below the counter threshold, when `ScoreBreakdown` computes vector distance, then it marks the pair as a structural counter through the monetization modifier only; trophy update rules are not changed.

B-EOMM-22: Given Player A is routed through monetization handling, when scoring fills simulation metadata, then `PredictedWinP` moves toward `EOMMMonetizeTargetWinP`.

B-EOMM-23: Given two candidates have similar trophy penalties, when cosine similarity is above `EOMMSimilarSimilarityThreshold`, then vector distance acts as a secondary quality signal and does not dominate trophy proximity.

B-EOMM-24: Given a player's rolling 10-tick win rate drops below `EOMMSpikeWinRateThreshold` and churn risk crosses `EOMMHighChurnThreshold`, when `RecordOutcome` runs, then it emits `EOMMChurnAlertEvent` with `PoolTag == PoolRetention`.

B-EOMM-25: Given fewer than `EOMMSpikeWindowTicks` outcomes are available, when `RecordOutcome` runs, then it updates state but does not emit a churn alert.

B-EOMM-26: Given the rolling window is full, when a new outcome is recorded, then the oldest outcome is removed and win count remains exact without allocation.

B-EOMM-27: Given malformed spike input with zero player ID or nil state/output storage, when `RecordOutcome` runs, then it returns `EOMMStatusInvalidTicket` without mutating unrelated state.

B-EOMM-28: Given candidate context contains a self-match, invalid player IDs, invalid pool metadata, or trophy delta outside tolerance, when `ScoreCandidate` runs, then it rejects the candidate with `MatchCandidateReject`.

## Allocation Budget Table

| Hot-path function or method | Max heap allocation | Justification |
| --- | ---: | --- |
| `EOMMRouter.RouteTicket` | `0 B/op` | Scalar routing over copied ticket metadata. |
| `EOMMRouter.RouteMatchOutcome` | `0 B/op` | Scalar evacuation decision. |
| `EOMMPoolMover.ApplyRoute` helper work before Redis call | `0 B/op` | Builds `RedisMoveRequest` from caller-owned decision. |
| `RedisQueueStore.MovePool` as invoked by EOMM | `<= 256 B/op` | Redis boundary inherited from redisqueue. |
| `EOMMFitnessScorer.ScoreBreakdown` | `0 B/op` | Fixed vector dot product and scalar math. |
| `EOMMFitnessScorer.ScoreCandidate` | `0 B/op` | Stack-only scoring into caller-owned output. |
| `EOMMSpikeDetector.RecordOutcome` | `0 B/op` | Fixed `[10]uint8` rolling window in caller-owned state. |

## Edge Case Register

| Edge case | Contracted handling |
| --- | --- |
| Multiple route predicates true | First-match order: loser, retention, monetization, mainstream. |
| Churn equals threshold | Strict `>` threshold; equality does not route retention. |
| Monetization equals threshold | Strict `>` threshold; equality does not route monetize. |
| Loser starvation exactly 10 ticks | Strict `>` threshold; exactly 10 ticks does not evacuate. |
| Loser pool has at least two peers | Do not starvation-evacuate. |
| Already in target pool | `Move == false`; no Redis move. |
| Redis move timeout | Return `EOMMStatusRedisTimeout`; no local retry. |
| Redis partial/unavailable/NOSCRIPT | Return `EOMMStatusRedisUnavailable`; redisqueue owns recovery. |
| Context canceled | Return `EOMMStatusCanceled` where Redis reports cancellation. |
| Weight sum drift | Reject config before scoring. |
| Cosine similarity outside production range | Clamp vector distance to `[0, 2]`. |
| Self-match | Reject before scoring. |
| Trophy delta beyond tolerance | Reject candidate. |
| Monetization counter | Affects EOMM scoring only, not trophy update or pool evacuation. |
| Retention easier target | Affects scoring and predicted win probability only. |
| Rolling window not full | Update state, emit no alert. |
| Rolling window wrap | Subtract evicted win sample before writing new sample. |
| Churn already high before sample | No alert unless it crosses from `<= threshold` to `> threshold`. |

## Formula Coverage

| Formula or invariant | Behaviour |
| --- | --- |
| Mutually exclusive pool routing | `B-EOMM-1` through `B-EOMM-6` |
| Loser starvation guard `WaitTicks > 10 && peers < 2` | `B-EOMM-9` |
| Weight sum `0.4 + 0.3 + 0.3 == 1.0` | `B-EOMM-7`, `B-EOMM-16` |
| `MatchScore = w1*|ΔTrophies| + w2*VectorDistance + w3*RetentionWeight` | `B-EOMM-16`, `B-EOMM-17` |
| Cosine distance `1.0 - CosineSimilarity` | `B-EOMM-16`, `B-EOMM-20`, `B-EOMM-23` |
| Counter threshold `< 0.2` | `B-EOMM-20`, `B-EOMM-21` |
| Retention target win probability `≈0.7` | `B-EOMM-19` |
| Monetization target win probability `≈0.3` | `B-EOMM-22` |
| Rolling 10-tick win-rate spike | `B-EOMM-24` through `B-EOMM-26` |

## Planner Assumptions

- EOMM may be used as a `MatchFitnessScorer` by matchcore, but this module does
  not rewrite matchcore construction.
- Redisqueue owns actual ZSET movement semantics; EOMM only computes movement
  requests and maps typed statuses.
- Trophy update rules remain outside matchmaking and are not changed by EOMM.
- Simulation will validate aggregate EOMM accuracy metrics later; this module
  emits deterministic predicted win probabilities for targeted scenarios.

## Signature

Signed-off Planner Agent: Project MatchPoint Planner  
Status: Planner contract complete for `eomm`
