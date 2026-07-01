# Task: CHECK — redisqueue

## Status
Planner completed and Orchestrator validated:
- `contracts/redisqueue_contract.go`
- `contracts/redisqueue_spec.md`

Implementor completed and Orchestrator validated presence of:
- `internal/redisqueue/redisqueue.go`
- `internal/redisqueue/redisqueue_test.go`
- `internal/redisqueue/README.md`
- `go.mod`
- `go.sum`

`tasks/redisqueue_implement.md` contains pasted outputs for the four mandatory commands:
- `GOCACHE=/private/tmp/matchpoint-gocache go test ./... -race -count=1 -timeout 60s`
- `GOCACHE=/private/tmp/matchpoint-gocache go test ./... -bench=. -benchmem -run='^$' -count=3`
- `GOCACHE=/private/tmp/matchpoint-gocache go vet ./...`
- `GOCACHE=/private/tmp/matchpoint-gocache STATICCHECK_CACHE=/private/tmp/matchpoint-staticcheck-cache staticcheck ./...`

Orchestrator spot-checks:
- `GOCACHE=/private/tmp/matchpoint-gocache go test ./... -race -count=1 -timeout 60s` passed locally.
- `GOCACHE=/private/tmp/matchpoint-gocache go vet ./...` passed locally.
- `GOCACHE=/private/tmp/matchpoint-gocache STATICCHECK_CACHE=/private/tmp/matchpoint-staticcheck-cache staticcheck ./...` passed locally with no output.
- Representative redisqueue benchmarks showed pure helper paths at `0 B/op`, and Redis-bound fake paths within contracted non-zero budgets.

## Your Job
Act as the Checker Agent for Module 3: `redisqueue`. Produce the adversarial audit report:
- `reports/redisqueue_checker_report.md`

Run and report the four independent checks required by `docs/AGENTS.md`:

1. Allocation Regression Audit
   - Parse benchmark output from a fresh `GOCACHE=/private/tmp/matchpoint-gocache go test ./... -bench=. -benchmem -run='^$' -count=3` run.
   - For every contracted `0 B/op` budget in `contracts/redisqueue_spec.md`, any non-zero allocation is `ALLOC_FAIL`.
   - For non-zero redis-bound budgets, any value exceeding the budget by more than 10% is `ALLOC_WARN`.
   - Include inherited ticket/ringbuffer benchmarks if they appear in `./...`.

2. Race Detector Stress Run
   - Run `GOCACHE=/private/tmp/matchpoint-gocache go test -race -count=50 -timeout 300s ./...`.
   - Any data race is `RACE_FAIL`.

3. CPU Profile Spot-Check
   - The canonical command in `docs/AGENTS.md` targets `BenchmarkMatchLoop` under `internal/matchcore`, which does not exist yet.
   - For this module, run a redisqueue hot-path CPU profile benchmark such as `BenchmarkAssignMatchFakeGOMAXPROCS1`, `BenchmarkFetchCandidateBatchFakeGOMAXPROCS1`, or pure score/key helpers.
   - Any unexpected syscall or GC-related frame in the top 5 of the redisqueue hot-path profile is `CPU_WARN`.

4. Contract Conformance
   - Diff exported symbols in `internal/redisqueue` against `contracts/redisqueue_contract.go`.
   - Any exported symbol present in implementation but absent from contract, or vice versa, is `CONTRACT_FAIL`.
   - Verify behaviours `B-REDISQUEUE-1` through `B-REDISQUEUE-40` have corresponding tests or documented coverage.
   - Confirm `internal/redisqueue` does not import `internal/ticket` or `internal/ringbuffer`.
   - Confirm default tests do not require Docker or a live Redis server.

Also audit:
- `staticcheck` must be run with `STATICCHECK_CACHE=/private/tmp/matchpoint-staticcheck-cache`; do not accept the prior no-package warning state as clean analysis.
- `github.com/redis/go-redis/v9` dependency usage should not leak into the public contracts package.
- Same-player assignment rejection, `NOSCRIPT`, timeout, cancellation, dual booking, score precision, score-range scaling by `1e6`, and copy-on-enqueue ownership semantics should be covered.

The report must contain an exact verdict line:
- `CHECKER: PASS`
- `CHECKER: WARN`
- `CHECKER: FAIL`

Use `PASS` only for zero FAILs and zero WARNs. Use `WARN` for zero FAILs and at least one WARN. Use `FAIL` for any FAIL.

## Relevant Spec Sections
- `docs/AGENTS.md` Checker Agent responsibilities, verdict matrix, and output artifact
- `docs/FEATURES.md` §4.2 Queue Segment Architecture
- `docs/FEATURES.md` §4.4 Match Candidate Selection
- `docs/FEATURES.md` §5.1 Pool Routing
- `docs/FEATURES.md` §5.2 The Loser's Pool
- `docs/FEATURES.md` §9 Storage Layer Contract
- `docs/FEATURES.md` §10.1 Hot-Path Allocation Budget
- `docs/FEATURES.md` §11.2 Shared State Inventory
- `docs/FEATURES.md` §12.1 Error Taxonomy
- `contracts/redisqueue_contract.go`
- `contracts/redisqueue_spec.md`
- `reports/ticket_checker_report.md`
- `reports/ringbuffer_checker_report.md`

## Inputs Available
- `tasks/redisqueue_plan.md`
- `tasks/redisqueue_implement.md`
- `contracts/redisqueue_contract.go`
- `contracts/redisqueue_spec.md`
- `internal/redisqueue/redisqueue.go`
- `internal/redisqueue/redisqueue_test.go`
- `internal/redisqueue/README.md`
- `go.mod`
- `go.sum`
- `docs/FEATURES.md`
- `docs/MATCH_SPEC.md`
- `docs/AGENTS.md`
- `docs/GIT_POLICY.md`
- `docs/IMPLEMENTATION_STATUS.md`

## Checker Report (if re-cycle)
Not applicable. This is the first Checker dispatch for `redisqueue`.
