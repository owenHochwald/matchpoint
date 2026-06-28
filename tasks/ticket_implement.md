# Task: IMPLEMENT — ticket

## Status
Planner completed the `ticket` contract and spec. Orchestrator validation passed:
- `contracts/ticket_contract.go` exists.
- `contracts/ticket_contract.go` contains no `func` bodies.
- `contracts/ticket_spec.md` exists.
- `contracts/ticket_spec.md` contains 26 `B-TICKET-N:` behaviours.
- `contracts/ticket_spec.md` contains an Allocation Budget Table.
- `contracts/ticket_spec.md` contains an Edge Case Register.

Repository-level note: plain `go test ./contracts` currently fails before package compilation because the repository has no `go.mod`. You may create the minimal Go module scaffolding required for the mandatory commands to run, but do not broaden scope beyond that.

## Your Job
Implement Module 1: `ticket` exactly against the signed Planner artifacts:
- `contracts/ticket_contract.go`
- `contracts/ticket_spec.md`

Write implementation artifacts:
- `internal/ticket/ticket.go`
- `internal/ticket/ticket_test.go`
- `internal/ticket/README.md`

Also create minimal repository Go tooling files only if required for the mandatory commands, for example `go.mod`.

Implementation requirements:
- Write tests covering every `B-TICKET-N:` behaviour before implementation.
- Include at least one `BenchmarkXxx` for every hot-path function in the contract/spec.
- Preserve the contracted 64-byte `Ticket` layout or explicitly fail tests if impossible.
- Use `sync.Pool` for pooled tickets.
- Keep the MessagePack success path allocation target at `0 B/op` after pool warm-up.
- Do not use `interface{}` or `any` on hot paths.
- Do not implement modules outside `ticket`; use test doubles for ring-buffer, auth, signal store, queue estimator, and clock boundaries.
- Client intake trophy validation must follow `MATCH_SPEC.md`: `[0, 15000]`.
- Client-supplied churn risk and monetization probability must never be trusted.

Before handoff, paste the full outputs of these mandatory commands into this task file under a new `## Implementor Command Outputs` section:

```bash
go test ./... -race -count=1 -timeout 60s
go test ./... -bench=. -benchmem -run='^$' -count=3
go vet ./...
staticcheck ./...
```

All four must show zero errors/warnings. If `staticcheck` is not installed, install/use the project-standard invocation only if available; otherwise record the exact failure output.

## Relevant Spec Sections
- `docs/FEATURES.md` §2.1 `Ticket` — Primary Unit of Queue Entry
- `docs/FEATURES.md` §3 Module A — Ingestion Engine
- `docs/FEATURES.md` §6.1-§6.2 Deck vector normalization and cosine assumptions
- `docs/FEATURES.md` §10.1 Hot-Path Allocation Budget
- `docs/FEATURES.md` §10.2 `sync.Pool` Objects
- `docs/FEATURES.md` §10.3 Struct Alignment
- `docs/FEATURES.md` §11.2 Shared State Inventory
- `docs/FEATURES.md` §12.1 Error Taxonomy
- `docs/FEATURES.md` §13.3 Fuzz Targets for `FuzzParseTicket`
- `docs/MATCH_SPEC.md` §1.1 Trophy Ladder
- `docs/MATCH_SPEC.md` §2.3 Deck Validity Rules
- `docs/MATCH_SPEC.md` §2.4 Vector Construction
- `docs/MATCH_SPEC.md` §2.5 Card Table
- `docs/MATCH_SPEC.md` §3 Player Intake Interface
- `docs/MATCH_SPEC.md` §5 Key Invariants for the Orchestrator
- `docs/AGENTS.md` Implementor Agent responsibilities and exit criteria
- `docs/GIT_POLICY.md` Commit granularity and branch policy

## Inputs Available
- `tasks/ticket_plan.md`
- `contracts/ticket_contract.go`
- `contracts/ticket_spec.md`
- `docs/FEATURES.md`
- `docs/MATCH_SPEC.md`
- `docs/AGENTS.md`
- `docs/GIT_POLICY.md`

## Checker Report (if re-cycle)
Not applicable. This is the first Implementor dispatch for `ticket`.

## Implementor Command Outputs

### `go test ./... -race -count=1 -timeout 60s`

```text
?   	matchpoint/contracts	[no test files]
ok  	matchpoint/internal/ticket	1.249s
```

### `go test ./... -bench=. -benchmem -run='^$' -count=3`

```text
?   	matchpoint/contracts	[no test files]
goos: darwin
goarch: arm64
pkg: matchpoint/internal/ticket
cpu: Apple M4
BenchmarkAcquireTicketGOMAXPROCS1-10                 	185903028	         6.488 ns/op	       0 B/op	       0 allocs/op
BenchmarkAcquireTicketGOMAXPROCS1-10                 	173636640	         6.264 ns/op	       0 B/op	       0 allocs/op
BenchmarkAcquireTicketGOMAXPROCS1-10                 	186087466	         5.747 ns/op	       0 B/op	       0 allocs/op
BenchmarkAcquireTicketGOMAXPROCSN-10                 	183853851	         6.587 ns/op	       0 B/op	       0 allocs/op
BenchmarkAcquireTicketGOMAXPROCSN-10                 	185701009	         6.893 ns/op	       0 B/op	       0 allocs/op
BenchmarkAcquireTicketGOMAXPROCSN-10                 	184384159	         6.427 ns/op	       0 B/op	       0 allocs/op
BenchmarkResetTicketGOMAXPROCS1-10                   	1000000000	         0.2343 ns/op	       0 B/op	       0 allocs/op
BenchmarkResetTicketGOMAXPROCS1-10                   	1000000000	         0.2333 ns/op	       0 B/op	       0 allocs/op
BenchmarkResetTicketGOMAXPROCS1-10                   	1000000000	         0.2369 ns/op	       0 B/op	       0 allocs/op
BenchmarkResetTicketGOMAXPROCSN-10                   	1000000000	         0.2361 ns/op	       0 B/op	       0 allocs/op
BenchmarkResetTicketGOMAXPROCSN-10                   	1000000000	         0.2315 ns/op	       0 B/op	       0 allocs/op
BenchmarkResetTicketGOMAXPROCSN-10                   	1000000000	         0.2333 ns/op	       0 B/op	       0 allocs/op
BenchmarkReleaseTicketGOMAXPROCS1-10                 	183660525	         6.761 ns/op	       0 B/op	       0 allocs/op
BenchmarkReleaseTicketGOMAXPROCS1-10                 	191230856	         6.438 ns/op	       0 B/op	       0 allocs/op
BenchmarkReleaseTicketGOMAXPROCS1-10                 	182713002	         6.389 ns/op	       0 B/op	       0 allocs/op
BenchmarkReleaseTicketGOMAXPROCSN-10                 	182357390	         6.928 ns/op	       0 B/op	       0 allocs/op
BenchmarkReleaseTicketGOMAXPROCSN-10                 	178827594	         6.942 ns/op	       0 B/op	       0 allocs/op
BenchmarkReleaseTicketGOMAXPROCSN-10                 	178904767	         6.599 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinMessagePackGOMAXPROCS1-10    	22646832	        52.36 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinMessagePackGOMAXPROCS1-10    	22661676	        52.73 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinMessagePackGOMAXPROCS1-10    	22806894	        52.26 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinMessagePackGOMAXPROCSN-10    	23044608	        52.62 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinMessagePackGOMAXPROCSN-10    	22799384	        52.39 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinMessagePackGOMAXPROCSN-10    	20147410	        52.62 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinJSONGOMAXPROCS1-10           	5598969	       217.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinJSONGOMAXPROCS1-10           	5568414	       214.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinJSONGOMAXPROCS1-10           	5472223	       215.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinJSONGOMAXPROCSN-10           	5546847	       213.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinJSONGOMAXPROCSN-10           	5674358	       211.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinJSONGOMAXPROCSN-10           	5729290	       212.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidateSessionGOMAXPROCS1-10               	642870056	         1.865 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidateSessionGOMAXPROCS1-10               	643394548	         1.865 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidateSessionGOMAXPROCS1-10               	642977558	         1.865 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidateSessionGOMAXPROCSN-10               	642679975	         1.866 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidateSessionGOMAXPROCSN-10               	643504093	         1.867 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidateSessionGOMAXPROCSN-10               	642934782	         1.866 ns/op	       0 B/op	       0 allocs/op
BenchmarkLoadSignalsGOMAXPROCS1-10                   	1000000000	         0.2427 ns/op	       0 B/op	       0 allocs/op
BenchmarkLoadSignalsGOMAXPROCS1-10                   	1000000000	         0.2386 ns/op	       0 B/op	       0 allocs/op
BenchmarkLoadSignalsGOMAXPROCS1-10                   	1000000000	         0.2389 ns/op	       0 B/op	       0 allocs/op
BenchmarkLoadSignalsGOMAXPROCSN-10                   	1000000000	         0.2367 ns/op	       0 B/op	       0 allocs/op
BenchmarkLoadSignalsGOMAXPROCSN-10                   	1000000000	         0.2365 ns/op	       0 B/op	       0 allocs/op
BenchmarkLoadSignalsGOMAXPROCSN-10                   	1000000000	         0.2384 ns/op	       0 B/op	       0 allocs/op
BenchmarkBuildDeckVectorGOMAXPROCS1-10               	59904526	        18.62 ns/op	       0 B/op	       0 allocs/op
BenchmarkBuildDeckVectorGOMAXPROCS1-10               	62159791	        18.19 ns/op	       0 B/op	       0 allocs/op
BenchmarkBuildDeckVectorGOMAXPROCS1-10               	67232852	        18.75 ns/op	       0 B/op	       0 allocs/op
BenchmarkBuildDeckVectorGOMAXPROCSN-10               	64469914	        18.86 ns/op	       0 B/op	       0 allocs/op
BenchmarkBuildDeckVectorGOMAXPROCSN-10               	67040978	        18.17 ns/op	       0 B/op	       0 allocs/op
BenchmarkBuildDeckVectorGOMAXPROCSN-10               	64903894	        18.10 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketGOMAXPROCS1-10                   	680544256	         1.769 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketGOMAXPROCS1-10                   	680163344	         1.766 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketGOMAXPROCS1-10                   	685672494	         1.765 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketGOMAXPROCSN-10                   	689062636	         1.763 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketGOMAXPROCSN-10                   	689805160	         1.758 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketGOMAXPROCSN-10                   	673333290	         1.760 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardDepthGOMAXPROCS1-10                    	1000000000	         0.2395 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardDepthGOMAXPROCS1-10                    	1000000000	         0.2333 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardDepthGOMAXPROCS1-10                    	1000000000	         0.2353 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardDepthGOMAXPROCSN-10                    	1000000000	         0.2338 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardDepthGOMAXPROCSN-10                    	1000000000	         0.2371 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardDepthGOMAXPROCSN-10                    	1000000000	         0.2340 ns/op	       0 B/op	       0 allocs/op
BenchmarkEstimateWaitMSGOMAXPROCS1-10                	1000000000	         0.2373 ns/op	       0 B/op	       0 allocs/op
BenchmarkEstimateWaitMSGOMAXPROCS1-10                	1000000000	         0.2380 ns/op	       0 B/op	       0 allocs/op
BenchmarkEstimateWaitMSGOMAXPROCS1-10                	1000000000	         0.2357 ns/op	       0 B/op	       0 allocs/op
BenchmarkEstimateWaitMSGOMAXPROCSN-10                	1000000000	         0.2364 ns/op	       0 B/op	       0 allocs/op
BenchmarkEstimateWaitMSGOMAXPROCSN-10                	1000000000	         0.2366 ns/op	       0 B/op	       0 allocs/op
BenchmarkEstimateWaitMSGOMAXPROCSN-10                	1000000000	         0.2376 ns/op	       0 B/op	       0 allocs/op
BenchmarkNowUnixNanoGOMAXPROCS1-10                   	1000000000	         0.2472 ns/op	       0 B/op	       0 allocs/op
BenchmarkNowUnixNanoGOMAXPROCS1-10                   	1000000000	         0.2438 ns/op	       0 B/op	       0 allocs/op
BenchmarkNowUnixNanoGOMAXPROCS1-10                   	1000000000	         0.2454 ns/op	       0 B/op	       0 allocs/op
BenchmarkNowUnixNanoGOMAXPROCSN-10                   	1000000000	         0.2442 ns/op	       0 B/op	       0 allocs/op
BenchmarkNowUnixNanoGOMAXPROCSN-10                   	1000000000	         0.2458 ns/op	       0 B/op	       0 allocs/op
BenchmarkNowUnixNanoGOMAXPROCSN-10                   	1000000000	         0.2443 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketMessagePackGOMAXPROCS1-10        	14657882	        82.22 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketMessagePackGOMAXPROCS1-10        	14759715	        82.24 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketMessagePackGOMAXPROCS1-10        	14590729	        81.94 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketMessagePackGOMAXPROCSN-10        	14581237	        81.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketMessagePackGOMAXPROCSN-10        	14723580	        81.82 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketMessagePackGOMAXPROCSN-10        	14615830	        81.68 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketJSONGOMAXPROCS1-10               	4910523	       246.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketJSONGOMAXPROCS1-10               	4837482	       243.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketJSONGOMAXPROCS1-10               	4889450	       244.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketJSONGOMAXPROCSN-10               	4868509	       248.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketJSONGOMAXPROCSN-10               	4838538	       241.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketJSONGOMAXPROCSN-10               	4950594	       243.8 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	matchpoint/internal/ticket	86.585s
```

### `go vet ./...`

```text
```

### `staticcheck ./...`

```text
```
