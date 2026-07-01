# Task: IMPLEMENT — matchcore

## Status
Planner completed the `matchcore` contract and spec. Orchestrator validation passed:
- `contracts/matchcore_contract.go` exists.
- `contracts/matchcore_contract.go` contains no `func` bodies.
- `contracts/matchcore_spec.md` exists.
- `contracts/matchcore_spec.md` contains 38 `B-MATCHCORE-N:` behaviours.
- `contracts/matchcore_spec.md` contains an Allocation Budget Table.
- `contracts/matchcore_spec.md` contains an Edge Case Register.
- `GOCACHE=/private/tmp/matchpoint-gocache go test ./contracts` passed.

Read-only scout findings to account for:
- Implement a pure single-tick `HandleTick` path separate from real ticker-loop orchestration.
- Add a canonical `BenchmarkMatchLoop` because Checker has a matchcore-specific CPU profile command.
- Use deterministic fake ring/Redis/scorer dependencies; default tests must not require Redis, Docker, network, or real time sleeps.
- Matchcore must pass tolerance in trophy units to `RedisScoreCodec.ScoreRange`; redisqueue owns score scaling by `1e6`.
- Matchcore must not import `internal/ringbuffer`, `internal/redisqueue`, `internal/ticket`, future `internal/eomm`, or future `internal/vectorarch`.
- `TicketRingBuffer` has no shard-count discovery; implementation config must own shard count.
- `RedisCandidate` lacks full ticket payload, so implementation should use contract-provided `RedisQueueEntry` candidate context through fakes/boundaries and stay within Planner scope.
- Restore `GOMAXPROCS` in benchmarks to avoid the redisqueue `BENCH_WARN`.

## Your Job
Implement Module 4: `matchcore` exactly against the signed Planner artifacts:
- `contracts/matchcore_contract.go`
- `contracts/matchcore_spec.md`

Write implementation artifacts:
- `internal/matchcore/matchcore.go`
- `internal/matchcore/matchcore_test.go`
- `internal/matchcore/README.md`

Implementation requirements:
- Write tests covering every `B-MATCHCORE-N:` behaviour before implementation.
- Include at least one `BenchmarkXxx` for every hot-path function in the allocation budget table.
- Include `BenchmarkMatchLoop`, `BenchmarkMatchLoopGOMAXPROCS1`, and `BenchmarkMatchLoopGOMAXPROCSCPU`.
- Keep `BenchmarkMatchLoop` fake-only, deterministic, no network, no timers, no goroutines, and no allocation in the loop except justified Redis-bound budget paths.
- Use `matchpoint/contracts` only for cross-module types and interfaces.
- Implement fixed scratch buffers for drained tickets, Redis entries, score ranges, candidate results, and match assignment results.
- Do not allocate on pure helper hot paths: tolerance calculation, ring drain into caller storage, candidate query preparation, candidate tie-breaking, baseline scorer, assigner helper before Redis call, metrics, match ID generation, and state snapshot.
- Do not use dynamic error formatting on hot paths.
- Overrun/skip behaviour must be tested without real 200ms sleeps, using injected/fake timestamps or duration source.
- Redis timeout/unavailable/partial/canceled/dual-booking statuses must map to typed matchcore statuses and metrics without local retry loops.
- Future EOMM/vector integration must be interface-only via `MatchFitnessScorer` and `MatchVectorScorer`.

Before handoff, paste the full outputs of these mandatory commands into this task file under a new `## Implementor Command Outputs` section:

```bash
GOCACHE=/private/tmp/matchpoint-gocache go test ./... -race -count=1 -timeout 60s
GOCACHE=/private/tmp/matchpoint-gocache go test ./... -bench=. -benchmem -run='^$' -count=3
GOCACHE=/private/tmp/matchpoint-gocache go vet ./...
GOCACHE=/private/tmp/matchpoint-gocache STATICCHECK_CACHE=/private/tmp/matchpoint-staticcheck-cache staticcheck ./...
```

All four must show zero errors/warnings. If any benchmark emits `GOMAXPROCS`
hygiene warnings, fix the benchmark before handoff.

## Relevant Spec Sections
- `docs/FEATURES.md` §4 Module B — Match Core Loop
- `docs/FEATURES.md` §4.1 Master Ticker
- `docs/FEATURES.md` §4.2 Queue Segment Architecture
- `docs/FEATURES.md` §4.3 Exponential Tolerance Expansion
- `docs/FEATURES.md` §4.4 Match Candidate Selection
- `docs/FEATURES.md` §5.3 EOMM Fitness Function
- `docs/FEATURES.md` §6.2 Cosine Similarity
- `docs/FEATURES.md` §9.3 Atomic Match Assignment
- `docs/FEATURES.md` §10.1 Hot-Path Allocation Budget
- `docs/FEATURES.md` §11 Concurrency Model
- `docs/FEATURES.md` §12 Error Handling & Observability
- `docs/MATCH_SPEC.md` §4.1 Matchmaking Quality Metrics
- `docs/MATCH_SPEC.md` §4.3 System Performance Metrics
- `docs/AGENTS.md` Implementor Agent responsibilities and exit criteria
- `docs/GIT_POLICY.md` Commit granularity and branch policy
- `contracts/matchcore_contract.go`
- `contracts/matchcore_spec.md`
- `contracts/ringbuffer_contract.go`
- `contracts/redisqueue_contract.go`
- `reports/redisqueue_checker_report.md`

## Inputs Available
- `tasks/matchcore_plan.md`
- `contracts/matchcore_contract.go`
- `contracts/matchcore_spec.md`
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
Not applicable. This is the first Implementor dispatch for `matchcore`.
