# Task: CHECK — matchcore

## Status

Planner completed and Orchestrator validated:
- `contracts/matchcore_contract.go`
- `contracts/matchcore_spec.md`

Implementor completed and Orchestrator validated presence of:
- `internal/matchcore/matchcore.go`
- `internal/matchcore/matchcore_test.go`
- `internal/matchcore/README.md`

Orchestrator spot-checks:
- `GOCACHE=/private/tmp/matchpoint-gocache go test ./internal/matchcore ./contracts` passed.
- `GOCACHE=/private/tmp/matchpoint-gocache go test -race ./internal/matchcore` passed.
- `GOCACHE=/private/tmp/matchpoint-gocache go vet ./internal/matchcore ./contracts` passed.
- `GOCACHE=/private/tmp/matchpoint-gocache STATICCHECK_CACHE=/private/tmp/matchpoint-staticcheck-cache staticcheck ./...` passed with no output.
- `GOCACHE=/private/tmp/matchpoint-gocache go test ./... -bench=. -benchmem -run='^$' -count=3` passed.

Checker re-cycle note:
- A checker found that integrated `HandleTick` dropped Redis enqueue error status and did not record original Redis assignment statuses. The implementation was updated before this checker dispatch.

## Your Job

Act as the Checker Agent for Module 4: `matchcore`. Produce the adversarial audit report:
- `reports/matchcore_checker_report.md`

Run and report the four independent checks required by `docs/AGENTS.md`:

1. Allocation Regression Audit
   - Parse benchmark output from a fresh `go test ./... -bench=. -benchmem -run='^$' -count=3` run.
   - For every contracted `0 B/op` budget in `contracts/matchcore_spec.md`, any non-zero allocation is `ALLOC_FAIL`.
   - `BenchmarkMatchLoop` is permitted to allocate below the signed `<512 B/op` loop budget.

2. Race Detector Stress Run
   - Run `go test -race -count=50 -timeout 300s ./...` or a targeted equivalent with rationale.
   - Any data race is `RACE_FAIL`.

3. CPU Profile Spot-Check
   - Run the canonical matchcore CPU profile command against `BenchmarkMatchLoop`.
   - If `go tool pprof` is unavailable, use `go tool preprofile` and record the tooling limitation.
   - Unexpected syscall or GC-related sampled frames are `CPU_WARN`.

4. Contract Conformance
   - Diff exported symbols in `internal/matchcore` against `contracts/matchcore_contract.go`.
   - Any exported symbol present in implementation but absent from contract, or vice versa, is `CONTRACT_FAIL`.
   - Verify behaviours `B-MATCHCORE-1` through `B-MATCHCORE-38` have corresponding tests or documented coverage.
   - Confirm `internal/matchcore` does not import concrete upstream or future internal packages.

The report must contain an exact verdict line:
- `CHECKER: PASS`
- `CHECKER: WARN`
- `CHECKER: FAIL`

Use `PASS` only for zero FAILs and zero WARNs. Use `WARN` for zero FAILs and at least one WARN. Use `FAIL` for any FAIL.

## Relevant Spec Sections

- `docs/AGENTS.md` Checker Agent responsibilities, verdict matrix, and output artifact
- `docs/FEATURES.md` §4 Module B — Match Core Loop
- `docs/FEATURES.md` §10 Memory & Allocation Contracts
- `docs/FEATURES.md` §11 Concurrency Model
- `docs/FEATURES.md` §12 Error Handling & Observability
- `contracts/matchcore_contract.go`
- `contracts/matchcore_spec.md`
- `reports/redisqueue_checker_report.md`

## Inputs Available

- `tasks/matchcore_plan.md`
- `tasks/matchcore_implement.md`
- `contracts/matchcore_contract.go`
- `contracts/matchcore_spec.md`
- `internal/matchcore/matchcore.go`
- `internal/matchcore/matchcore_test.go`
- `internal/matchcore/README.md`
- `reports/redisqueue_checker_report.md`
- `go.mod`
- `docs/FEATURES.md`
- `docs/MATCH_SPEC.md`
- `docs/AGENTS.md`
- `docs/GIT_POLICY.md`

## Checker Report (if re-cycle)

Not applicable. This is the first Checker dispatch for `matchcore`.
