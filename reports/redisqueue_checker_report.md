# Redisqueue Checker Report

Module: `redisqueue`  
Checker: Codex  
Date: 2026-06-28

CHECKER: WARN

## Summary

No FAIL conditions were found. The module passes allocation, race, vet,
staticcheck, contract conformance, import-boundary, and default test isolation
checks.

Warnings:

- `CPU_WARN`: The redisqueue CPU profile benchmark completed, but this Go
  toolchain does not include `go tool pprof`. The fallback `go tool preprofile`
  output contains syscall/GC-related sampled frames including `runtime.netpoll`,
  `runtime.kevent`, and `runtime.mallocgc`; these are treated as CPU profile
  warnings.
- `BENCH_WARN`: The fresh benchmark run emitted Go test warnings that several
  `GOMAXPROCS1` benchmarks left `GOMAXPROCS` set to `1`. Allocation values are
  still valid, but benchmark hygiene is not warning-free.
- `COVERAGE_WARN`: Behaviour markers `B-REDISQUEUE-1` through
  `B-REDISQUEUE-40` are all present, but some multi-clause behaviours are
  covered through representative fake-store paths rather than every Redis-bound
  method variant.

## Check 1: Allocation Regression Audit

Command:

```bash
GOCACHE=/private/tmp/matchpoint-gocache go test ./... -bench=. -benchmem -run='^$' -count=3
```

Result: PASS with warnings noted above.

Redisqueue allocation rows from the fresh run stayed within contract:

| Benchmark family | Contract budget | Fresh result |
| --- | ---: | ---: |
| `SegmentForTrophies` | `0 B/op` | `0 B/op` |
| `KeyForPool` | `0 B/op` | `0 B/op` |
| `SegmentRange` | `0 B/op` | `0 B/op` |
| `EncodeMember` | `0 B/op` | `0 B/op` |
| `EncodeScore` | `0 B/op` | `0 B/op` |
| `ScoreRange` | `0 B/op` | `0 B/op` |
| `ScriptSHA` | `0 B/op` | `0 B/op` |
| `MarkNoScript` | `0 B/op` | `0 B/op` |
| `Metrics` | `0 B/op` | `0 B/op` |
| `EnqueueFake` | `<= 256 B/op` | `3 B/op` |
| `RemoveFake` | `<= 128 B/op` | `3 B/op` |
| `FetchCandidatesFake` | `<= 256 B/op` | `0 B/op` |
| `FetchCandidateBatchFake` | `< 512 B/op` | `0 B/op` |
| `MovePoolFake` | `<= 256 B/op` | `3 B/op` |
| `AssignMatchFake` | `<= 256 B/op` | `71 B/op` |

Inherited `ticket` and `ringbuffer` benchmark rows observed in `./...` reported
`0 B/op` on their contracted zero-allocation paths.

No `ALLOC_FAIL` or `ALLOC_WARN`.

## Check 2: Race Detector Stress Run

Command:

```bash
GOCACHE=/private/tmp/matchpoint-gocache go test -race -count=50 -timeout 300s ./...
```

Result:

```text
?   	matchpoint/contracts	[no test files]
ok  	matchpoint/internal/redisqueue	2.048s
ok  	matchpoint/internal/ringbuffer	1.415s
ok  	matchpoint/internal/ticket	1.683s
```

No `RACE_FAIL`.

## Check 3: CPU Profile Spot-Check

Command:

```bash
GOCACHE=/private/tmp/matchpoint-gocache go test -cpuprofile=/private/tmp/redisqueue_cpu.prof -bench=BenchmarkAssignMatchFakeGOMAXPROCS1 -benchmem -run='^$' ./internal/redisqueue
```

Result:

```text
BenchmarkAssignMatchFakeGOMAXPROCS1-10    	 3109412	       386.4 ns/op	      71 B/op	       5 allocs/op
PASS
ok  	matchpoint/internal/redisqueue	2.062s
```

`go tool pprof` is not available in this Go toolchain:

```text
go: no such tool "pprof"
```

Fallback command:

```bash
go tool preprofile -i /private/tmp/redisqueue_cpu.prof -o /private/tmp/redisqueue_cpu.preprofile
```

The converted preprofile contained sampled syscall/GC-related frames:

```text
runtime.netpoll
runtime.kevent
runtime.mallocgc
runtime.(*mheap).allocSpan
runtime.sysUsed
```

Verdict: `CPU_WARN`. No `CPU_FAIL` category is defined by the Checker contract.

## Check 4: Contract Conformance

Result: PASS.

- Exported implementation symbols plus exported concrete method names match the
  contract/interface method surface exactly: `77` contract names and `77`
  implementation names, with no missing or extra symbols.
- `internal/redisqueue` imports `matchpoint/contracts` and
  `github.com/redis/go-redis/v9`; it does not import `internal/ticket` or
  `internal/ringbuffer`.
- `contracts` does not import or expose `go-redis/v9`.
- `go-redis/v9` remains an implementation detail in `internal/redisqueue`.

## Behaviour Coverage

Result: PASS with coverage warning.

- `B-REDISQUEUE-1` through `B-REDISQUEUE-40` all appear in
  `internal/redisqueue/redisqueue_test.go`.
- Tests cover same-player assignment rejection, `NOSCRIPT`, timeout,
  cancellation, Redis unavailable mapping, dual booking, score precision,
  score-range scaling by `1e6`, and copy-on-enqueue ownership semantics.
- Default tests do not require Docker or a live Redis server. Live Redis is
  gated by `MP_REDIS_INTEGRATION=1` and skipped by default.

Coverage warning:

- Some behaviours with wording such as "any store operation" are tested through
  representative fake-store paths rather than exhaustively across every store
  method and real Redis adapter path.

## Static Analysis

Commands:

```bash
GOCACHE=/private/tmp/matchpoint-gocache go vet ./...
GOCACHE=/private/tmp/matchpoint-gocache STATICCHECK_CACHE=/private/tmp/matchpoint-staticcheck-cache staticcheck ./...
```

Result: PASS. Both commands exited `0` with no output.

## Final Verdict

CHECKER: WARN
