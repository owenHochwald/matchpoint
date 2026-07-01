# Task: PLAN — eomm

## Status

Previous modules are delivered:
- `ticket`: `reports/ticket_checker_report.md` verdict is `CHECKER: WARN`.
- `ringbuffer`: `reports/ringbuffer_checker_report.md` verdict is `CHECKER: WARN`.
- `redisqueue`: `reports/redisqueue_checker_report.md` verdict is `CHECKER: WARN`.
- `matchcore`: `reports/matchcore_checker_report.md` verdict is `CHECKER: WARN`.

All reports have zero FAILs. Current module: `eomm` — loser's pool, retention
matches, monetization triggers, and EOMM fitness scoring.

## Your Job

Produce the signed Planner contract for Module 5: `eomm`.

Write exactly these output artifacts:
- `contracts/eomm_contract.go`
- `contracts/eomm_spec.md`

The contract must include only pure Go type/interface contracts: interface
definitions, struct layouts with field-level invariant comments, method
signatures with explicit parameter and return types, and no function bodies.

Contract scope:
- Mutually exclusive EOMM pool routing in loser, retention, monetization,
  then mainstream order.
- Special-pool movement through the existing `RedisQueueStore.MovePool`
  boundary only.
- Loser's pool peer-only matching metadata and starvation evacuation after
  more than 10 ticks when fewer than two other loser-pool players are present.
- Match-completion evacuation rules for loser, retention, and monetization
  pools.
- EOMM fitness scoring using trophy proximity, vector distance, and
  retention/monetization modifiers.
- Weight validation with the fixed `0.4 + 0.3 + 0.3 == 1.0` default.
- Retention target shaping and monetization structural-counter shaping without
  changing trophy update rules.
- Rolling 10-tick win-rate spike detection and churn alert emission boundary.
- No concrete imports from `internal/matchcore`, `internal/redisqueue`,
  future `internal/vectorarch`, simulation, or telemetry.

## Relevant Spec Sections

- `docs/FEATURES.md` §5 Module C — EOMM
- `docs/FEATURES.md` §5.1 Pool Routing
- `docs/FEATURES.md` §5.2 The Loser's Pool
- `docs/FEATURES.md` §5.3 EOMM Fitness Function
- `docs/FEATURES.md` §5.4 Adaptive Spike Detection
- `docs/FEATURES.md` §6.2 Cosine Similarity
- `docs/FEATURES.md` §6.3 Counter Archetype Detection
- `docs/MATCH_SPEC.md` §4.2 EOMM Accuracy Metrics
- `docs/MATCH_SPEC.md` §5 Key Invariants
- `contracts/ticket_contract.go`
- `contracts/redisqueue_contract.go`
- `contracts/matchcore_contract.go`

## Inputs Available

- `docs/FEATURES.md`
- `docs/MATCH_SPEC.md`
- `docs/AGENTS.md`
- `docs/GIT_POLICY.md`
- `docs/IMPLEMENTATION_STATUS.md`
- `contracts/ticket_contract.go`
- `contracts/redisqueue_contract.go`
- `contracts/matchcore_contract.go`
- `reports/matchcore_checker_report.md`

## Checker Report (if re-cycle)

Not applicable. This is the first Planner dispatch for `eomm`.
