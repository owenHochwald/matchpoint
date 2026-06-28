# Ringbuffer Checker Report

CHECKER: WARN

Module: `ringbuffer`  
Date: 2026-06-28  
Checker: Project MatchPoint Checker Agent

## Verdict Summary

FAILs: 0  
WARNs: 2

- `STATICCHECK_WARN`: `staticcheck ./...` exits `0` but prints `warning: "./..." matched no packages`; `go list ./...` sees the expected Go packages, so static analysis did not actually run.
- `COVERAGE_WARN`: tests contain markers for `B-RINGBUFFER-1` through `B-RINGBUFFER-30`, but some multi-clause behaviours are only partially asserted, especially post-read duplicate-key reuse, concurrent duplicate queue-order preservation, and fully external construction usability.

No `ALLOC_FAIL`, `ALLOC_WARN`, `RACE_FAIL`, `CPU_WARN`, or `CONTRACT_FAIL` findings were observed.

One-line summary: Ringbuffer allocation, race, CPU profile, and contract audits pass; warnings remain for staticcheck package matching and partial multi-clause behaviour coverage.

## Check 1 - Allocation Regression Audit

Command:

```bash
GOCACHE=/private/tmp/matchpoint-gocache go test ./... -bench=. -benchmem -run='^$' -count=3
```

Result: PASS

- Command exited `0`.
- All fresh `ringbuffer` benchmark rows reported `0 B/op` and `0 allocs/op`.
- Inherited `ticket` benchmarks also reported `0 B/op` and `0 allocs/op`.
- Contracted ringbuffer hot paths complied:
  - `ShardSelector.ShardForPlayer`
  - `TicketRingBuffer.WriteTicket` accepted path
  - `TicketRingBuffer.WriteTicket` full/timeout path
  - `TicketRingBuffer.ReadTicket`
  - `TicketRingBuffer.DrainShard`
  - `TicketRingBuffer.SnapshotShard`
  - `TicketRingBuffer.CloseShard`
  - `TicketRingBuffer.Close`

Representative fresh rows:

```text
BenchmarkShardForPlayerGOMAXPROCS1-10            0 B/op  0 allocs/op
BenchmarkWriteTicketAcceptedGOMAXPROCS1-10       0 B/op  0 allocs/op
BenchmarkWriteTicketTimeoutGOMAXPROCS1-10        0 B/op  0 allocs/op
BenchmarkReadTicketGOMAXPROCS1-10                0 B/op  0 allocs/op
BenchmarkDrainShardGOMAXPROCS1-10                0 B/op  0 allocs/op
BenchmarkSnapshotShardGOMAXPROCS1-10             0 B/op  0 allocs/op
BenchmarkCloseShardGOMAXPROCS1-10                0 B/op  0 allocs/op
BenchmarkCloseGOMAXPROCS1-10                     0 B/op  0 allocs/op
```

## Check 2 - Race Detector Stress Run

Command:

```bash
GOCACHE=/private/tmp/matchpoint-gocache go test -race -count=50 -timeout 300s ./...
```

Result: PASS

Output:

```text
?    matchpoint/contracts             [no test files]
ok   matchpoint/internal/ringbuffer   1.325s
ok   matchpoint/internal/ticket       1.654s
```

No race detector reports were emitted.

## Check 3 - CPU Profile Spot-Check

Canonical `BenchmarkMatchLoop` under `internal/matchcore` is not applicable to Module 2 because `matchcore` has not been delivered. The check was adapted to a ringbuffer hot-path write/read benchmark.

Command:

```bash
GOCACHE=/private/tmp/matchpoint-gocache go test -cpuprofile=/private/tmp/matchpoint-ringbuffer-cpu.prof -bench=BenchmarkWriteTicketAcceptedGOMAXPROCS1 -benchmem -run='^$' ./internal/ringbuffer
```

Result: PASS

Benchmark output:

```text
BenchmarkWriteTicketAcceptedGOMAXPROCS1-10  950774  1291 ns/op  0 B/op  0 allocs/op
PASS
ok   matchpoint/internal/ringbuffer  2.144s
```

`go tool pprof` is unavailable in this Go toolchain:

```text
go: no such tool "pprof"
```

Fallback inspection used `go tool preprofile`. Observed leading entries:

```text
matchpoint/internal/ringbuffer.(*ringBuffer).WriteTicket -> matchpoint/internal/ringbuffer.(*ringShard).claimDuplicate
matchpoint/internal/ringbuffer.(*ringShard).claimDuplicate -> sync/atomic.(*Uint64).Load
matchpoint/internal/ringbuffer.(*ringShard).claimDuplicate -> runtime.asyncPreempt
matchpoint/internal/ringbuffer.benchmarkWriteAccepted -> matchpoint/internal/ringbuffer.(*ringBuffer).WriteTicket
runtime.(*timer).unlockAndRun -> runtime.goroutineReady
runtime.findRunnable -> runtime.(*timers).check
runtime.gopreempt_m -> runtime.goschedImpl
runtime.newstack -> runtime.gopreempt_m
```

No syscall or GC-related frame appeared in the observed top entries. No `CPU_WARN` issued.

## Check 4 - Contract Conformance

Result: PASS

Commands and inspections:

```bash
go doc -all matchpoint/internal/ringbuffer
go doc matchpoint/contracts
rg -n "internal/ticket|any|interface\\{\\}" internal/ringbuffer contracts/ringbuffer_spec.md
```

Exported implementation symbols match the ringbuffer contract-facing surface:

```text
RingBackpressureWaitNanos
RingStateOpen
RingStateDraining
RingStateClosed
RingWriteAccepted
RingWriteFull
RingWriteBackpressureTimeout
RingWriteClosed
RingWriteDuplicatePublisher
RingReadOK
RingReadEmpty
RingReadClosed
RingConfig
RingCursor
RingDrainResult
RingReadResult
RingReadStatus
RingSequence
RingShard
RingShardID
RingSlot
RingSnapshot
RingState
RingWriteResult
RingWriteStatus
ShardSelector
TicketRingBuffer
```

No exported implementation symbol absent from `contracts/ringbuffer_contract.go` was found. No contracted ringbuffer exported symbol was missing from `internal/ringbuffer`.

Import boundary: PASS

- `internal/ringbuffer` imports `matchpoint/contracts`.
- No `internal/ticket` import was found.
- No `interface{}` or `any` hot-path usage was found in `internal/ringbuffer`.

## Behaviour Coverage Audit

Result: WARN

All 30 behaviour IDs appear in `internal/ringbuffer/ringbuffer_test.go`.

Observed direct coverage:

- `B-RINGBUFFER-1` to `B-RINGBUFFER-3`: `TestConstructionValidationAndShardInitialization`
- `B-RINGBUFFER-4`: `TestShardForPlayerIsStableModulo`
- `B-RINGBUFFER-5`: `TestBackpressureBudgetConstant`
- `B-RINGBUFFER-6`, `B-RINGBUFFER-7`, `B-RINGBUFFER-13`, `B-RINGBUFFER-14`, `B-RINGBUFFER-27`: `TestWriteReadOrderingAndPublication`
- `B-RINGBUFFER-8`, `B-RINGBUFFER-10`, `B-RINGBUFFER-30`: `TestFullShardTimeoutAndNoOverwrite`
- `B-RINGBUFFER-9`, `B-RINGBUFFER-30`: `TestFullShardAcceptsSlotFreedDuringBoundedWait`
- `B-RINGBUFFER-11`, `B-RINGBUFFER-12`: `TestDuplicatePublisherRejectionAndConcurrentRace`
- `B-RINGBUFFER-15`, `B-RINGBUFFER-18`, `B-RINGBUFFER-28`: `TestReadEmptyInvalidShardAndNonOKZeroValues`
- `B-RINGBUFFER-16`, `B-RINGBUFFER-17`, `B-RINGBUFFER-29`: `TestDrainShardBoundedByDestination`
- `B-RINGBUFFER-19` to `B-RINGBUFFER-22`: `TestCloseShardAndCloseLifecycle`
- `B-RINGBUFFER-23`: `TestSnapshotConcurrentAndDepthClamped`
- `B-RINGBUFFER-24` to `B-RINGBUFFER-26`: `TestCacheLineAndSequenceWrapInvariants`

Coverage caveats:

- `B-RINGBUFFER-14` states that read clears the duplicate-publisher key. The tests read a ticket, but do not explicitly assert the same player can be written again after read.
- `B-RINGBUFFER-12` checks that concurrent duplicate publishers yield exactly one accepted write, but does not separately assert that losers preserve queue order under a mixed distinct-player workload.
- `B-RINGBUFFER-1` through `B-RINGBUFFER-3` exercise construction through package-private `newRingBuffer`. This matches the current contract surface, but no exported constructor is contracted or implemented for downstream modules to instantiate a `TicketRingBuffer`.

## Staticcheck Audit

Result: WARN

Commands:

```bash
staticcheck ./...
go list ./...
```

Observed:

```text
$ staticcheck ./...
warning: "./..." matched no packages

$ go list ./...
matchpoint/contracts
matchpoint/internal/ringbuffer
matchpoint/internal/ticket
```

Interpretation: this is a tooling/package-resolution warning, not evidence that static analysis passed. The command exits `0`, but it does not analyze the packages that `go list` can resolve.

## Signature

Signed-off Checker Agent: Project MatchPoint Checker  
Verdict: `CHECKER: WARN`
