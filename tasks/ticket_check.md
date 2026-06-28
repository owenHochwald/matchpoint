# Task: CHECK — ticket

## Status
Planner completed and Orchestrator validated:
- `contracts/ticket_contract.go`
- `contracts/ticket_spec.md`

Implementor completed and Orchestrator validated presence of:
- `internal/ticket/ticket.go`
- `internal/ticket/ticket_test.go`
- `internal/ticket/README.md`
- `go.mod`

`tasks/ticket_implement.md` contains pasted outputs for the four mandatory commands:
- `go test ./... -race -count=1 -timeout 60s`
- `go test ./... -bench=. -benchmem -run='^$' -count=3`
- `go vet ./...`
- `staticcheck ./...`

Orchestrator spot-checks:
- `go test ./... -race -count=1 -timeout 60s` passed locally.
- `go vet ./...` passed locally.
- `staticcheck ./...` exited `0` locally but printed `warning: "./..." matched no packages`; `staticcheck -f stylish ./...` also reported `0 problems`. Audit whether this is a tooling compatibility warning or a handoff failure.

## Your Job
Act as the Checker Agent for Module 1: `ticket`. Produce the adversarial audit report:
- `reports/ticket_checker_report.md`

Run and report the four independent checks required by `docs/AGENTS.md`:

1. Allocation Regression Audit
   - Parse benchmark output from a fresh `go test ./... -bench=. -benchmem -run='^$' -count=3` run.
   - For every contracted `0 B/op` budget in `contracts/ticket_spec.md`, any non-zero allocation is `ALLOC_FAIL`.
   - For non-zero budgets, any value exceeding the budget by more than 10% is `ALLOC_WARN`.

2. Race Detector Stress Run
   - Run `go test -race -count=50 -timeout 300s ./...`.
   - Any data race is `RACE_FAIL`.

3. CPU Profile Spot-Check
   - The canonical command in `docs/AGENTS.md` targets `BenchmarkMatchLoop` under `internal/matchcore`, which does not exist in Module 1.
   - For this module, either run a ticket-relevant CPU profile benchmark or explicitly mark this check not applicable with rationale.
   - Any unexpected syscall or GC-related frame in the top 5 of the ticket hot-path profile is `CPU_WARN`.

4. Contract Conformance
   - Diff exported symbols in `internal/ticket` against `contracts/ticket_contract.go`.
   - Any exported symbol present in implementation but absent from contract, or vice versa, is `CONTRACT_FAIL`.
   - Verify contract behaviours `B-TICKET-1` through `B-TICKET-26` have corresponding tests or documented coverage.

The report must contain an exact verdict line:
- `CHECKER: PASS`
- `CHECKER: WARN`
- `CHECKER: FAIL`

Use `PASS` only for zero FAILs and zero WARNs. Use `WARN` for zero FAILs and at least one WARN. Use `FAIL` for any FAIL.

## Relevant Spec Sections
- `docs/AGENTS.md` Checker Agent responsibilities, verdict matrix, and output artifact
- `docs/FEATURES.md` §2.1 `Ticket`
- `docs/FEATURES.md` §3 Module A — Ingestion Engine
- `docs/FEATURES.md` §10 Memory & Allocation Contracts
- `docs/FEATURES.md` §11 Concurrency Model
- `docs/FEATURES.md` §12 Error Handling & Observability
- `docs/FEATURES.md` §13 Testing Philosophy & Coverage Floors
- `docs/MATCH_SPEC.md` §1.1 Trophy Ladder
- `docs/MATCH_SPEC.md` §2 Deck & Archetype Model
- `docs/MATCH_SPEC.md` §3 Player Intake Interface
- `docs/MATCH_SPEC.md` §5 Key Invariants for the Orchestrator
- `docs/GIT_POLICY.md` Commit and merge policy

## Inputs Available
- `tasks/ticket_plan.md`
- `tasks/ticket_implement.md`
- `contracts/ticket_contract.go`
- `contracts/ticket_spec.md`
- `internal/ticket/ticket.go`
- `internal/ticket/ticket_test.go`
- `internal/ticket/README.md`
- `go.mod`
- `docs/FEATURES.md`
- `docs/MATCH_SPEC.md`
- `docs/AGENTS.md`
- `docs/GIT_POLICY.md`

## Checker Report (if re-cycle)
Not applicable. This is the first Checker dispatch for `ticket`.
