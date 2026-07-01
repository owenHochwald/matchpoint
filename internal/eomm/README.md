# EOMM

`eomm` owns special-pool routing and engagement-aware candidate scoring. It
uses only `matchpoint/contracts` so matchcore can consume it through the
existing `MatchFitnessScorer` boundary without importing this package directly.

## Invariants

- Routing is mutually exclusive in loser, retention, monetization, mainstream
  order.
- Redis movement is represented as `RedisMoveRequest`; redisqueue owns command
  execution and retry/script semantics.
- Scoring treats lower weighted penalty as better and exposes it to matchcore as
  higher `Fitness` by negating the penalty.
- Retention and monetization shape predicted win probability for simulation
  accuracy; they do not change trophy update rules.
- Adaptive spike detection uses caller-owned `[10]uint8` storage and performs no
  heap allocation on update.
