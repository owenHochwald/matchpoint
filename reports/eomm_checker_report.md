# EOMM Checker Report

Module: `eomm`  
Checker: Codex  
Date: 2026-07-01

CHECKER: WARN

## Summary

No FAIL conditions were found. The module passes allocation, race, vet,
staticcheck, contract conformance, import-boundary, and default test isolation
checks.

Warning:

- `CPU_WARN`: The EOMM CPU profile benchmark completed, but this Go toolchain
  does not include `go tool pprof`. The fallback `go tool preprofile` output
  was inspectable and showed EOMM scoring frames plus runtime scheduler samples.

## Check 1: Allocation Regression Audit

Command:

```bash
GOCACHE=/private/tmp/matchpoint-gocache go test ./... -bench=. -benchmem -run='^$' -count=1
```

Result: PASS.

EOMM allocation rows from the fresh run stayed within contract:

| Benchmark family | Contract budget | Fresh result |
| --- | ---: | ---: |
| `RouteTicket` | `0 B/op` | `0 B/op` |
| `RouteMatchOutcome` | `0 B/op` | `0 B/op` |
| `ApplyRoute` helper | `0 B/op` | `0 B/op` |
| `ScoreBreakdown` | `0 B/op` | `0 B/op` |
| `ScoreCandidate` | `0 B/op` | `0 B/op` |
| `RecordOutcome` | `0 B/op` | `0 B/op` |

Inherited `ticket`, `ringbuffer`, `redisqueue`, and `matchcore` benchmark rows
remained inside their signed budgets. No `ALLOC_FAIL` or `ALLOC_WARN`.

## Check 2: Race Detector Stress Run

Command:

```bash
GOCACHE=/private/tmp/matchpoint-gocache go test -race -count=50 -timeout 300s ./internal/eomm
```

Result:

```text
ok  	matchpoint/internal/eomm	1.313s
```

No `RACE_FAIL`.

## Check 3: CPU Profile Spot-Check

Command:

```bash
GOCACHE=/private/tmp/matchpoint-gocache go test -cpuprofile=/private/tmp/eomm_cpu.prof -bench=BenchmarkScoreCandidateGOMAXPROCS1 -benchmem -run='^$' ./internal/eomm
```

Result:

```text
BenchmarkScoreCandidateGOMAXPROCS1-10    	49614111	        24.57 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	matchpoint/internal/eomm	1.663s
```

`go tool pprof` is not available in this Go toolchain:

```text
go: no such tool "pprof"
```

Fallback command:

```bash
go tool preprofile -i /private/tmp/eomm_cpu.prof -o /private/tmp/eomm_cpu.preprofile
```

The converted preprofile included EOMM hot-path frames:

```text
matchpoint/internal/eomm.(*engine).ScoreCandidate
matchpoint/internal/eomm.(*engine).ScoreBreakdown
matchpoint/internal/eomm.cosine
matchpoint/internal/eomm.validCandidate
```

It also included runtime scheduler samples such as `runtime.goschedImpl`,
`runtime.findRunnable`, and `runtime.schedule`. Verdict: `CPU_WARN` because
the canonical pprof top view is unavailable.

## Check 4: Contract Conformance

Result: PASS.

- Exported implementation aliases/constants align with
  `contracts/eomm_contract.go`.
- `internal/eomm` imports only standard-library packages and
  `matchpoint/contracts`.
- No concrete `internal/matchcore`, `internal/redisqueue`, future
  `internal/vectorarch`, simulation, or telemetry imports are present.

## Behaviour Coverage

Result: PASS.

- `B-EOMM-1` through `B-EOMM-28` all appear in
  `internal/eomm/eomm_test.go`.
- Tests cover routing priority, no-op routing, starvation evacuation,
  post-match evacuation, Redis movement status mapping, score weights,
  retention/monetization targets, structural counters, invalid candidate
  rejection, rolling spike detection, and malformed spike input.
- Default tests use fakes only and do not require Redis, Docker, network, or
  real timers.

## Static Analysis

Commands:

```bash
GOCACHE=/private/tmp/matchpoint-gocache go vet ./...
GOCACHE=/private/tmp/matchpoint-gocache STATICCHECK_CACHE=/private/tmp/matchpoint-staticcheck-cache staticcheck ./...
```

Result: PASS. Both commands exited `0` with no output.

## Final Verdict

CHECKER: WARN
