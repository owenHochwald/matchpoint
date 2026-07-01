# Task: PLAN — redisqueue

## Status
Previous modules are delivered:
- `ticket`: `reports/ticket_checker_report.md` verdict is `CHECKER: WARN`.
- `ringbuffer`: `reports/ringbuffer_checker_report.md` verdict is `CHECKER: WARN`.

Both reports have zero FAILs. The known inherited warning is that
`staticcheck ./...` exits `0` while matching no packages under the current
Go/staticcheck toolchain. Treat that as a tooling warning unless resolved.

Current module: `redisqueue` — Redis ZSET priority queue + Lua atomic scripts.

## Your Job
Produce the signed Planner contract for Module 3: `redisqueue`.

Write exactly these output artifacts:
- `contracts/redisqueue_contract.go`
- `contracts/redisqueue_spec.md`

The contract must include only pure Go type/interface contracts: interface
definitions, struct layouts with field-level invariant comments, method
signatures with explicit parameter and return types, and no function bodies
except trivial zero-value constructors if absolutely needed.

Contract scope:
- Redis 7.0+ single-instance queue layer using `go-redis/v9`.
- Trophy segment ZSET key schema for `mq:seg:0` through `mq:seg:4`.
- Special pool ZSET key schema for `mq:losers`, `mq:retention`, and
  `mq:monetize`.
- Player metadata hash and match record hash contracts.
- Score encoding: `Trophies * 1e6 + EnqueuedAt_microseconds_truncated`.
- Enqueue, remove, range query, candidate fetch, special-pool movement, and
  atomic match assignment contracts.
- Lua script source contract and SHA loading/cache contract.
- `SCRIPT LOAD` startup behaviour and `EVALSHA` match assignment behaviour.
- Timeout/latency budget semantics: Redis command timeout is 5ms; slow commands
  return typed timeout status and increment latency counters in implementation.
- Pipeline batching boundary for tick handler query batches.
- Dual-booking/race guard semantics when the Lua script returns `0`.
- Deterministic typed statuses/errors with no dynamic error formatting on hot
  paths.

The behaviour spec must include:
- At least 5 numbered behaviours in the exact format
  `B-REDISQUEUE-N: Given <precondition>, when <action>, then <observable outcome>.`
- Behaviour coverage for every public symbol in `contracts/redisqueue_contract.go`.
- Behaviour coverage for every formula/key/script in the relevant spec sections.
- Allocation Budget Table for every hot-path function.
- Edge Case Register covering Redis timeout, script cache miss, `NOSCRIPT`,
  dual booking, ZSET score precision, segment boundary trophies, empty query
  results, pool movement idempotency, pipeline partial errors, context
  cancellation, and Redis unavailability.

Do not write implementation files under `internal/`.

## Relevant Spec Sections
- `docs/FEATURES.md` §4.2 Queue Segment Architecture
- `docs/FEATURES.md` §4.4 Match Candidate Selection
- `docs/FEATURES.md` §5.1 Pool Routing
- `docs/FEATURES.md` §5.2 The Loser's Pool
- `docs/FEATURES.md` §9 Storage Layer Contract
- `docs/FEATURES.md` §10.1 Hot-Path Allocation Budget (`TickHandler` Redis boundary)
- `docs/FEATURES.md` §11.2 Shared State Inventory (`Redis ZSET state`)
- `docs/FEATURES.md` §12.1 Error Taxonomy
- `docs/FEATURES.md` §13.2 Required Test Types (`testcontainers-go` Redis integration)
- `docs/FEATURES.md` §14 Delivery Sequence & Dependency Graph
- `docs/MATCH_SPEC.md` §4.3 System Performance Metrics (`Redis command latency p99`)
- `docs/AGENTS.md` Planner Agent responsibilities and exit criteria
- `docs/GIT_POLICY.md` Commit granularity and branch policy
- `contracts/ticket_contract.go`
- `contracts/ringbuffer_contract.go`
- `reports/ticket_checker_report.md`
- `reports/ringbuffer_checker_report.md`

## Inputs Available
- `contracts/ticket_contract.go`
- `contracts/ringbuffer_contract.go`
- `reports/ticket_checker_report.md`
- `reports/ringbuffer_checker_report.md`
- `docs/FEATURES.md`
- `docs/MATCH_SPEC.md`
- `docs/AGENTS.md`
- `docs/GIT_POLICY.md`
- `docs/IMPLEMENTATION_STATUS.md`

## Checker Report (if re-cycle)
Not applicable. This is the first Planner dispatch for `redisqueue`.
