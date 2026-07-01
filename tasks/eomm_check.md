# Task: CHECK — eomm

## Status

Planner completed:
- `contracts/eomm_contract.go`
- `contracts/eomm_spec.md`

Implementor completed:
- `internal/eomm/eomm.go`
- `internal/eomm/eomm_test.go`
- `internal/eomm/README.md`

Orchestrator spot-checks:
- `GOCACHE=/private/tmp/matchpoint-gocache go test ./...` passed.
- `GOCACHE=/private/tmp/matchpoint-gocache go vet ./...` passed.
- `GOCACHE=/private/tmp/matchpoint-gocache STATICCHECK_CACHE=/private/tmp/matchpoint-staticcheck-cache staticcheck ./...` passed.
- `GOCACHE=/private/tmp/matchpoint-gocache go test -race -count=50 -timeout 300s ./internal/eomm` passed.
- `GOCACHE=/private/tmp/matchpoint-gocache go test ./... -bench=. -benchmem -run='^$' -count=1` passed.

## Your Job

Act as the Checker Agent for Module 5: `eomm`. Produce the adversarial audit
report:
- `reports/eomm_checker_report.md`

Run and report the four independent checks required by `docs/AGENTS.md`:

1. Allocation Regression Audit
   - Parse benchmark output from a fresh `go test ./... -bench=. -benchmem -run='^$'` run.
   - For every contracted `0 B/op` budget in `contracts/eomm_spec.md`, any non-zero allocation is `ALLOC_FAIL`.

2. Race Detector Stress Run
   - Run `go test -race -count=50 -timeout 300s ./internal/eomm`.
   - Any data race is `RACE_FAIL`.

3. CPU Profile Spot-Check
   - Run an EOMM hot-path CPU profile benchmark such as `BenchmarkScoreCandidateGOMAXPROCS1`.
   - If `go tool pprof` is unavailable, use `go tool preprofile` and record the tooling limitation.

4. Contract Conformance
   - Diff exported symbols in `internal/eomm` against `contracts/eomm_contract.go`.
   - Verify behaviours `B-EOMM-1` through `B-EOMM-28` have corresponding tests.
   - Confirm `internal/eomm` does not import concrete internal packages.

The report must contain an exact verdict line:
- `CHECKER: PASS`
- `CHECKER: WARN`
- `CHECKER: FAIL`

Use `PASS` only for zero FAILs and zero WARNs. Use `WARN` for zero FAILs and at
least one WARN. Use `FAIL` for any FAIL.

## Relevant Spec Sections

- `docs/AGENTS.md` Checker Agent responsibilities
- `docs/FEATURES.md` §5 Module C — EOMM
- `contracts/eomm_contract.go`
- `contracts/eomm_spec.md`
- `reports/matchcore_checker_report.md`

## Inputs Available

- `tasks/eomm_plan.md`
- `tasks/eomm_implement.md`
- `contracts/eomm_contract.go`
- `contracts/eomm_spec.md`
- `internal/eomm/eomm.go`
- `internal/eomm/eomm_test.go`
- `internal/eomm/README.md`

## Checker Report (if re-cycle)

Not applicable. This is the first Checker dispatch for `eomm`.
