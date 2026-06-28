# Ticket Checker Report

CHECKER: WARN

Module: `ticket`  
Date: 2026-06-27  
Checker: Project MatchPoint Checker Agent

## Verdict Summary

FAILs: 0  
WARNs: 2

- `STATICCHECK_WARN`: `staticcheck ./...` exits `0` but prints `warning: "./..." matched no packages`; explicit package paths also match no packages, while `go list ./...` sees `matchpoint/contracts` and `matchpoint/internal/ticket`.
- `COVERAGE_WARN`: behaviour coverage exists for `B-TICKET-1` through `B-TICKET-26`, but some behaviours are only partially/directly covered: malformed JSON decode, fewer/more-than-8-card decode paths, exact primary/secondary weight assertions, and the 10us backpressure-wait responsibility delegated to the sink boundary.

No `ALLOC_FAIL`, `ALLOC_WARN`, `RACE_FAIL`, `CPU_WARN`, or `CONTRACT_FAIL` findings were observed.

## Check 1 - Allocation Regression Audit

Command:

```bash
go test ./... -bench=. -benchmem -run='^$' -count=3
```

Result: PASS

- Command exited `0`.
- All benchmark rows reported `0 B/op` and `0 allocs/op`.
- Contracted `0 B/op` paths complied, including:
  - `AcquireTicket`
  - `ResetTicket`
  - `ReleaseTicket`
  - `DecodeQueueJoin` MessagePack
  - `ValidateSession`
  - `LoadSignals`
  - `BuildDeckVector`
  - `WriteTicket`
  - `ShardDepth`
  - `EstimateWaitMS`
  - `NowUnixNano`
  - `ParseTicket` MessagePack
- JSON fallback paths also reported `0 B/op`, below the allowed `256 B/op` budget.

Representative rows:

```text
BenchmarkDecodeQueueJoinMessagePackGOMAXPROCS1-10     0 B/op  0 allocs/op
BenchmarkDecodeQueueJoinJSONGOMAXPROCS1-10            0 B/op  0 allocs/op
BenchmarkBuildDeckVectorGOMAXPROCS1-10                0 B/op  0 allocs/op
BenchmarkParseTicketMessagePackGOMAXPROCS1-10         0 B/op  0 allocs/op
BenchmarkParseTicketJSONGOMAXPROCS1-10                0 B/op  0 allocs/op
```

## Check 2 - Race Detector Stress Run

Command:

```bash
go test -race -count=50 -timeout 300s ./...
```

Result: PASS

Output:

```text
?    matchpoint/contracts      [no test files]
ok   matchpoint/internal/ticket 1.301s
```

No race detector reports were emitted.

## Check 3 - CPU Profile Spot-Check

Canonical `BenchmarkMatchLoop` under `internal/matchcore` is not applicable to Module 1. The check was adapted to the ticket hot path.

Command:

```bash
go test -cpuprofile=/private/tmp/matchpoint-ticket-cpu.prof -bench=BenchmarkParseTicketMessagePackGOMAXPROCS1 -benchmem -run='^$' ./internal/ticket
```

Result: PASS

Benchmark output:

```text
BenchmarkParseTicketMessagePackGOMAXPROCS1-10  14425443  84.58 ns/op  0 B/op  0 allocs/op
PASS
ok   matchpoint/internal/ticket 2.590s
```

`go tool pprof` is unavailable in this Go toolchain:

```text
go: no such tool "pprof"
```

Fallback inspection used `go tool preprofile`, which was available. The leading profile entries were ticket decode/vector functions, with no syscall or GC-related frame in the top observed entries:

```text
matchpoint/internal/ticket.(*intakeProcessor).ParseTicket -> buildDeckVector
matchpoint/internal/ticket.(*intakeProcessor).ParseTicket -> decodeMessagePack
bytes.Equal -> runtime.memequal
matchpoint/internal/ticket.decodeMessagePack -> readMPString
matchpoint/internal/ticket.decodeMessagePack -> readMPCards
matchpoint/internal/ticket.benchmarkParseTicket -> ParseTicket
matchpoint/internal/ticket.readMPCards -> readMPInt
matchpoint/internal/ticket.decodeMessagePack -> bytes.Equal
matchpoint/internal/ticket.buildDeckVector -> popcount16
matchpoint/internal/ticket.(*authDouble).ValidateSession -> bytes.Equal
```

No `CPU_WARN` issued.

## Check 4 - Contract Conformance

Result: PASS

Compared exported package surfaces with:

```bash
go doc matchpoint/internal/ticket
go doc matchpoint/contracts
```

The implementation exports the same public contract symbols:

```text
Authenticator
CardDef
Clock
DeckValidator
DerivedSignals
IntakeErrorCode
IntakeProcessor
PayloadDecoder
QueueEstimator
QueueJoinAck
QueueJoinPayload
QueueJoinStatus
RingBufferSink
SignalStore
Ticket
TicketPool
TicketPoolTag
WireFormat
```

No extra exported implementation symbol and no missing contracted exported symbol were found.

## Behaviour Coverage Audit

Result: WARN

Observed direct coverage:

- `B-TICKET-1`: `TestTicketLayoutIsOneCacheLine`
- `B-TICKET-2` to `B-TICKET-5`: `TestTicketPoolLifecycle`
- `B-TICKET-6`: `TestDecodeRejectsMalformedPayload`
- `B-TICKET-7`: `TestParseTimeoutReleasesNoTicketAndPublishesNothing`
- `B-TICKET-8`: `TestParseRejectsZeroPlayerID`
- `B-TICKET-9`: `TestParseRejectsMissingOrInvalidSession`
- `B-TICKET-10`: `TestParseRejectsTrophiesOutsideMatchSpecBounds`
- `B-TICKET-11`: `TestParseRejectsTierMismatch`
- `B-TICKET-12`: `TestDeckValidatorRejectsInvalidDecks`
- `B-TICKET-13`: `TestDeckValidatorNormalizesValidDeck`
- `B-TICKET-14`: `TestDeckValidatorRejectsZeroVector`
- `B-TICKET-15`: `TestParseUsesFallbackSignalsWhenStoreUnavailable`
- `B-TICKET-16`: `TestParseOverridesClientSignalsWithServerSignals`
- `B-TICKET-17` and `B-TICKET-18`: `TestParseRejectsInvalidDerivedSignals`
- `B-TICKET-19` to `B-TICKET-21`: `TestParsePopulatesTicketAndPublishesOnce`
- `B-TICKET-22`: `TestParseHandlesRingBufferFullAndReleasesTicket`
- `B-TICKET-23`: `TestParseRejectsDuplicateActiveTicket`
- `B-TICKET-24` and `B-TICKET-25`: `TestDecodersAcceptMessagePackAndJSONAndIgnoreClientRisk`
- `B-TICKET-26`: `TestSuccessfulParseTransfersTicketOwnership`

Coverage caveats:

- `B-TICKET-6` directly tests malformed MessagePack, but not malformed JSON.
- `B-TICKET-12` tests duplicate, out-of-range, heavy, and single-archetype decks, but does not explicitly test fewer/more-than-8-card JSON or MessagePack payloads.
- `B-TICKET-13` verifies normalization and non-zero dimensional contribution, but does not assert exact primary `1.0` and secondary `0.4` raw contribution math.
- `B-TICKET-22` verifies ring-buffer-full handling after sink rejection, but the single 10us backpressure wait is only represented as a sink-boundary responsibility, not directly exercised by this module.

## Staticcheck Audit

Result: WARN

Commands:

```bash
staticcheck ./...
staticcheck ./internal/ticket ./contracts
go list ./...
staticcheck -debug.version
```

Observed:

```text
$ staticcheck ./...
warning: "./..." matched no packages

$ staticcheck ./internal/ticket ./contracts
warning: "./internal/ticket" matched no packages
warning: "./contracts" matched no packages

$ go list ./...
matchpoint/contracts
matchpoint/internal/ticket
```

Tool versions:

```text
go version go1.25.4 darwin/arm64
staticcheck 2025.1.1 (0.6.1), compiled with go1.24.5
```

Interpretation: this is a tooling/package-resolution warning, not evidence that static analysis passed. The implementor handoff command exited `0`, but it did not actually analyze the Go packages in this workspace.

## Signature

Signed-off Checker Agent: Project MatchPoint Checker  
Verdict: `CHECKER: WARN`
