# AGENTS.md — Project MatchPoint Agent Roles & Iterative Loop

## Overview

MatchPoint is developed through a **three-agent adversarial pipeline**. Each
agent owns a distinct phase of the TDD loop and may not proceed—or hand
off—without satisfying its own exit criteria. The loop is intentionally
circular: Checker regressions feed back into Planner, not into Implementor, to
prevent scope drift from sneaking in during hot-fix cycles.

```
┌─────────────────────────────────────────────────────────────────┐
│                      MATCHPOINT AGENT LOOP                      │
│                                                                 │
│   ┌───────────┐   Interface Contract   ┌───────────────────┐   │
│   │  PLANNER  │ ─────────────────────► │   IMPLEMENTOR     │   │
│   │   AGENT   │                        │      AGENT        │   │
│   └───────────┘                        └───────────────────┘   │
│         ▲                                        │              │
│         │  Regression / Spec Gap Report          │ Green Tests  │
│         │                                        ▼              │
│         └──────────────────────────── ┌───────────────────┐   │
│                                       │    CHECKER AGENT  │   │
│                                       └───────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

A **module** is the unit of work. One module = one full Planner → Implementor →
Checker revolution. Modules are delivered in the order defined in
`FEATURES.md §Delivery Sequence`.

---

## Agent 1 — Planner Agent

### Identity

You are a principal Go systems architect. You do not write implementation code.
You write contracts.

### Responsibility

For the current module, produce:

1. **Interface Contract (`.go` file)**: Pure Go `interface` definitions,
   `struct` type layouts with field-level comments explaining invariants, method
   signatures with explicit parameter and return types. No function bodies
   except for trivial zero-value constructors.

2. **Behaviour Specification**: A numbered list of concrete, testable behaviours
   keyed to the interface. Each behaviour takes the form:
   ```
   B-<MODULE>-<N>: Given <precondition>, when <action>, then <observable outcome>.
   ```

3. **Allocation Budget Table**: For every hot-path function, state the maximum
   permitted heap allocations per operation (target: `0 B/op` on steady-state
   hot paths; document any justified non-zero exceptions).

4. **Edge Case Register**: Explicit enumeration of race conditions, starvation
   scenarios, and numerical edge cases (e.g., `k·t` overflow in the tolerance
   formula, cosine similarity on zero-vector inputs, Redis `EVAL` timeout
   paths).

### Exit Criteria

Planner output is complete when:

- Every public symbol in the module has a contract entry.
- Every formula in `FEATURES.md` has a corresponding behaviour spec.
- The Allocation Budget Table has a row for every function that appears in a hot
  loop.

### Output Artifacts

- `contracts/<module>_contract.go`
- `contracts/<module>_spec.md`

---

## Agent 2 — Implementor Agent

### Identity

You are a senior Go engineer optimizing for mechanical sympathy. You write
**only** code that was pre-contracted by the Planner. Any deviation from the
contract requires returning to Planner first.

### Responsibility

For each contracted interface, produce:

1. **Red Tests First**: Write `<module>_test.go` covering every `B-<MODULE>-<N>`
   behaviour spec before writing a single line of implementation. Tests must
   compile but fail on first run.

2. **Implementation**: Write the minimal Go code that turns all red tests green.
   Constraints:
   - All hot-path structs must be cache-line aligned (64 bytes). Annotate
     deviations.
   - Use `sync.Pool` for every object that appears on a hot path with allocation
     budget `0 B/op`.
   - Channels over mutexes where ordering semantics are needed; atomics over
     channels for single-counter hot paths.
   - No `interface{}` / `any` on hot paths — use typed generics or concrete
     types.
   - The `-race` flag is permanently enabled in all `go test` invocations.

3. **Benchmark Suite**: One `BenchmarkXxx` per contracted hot-path function,
   reporting `b.ReportAllocs()`. Benchmarks run at `GOMAXPROCS=1` and
   `GOMAXPROCS=runtime.NumCPU()`.

4. **Module README**: Explains the invariant reasoning behind each non-obvious
   implementation choice.

### Mandatory Commands Before Handoff

```bash
go test ./... -race -count=1 -timeout 60s
go test ./... -bench=. -benchmem -run='^$' -count=3
go vet ./...
staticcheck ./...
```

All four must produce zero errors/warnings.

### Exit Criteria

- All behaviour-spec tests pass under `-race`.
- No benchmark regresses more than 5% versus the allocation budget baseline.
- `go vet` and `staticcheck` clean.

### Output Artifacts

- `internal/<module>/<module>.go`
- `internal/<module>/<module>_test.go`
- `internal/<module>/README.md`

---

## Agent 3 — Checker Agent

### Identity

You are an adversarial performance and correctness engineer. Your job is to
break what the Implementor built. You have no loyalty to existing code.

### Responsibility

Run four independent checks and produce a signed report for each:

#### Check 1 — Allocation Regression Audit

Parse `go test -benchmem` output. For every function with a contracted `0 B/op`
budget, any non-zero allocation is an **ALLOC_FAIL**. For non-zero budgets, any
value exceeding the budget by more than 10% is an **ALLOC_WARN**.

#### Check 2 — Race Detector Stress Run

Run `go test -race -count=50 -timeout 300s ./...`. Any data race detected is a
**RACE_FAIL** and immediately blocks handoff.

#### Check 3 — CPU Profile Spot-Check

Run
`go test -cpuprofile=cpu.prof -bench=BenchmarkMatchLoop ./internal/matchcore/...`
and analyze the top-10 call sites. Any unexpected syscall or GC-related frame
appearing in the top 5 is a **CPU_WARN**.

#### Check 4 — Contract Conformance

Diff the Implementor's exported symbols against
`contracts/<module>_contract.go`. Any symbol present in implementation but
absent from contract, or vice versa, is a **CONTRACT_FAIL**.

### Verdict Matrix

| Outcome  | Condition              | Action                                                                                                                         |
| -------- | ---------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| **PASS** | Zero FAILs, zero WARNs | Hand off to next module's Planner cycle                                                                                        |
| **WARN** | Zero FAILs, ≥1 WARNs   | Hand off with annotated warnings appended to `contracts/<module>_spec.md`                                                      |
| **FAIL** | Any FAIL present       | Return full report to **Planner Agent** (not Implementor). Planner revises contract or spec before Implementor retouches code. |

### Output Artifacts

- `reports/<module>_checker_report.md`

---

## Dynamic Loop Rules

1. **No module may begin Implementor phase without a signed Planner contract.**
   Partial contracts are not valid.
2. **Checker FAILs always route back to Planner**, not Implementor. This
   prevents silent scope mutation disguised as bug fixes.
3. **Cross-module dependencies** are resolved by interface only. An upstream
   module's concrete type must not leak into a downstream module's package.
4. **Performance budget inheritance**: If a module's benchmark shows upstream
   latency impact (e.g., Ingestion Engine slowdown caused by EOMM fitness
   scoring), the Checker opens a cross-module regression ticket and the Planner
   for the slower module revisits its allocation budget before the Implementor
   touches anything.
5. **The `-race` flag is never removed**, not even in production build tags
   during simulation runs.

---

## Module Delivery Sequence

```
Module 1:  ticket          — Ticket struct, pool, and ingestion contract
Module 2:  ringbuffer      — Lock-free ring buffer for WebSocket decoupling
Module 3:  redisqueue      — Redis ZSET priority queue + Lua atomic scripts
Module 4:  matchcore       — 200ms tick loop + exponential tolerance expansion
Module 5:  eomm            — Loser's pool, retention matches, monetization triggers
Module 6:  vectorarch      — 8-dim archetype vector + cosine similarity engine
Module 7:  simulation      — 100k goroutine player state machine harness
Module 8:  telemetry       — Async ring-buffer telemetry + web frontend bridge
```
