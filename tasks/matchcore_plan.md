# Task: PLAN — matchcore

## Status
Previous modules are delivered:
- `ticket`: `reports/ticket_checker_report.md` verdict is `CHECKER: WARN`.
- `ringbuffer`: `reports/ringbuffer_checker_report.md` verdict is `CHECKER: WARN`.
- `redisqueue`: `reports/redisqueue_checker_report.md` verdict is `CHECKER: WARN`.

All reports have zero FAILs. WARN annotations have been appended to the module
spec files, and `docs/IMPLEMENTATION_STATUS.md` has been updated.

Current module: `matchcore` — 200ms tick loop + exponential tolerance expansion.

## Your Job
Produce the signed Planner contract for Module 4: `matchcore`.

Write exactly these output artifacts:
- `contracts/matchcore_contract.go`
- `contracts/matchcore_spec.md`

The contract must include only pure Go type/interface contracts: interface
definitions, struct layouts with field-level invariant comments, method
signatures with explicit parameter and return types, and no function bodies
except trivial zero-value constructors if absolutely needed.

Contract scope:
- 200ms master tick loop semantics.
- Tick overrun detection, skipped-next-tick behaviour, and consecutive-overrun
  warning threshold.
- Ringbuffer drain integration through `contracts.TicketRingBuffer`.
- Redis queue integration through `contracts.RedisQueueStore`,
  `contracts.RedisQueueKeyer`, and `contracts.RedisScoreCodec`.
- Exponential tolerance formula:
  `Tolerance(t) = BaseTolerance * exp(k*t)`, clamped to `MaxTolerance = 2000`.
- Overflow guard for `k*t > 10.0` and safe `math.Min` behaviour.
- Candidate query range generation with tolerance in trophies handed to
  redisqueue score-range codec.
- Candidate scoring boundary that can later call EOMM/vector modules by
  interface only; no concrete downstream package imports.
- Atomic match assignment call boundary and dual-booking handling.
- Tick metrics: duration, overrun count, skipped ticks, matches made, drained
  tickets, dual bookings, and empty queries.
- No direct Redis/go-redis or internal module imports in the public contract.

The behaviour spec must include:
- At least 5 numbered behaviours in the exact format
  `B-MATCHCORE-N: Given <precondition>, when <action>, then <observable outcome>.`
- Behaviour coverage for every public symbol in `contracts/matchcore_contract.go`.
- Behaviour coverage for every formula in the relevant spec sections.
- Allocation Budget Table for every hot-path function.
- Edge Case Register covering tick drift, skipped ticks, three consecutive
  overruns, `k*t` overflow, zero/negative waits, empty ring shards, empty Redis
  candidate results, dual booking, Redis timeout/unavailable statuses, candidate
  tie-breaking, invalid tolerance config, and future EOMM/vector integration
  boundaries.

Do not write implementation files under `internal/`.

## Relevant Spec Sections
- `docs/FEATURES.md` §4 Module B — Match Core Loop (200ms Tick)
- `docs/FEATURES.md` §4.1 Master Ticker
- `docs/FEATURES.md` §4.2 Queue Segment Architecture
- `docs/FEATURES.md` §4.3 Exponential Tolerance Expansion
- `docs/FEATURES.md` §4.4 Match Candidate Selection
- `docs/FEATURES.md` §5.3 EOMM Fitness Function (interface boundary only)
- `docs/FEATURES.md` §6.2 Cosine Similarity (interface boundary only)
- `docs/FEATURES.md` §9.3 Atomic Match Assignment
- `docs/FEATURES.md` §10.1 Hot-Path Allocation Budget
- `docs/FEATURES.md` §11 Concurrency Model
- `docs/FEATURES.md` §12 Error Handling & Observability
- `docs/MATCH_SPEC.md` §4.1 Matchmaking Quality Metrics
- `docs/MATCH_SPEC.md` §4.3 System Performance Metrics
- `docs/AGENTS.md` Planner Agent responsibilities and exit criteria
- `docs/GIT_POLICY.md` Commit granularity and branch policy
- `contracts/ringbuffer_contract.go`
- `contracts/redisqueue_contract.go`
- `contracts/ticket_contract.go`
- `reports/ringbuffer_checker_report.md`
- `reports/redisqueue_checker_report.md`

## Inputs Available
- `contracts/ticket_contract.go`
- `contracts/ringbuffer_contract.go`
- `contracts/redisqueue_contract.go`
- `reports/ticket_checker_report.md`
- `reports/ringbuffer_checker_report.md`
- `reports/redisqueue_checker_report.md`
- `docs/FEATURES.md`
- `docs/MATCH_SPEC.md`
- `docs/AGENTS.md`
- `docs/GIT_POLICY.md`
- `docs/IMPLEMENTATION_STATUS.md`

## Checker Report (if re-cycle)
Not applicable. This is the first Planner dispatch for `matchcore`.
