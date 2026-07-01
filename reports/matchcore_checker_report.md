# Matchcore Checker Report

Module: `matchcore`  
Checker: Codex  
Date: 2026-07-01

CHECKER: WARN

## Summary

No FAIL conditions were found. The module passes allocation, race, vet,
staticcheck, contract conformance, import-boundary, and default test isolation
checks.

Warnings:

- `CPU_WARN`: The matchcore CPU profile benchmark completed, but this Go
  toolchain does not include `go tool pprof`. The fallback `go tool preprofile`
  output contains sampled runtime/syscall/GC-related frames including
  `runtime.netpoll`, `runtime.kevent`, `runtime.mallocgc`, and
  `runtime.(*mheap).allocSpan`.

## Check 1: Allocation Regression Audit

Command:

```bash
GOCACHE=/private/tmp/matchpoint-gocache go test ./... -bench=. -benchmem -run='^$' -count=3
```

Result: PASS.

Matchcore allocation rows from the fresh run stayed within contract:

| Benchmark family | Contract budget | Fresh result |
| --- | ---: | ---: |
| `NowUnixNano` | `0 B/op` | `0 B/op` |
| `ComputeTolerance` | `0 B/op` | `0 B/op` |
| `DrainRings` | `0 B/op` | `0 B/op` |
| `EnqueueDrained` helper | `0 B/op` | `0 B/op` |
| `BuildCandidateQueries` | `0 B/op` | `0 B/op` |
| `ScoreCandidate` | `0 B/op` | `0 B/op` |
| `CandidateTieBreaking` | `0 B/op` | `0 B/op` |
| `AssignPair` helper | `0 B/op` | `0 B/op` |
| `MetricsSink` | `0 B/op` | `0 B/op` |
| `NextMatchID` | `0 B/op` | `0 B/op` |
| `SnapshotTickState` | `0 B/op` | `0 B/op` |
| `MatchLoop` | `< 512 B/op` | `24 B/op` |

Inherited `ticket`, `ringbuffer`, and `redisqueue` benchmark rows remained
inside their signed budgets. No `ALLOC_FAIL` or `ALLOC_WARN`.

## Check 2: Race Detector Run

Commands:

```bash
GOCACHE=/private/tmp/matchpoint-gocache go test ./... -race -count=1 -timeout 60s
GOCACHE=/private/tmp/matchpoint-gocache go test -race ./internal/matchcore
```

Result: PASS.

Output included:

```text
?   	matchpoint/contracts	[no test files]
ok  	matchpoint/internal/matchcore	1.162s
ok  	matchpoint/internal/redisqueue	1.160s
ok  	matchpoint/internal/ringbuffer	1.457s
ok  	matchpoint/internal/ticket	1.311s
```

No `RACE_FAIL`.

## Check 3: CPU Profile Spot-Check

Command:

```bash
GOCACHE=/private/tmp/matchpoint-gocache go test -cpuprofile=/private/tmp/matchcore_cpu.prof -bench=BenchmarkMatchLoop -benchmem -run='^$' ./internal/matchcore
```

Result:

```text
BenchmarkMatchLoop-10                 	 5488300	       205.8 ns/op	      24 B/op	       1 allocs/op
BenchmarkMatchLoopGOMAXPROCS1-10      	 5702942	       211.4 ns/op	      24 B/op	       1 allocs/op
BenchmarkMatchLoopGOMAXPROCSCPU-10    	 5767135	       207.7 ns/op	      24 B/op	       1 allocs/op
PASS
ok  	matchpoint/internal/matchcore	4.589s
```

`go tool pprof` is not available in this Go toolchain:

```text
go: no such tool "pprof"
```

Fallback command:

```bash
go tool preprofile -i /private/tmp/matchcore_cpu.prof -o /private/tmp/matchcore_cpu.preprofile
```

The converted preprofile contained sampled runtime/syscall/GC-related frames:

```text
runtime.netpoll
runtime.kevent
runtime.mallocgcSmallNoscan
runtime.(*mheap).allocSpan
runtime.sysUsed
```

Verdict: `CPU_WARN`. No `CPU_FAIL` category is defined by the Checker contract.

## Check 4: Contract Conformance

Result: PASS.

- Exported implementation aliases/constants align with the Planner contract
  surface in `contracts/matchcore_contract.go`.
- `internal/matchcore` imports `matchpoint/contracts` and standard-library
  packages only.
- No concrete `internal/ticket`, `internal/ringbuffer`, `internal/redisqueue`,
  future `internal/eomm`, or future `internal/vectorarch` imports are present.
- Redis enqueue timeout/unavailable and Redis assignment statuses are mapped
  through typed matchcore statuses and recorded through `MatchMetricsSink`.

## Behaviour Coverage

Result: PASS.

- `B-MATCHCORE-1` through `B-MATCHCORE-38` all appear in
  `internal/matchcore/matchcore_test.go`.
- Tests cover tick overrun/skip/warn behavior, config validation, deterministic
  ring drains, tolerance expansion and clamps, Redis enqueue and candidate query
  status mapping, candidate scoring and tie-breaking, assignment success/dual
  booking/error mapping, per-tick metrics, Redis status recording, and state
  snapshots.
- Default tests use fakes only and do not require Redis, Docker, network, real
  200ms sleeps, or real ticker timing.

## Static Analysis

Commands:

```bash
GOCACHE=/private/tmp/matchpoint-gocache go vet ./...
GOCACHE=/private/tmp/matchpoint-gocache STATICCHECK_CACHE=/private/tmp/matchpoint-staticcheck-cache staticcheck ./...
```

Result: PASS. Both commands exited `0` with no output after removing the dead
matchcore scratch field and using the clock constructor.

## Final Verdict

CHECKER: WARN
