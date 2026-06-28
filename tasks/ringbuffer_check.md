# Task: CHECK — ringbuffer

## Status
Planner completed and Orchestrator validated:
- `contracts/ringbuffer_contract.go`
- `contracts/ringbuffer_spec.md`

Implementor completed and Orchestrator validated presence of:
- `internal/ringbuffer/ringbuffer.go`
- `internal/ringbuffer/ringbuffer_test.go`
- `internal/ringbuffer/README.md`

`tasks/ringbuffer_implement.md` contains pasted outputs for the four mandatory commands:
- `go test ./... -race -count=1 -timeout 60s`
- `go test ./... -bench=. -benchmem -run='^$' -count=3`
- `go vet ./...`
- `staticcheck ./...`

Orchestrator spot-checks:
- `GOCACHE=/private/tmp/matchpoint-gocache go test ./... -race -count=1 -timeout 60s` passed locally.
- `GOCACHE=/private/tmp/matchpoint-gocache go vet ./...` passed locally.
- `GOCACHE=/private/tmp/matchpoint-gocache go test ./internal/ringbuffer -run Test -count=1 -timeout 60s` passed locally.
- `GOCACHE=/private/tmp/matchpoint-gocache go test ./internal/ringbuffer -bench=BenchmarkShardForPlayerGOMAXPROCS1 -benchmem -run='^$' -count=1` reported `0 B/op`.
- `staticcheck ./...` still exits `0` while printing `warning: "./..." matched no packages`; audit as a warning unless you can establish package analysis actually ran.

## Your Job
Act as the Checker Agent for Module 2: `ringbuffer`. Produce the adversarial audit report:
- `reports/ringbuffer_checker_report.md`

Run and report the four independent checks required by `docs/AGENTS.md`:

1. Allocation Regression Audit
   - Parse benchmark output from a fresh `go test ./... -bench=. -benchmem -run='^$' -count=3` run.
   - For every contracted `0 B/op` budget in `contracts/ringbuffer_spec.md`, any non-zero allocation is `ALLOC_FAIL`.
   - Include inherited ticket benchmarks in the audit if they appear in `./...`.

2. Race Detector Stress Run
   - Run `go test -race -count=50 -timeout 300s ./...`.
   - Any data race is `RACE_FAIL`.

3. CPU Profile Spot-Check
   - The canonical command in `docs/AGENTS.md` targets `BenchmarkMatchLoop` under `internal/matchcore`, which does not exist yet.
   - For this module, run a ringbuffer hot-path CPU profile benchmark such as write/read/drain, or explicitly mark the canonical check not applicable with rationale.
   - Any unexpected syscall or GC-related frame in the top 5 of the ringbuffer hot-path profile is `CPU_WARN`.

4. Contract Conformance
   - Diff exported symbols in `internal/ringbuffer` against `contracts/ringbuffer_contract.go`.
   - Any exported symbol present in implementation but absent from contract, or vice versa, is `CONTRACT_FAIL`.
   - Verify behaviours `B-RINGBUFFER-1` through `B-RINGBUFFER-30` have corresponding tests or documented coverage.
   - Confirm `internal/ringbuffer` does not import `internal/ticket`.

The report must contain an exact verdict line:
- `CHECKER: PASS`
- `CHECKER: WARN`
- `CHECKER: FAIL`

Use `PASS` only for zero FAILs and zero WARNs. Use `WARN` for zero FAILs and at least one WARN. Use `FAIL` for any FAIL.

## Relevant Spec Sections
- `docs/AGENTS.md` Checker Agent responsibilities, verdict matrix, and output artifact
- `docs/FEATURES.md` §3.4 Ingestion → Queue Handoff
- `docs/FEATURES.md` §3.5 Backpressure
- `docs/FEATURES.md` §10 Memory & Allocation Contracts
- `docs/FEATURES.md` §11 Concurrency Model
- `contracts/ringbuffer_contract.go`
- `contracts/ringbuffer_spec.md`
- `reports/ticket_checker_report.md`

## Inputs Available
- `tasks/ringbuffer_plan.md`
- `tasks/ringbuffer_implement.md`
- `contracts/ringbuffer_contract.go`
- `contracts/ringbuffer_spec.md`
- `internal/ringbuffer/ringbuffer.go`
- `internal/ringbuffer/ringbuffer_test.go`
- `internal/ringbuffer/README.md`
- `reports/ticket_checker_report.md`
- `go.mod`
- `docs/FEATURES.md`
- `docs/MATCH_SPEC.md`
- `docs/AGENTS.md`
- `docs/GIT_POLICY.md`

## Checker Report (if re-cycle)
Not applicable. This is the first Checker dispatch for `ringbuffer`.
