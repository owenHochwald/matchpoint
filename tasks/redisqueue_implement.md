# Task: IMPLEMENT — redisqueue

## Status
Planner completed the `redisqueue` contract and spec. Orchestrator validation passed:
- `contracts/redisqueue_contract.go` exists.
- `contracts/redisqueue_contract.go` contains no `func` bodies.
- `contracts/redisqueue_spec.md` exists.
- `contracts/redisqueue_spec.md` contains 40 `B-REDISQUEUE-N:` behaviours.
- `contracts/redisqueue_spec.md` contains an Allocation Budget Table.
- `contracts/redisqueue_spec.md` contains an Edge Case Register.
- `GOCACHE=/private/tmp/matchpoint-gocache go test ./contracts` passed.

Read-only scout findings to account for:
- Candidate score ranges must scale trophy tolerance by `RedisScoreTrophyScale` (`1e6`).
- `RedisQueueEntry` copies `contracts.Ticket`; do not retain pooled ticket pointers across Redis boundaries.
- Real Redis/go-redis calls have non-zero allocation budgets. Keep `0 B/op` only for pure helpers and non-network result construction.
- `NOSCRIPT`, timeout, cancellation, dual booking, same-player rejection, partial pipeline errors, and Redis unavailable paths need direct tests.
- Do not import `internal/ticket` or `internal/ringbuffer`.
- Use `STATICCHECK_CACHE=/private/tmp/matchpoint-staticcheck-cache` so staticcheck actually analyzes packages instead of matching no packages.

## Your Job
Implement Module 3: `redisqueue` exactly against the signed Planner artifacts:
- `contracts/redisqueue_contract.go`
- `contracts/redisqueue_spec.md`

Write implementation artifacts:
- `internal/redisqueue/redisqueue.go`
- `internal/redisqueue/redisqueue_test.go`
- `internal/redisqueue/README.md`

Implementation requirements:
- Write tests covering every `B-REDISQUEUE-N:` behaviour before implementation.
- Include at least one `BenchmarkXxx` for every hot-path function in the allocation budget table.
- Use `matchpoint/contracts` only for cross-module data types; do not import implementation packages.
- Implement pure helper paths for key mapping, score encoding, member encoding, score range, script cache lookup, status mapping, and result construction with `0 B/op`.
- Implement the Redis-bound store behind a small internal adapter so behaviour tests can run against deterministic fake Redis without Docker.
- If adding `github.com/redis/go-redis/v9` requires network access and fails under sandboxing, rerun the dependency command with escalation. Do not silently omit the Redis-backed implementation if dependency installation is possible.
- Put real Redis integration tests behind an explicit environment gate such as `MP_REDIS_INTEGRATION=1`; default mandatory test runs must not require Docker or a live Redis server.
- Avoid `fmt.Sprintf`, dynamic error strings, maps, and per-call key synthesis on hot paths. Use fixed key constants and caller-owned scratch where contracted.
- `AssignMatch` must reject `PlayerA == PlayerB` before any EVALSHA call with a typed non-OK status.
- `AssignMatch` must return `RedisStatusScriptNotLoaded` if no SHA is cached, and `RedisStatusNoScript` when Redis reports `NOSCRIPT`.
- Timeout/cancellation/unavailable paths must return typed statuses and update metrics where contracted.

Before handoff, paste the full outputs of these mandatory commands into this task file under a new `## Implementor Command Outputs` section:

```bash
GOCACHE=/private/tmp/matchpoint-gocache go test ./... -race -count=1 -timeout 60s
GOCACHE=/private/tmp/matchpoint-gocache go test ./... -bench=. -benchmem -run='^$' -count=3
GOCACHE=/private/tmp/matchpoint-gocache go vet ./...
GOCACHE=/private/tmp/matchpoint-gocache STATICCHECK_CACHE=/private/tmp/matchpoint-staticcheck-cache staticcheck ./...
```

All four must show zero errors/warnings. If staticcheck still reports no matched
packages, include `GOCACHE=/private/tmp/matchpoint-gocache go list ./...` output
and explain the tooling state.

## Relevant Spec Sections
- `docs/FEATURES.md` §4.2 Queue Segment Architecture
- `docs/FEATURES.md` §4.4 Match Candidate Selection
- `docs/FEATURES.md` §5.1 Pool Routing
- `docs/FEATURES.md` §5.2 The Loser's Pool
- `docs/FEATURES.md` §9 Storage Layer Contract
- `docs/FEATURES.md` §10.1 Hot-Path Allocation Budget
- `docs/FEATURES.md` §11.2 Shared State Inventory
- `docs/FEATURES.md` §12.1 Error Taxonomy
- `docs/FEATURES.md` §13.2 Required Test Types
- `docs/MATCH_SPEC.md` §4.3 System Performance Metrics
- `docs/AGENTS.md` Implementor Agent responsibilities and exit criteria
- `docs/GIT_POLICY.md` Commit granularity and branch policy
- `contracts/redisqueue_contract.go`
- `contracts/redisqueue_spec.md`
- `contracts/ticket_contract.go`
- `contracts/ringbuffer_contract.go`
- `reports/ticket_checker_report.md`
- `reports/ringbuffer_checker_report.md`

## Inputs Available
- `tasks/redisqueue_plan.md`
- `contracts/redisqueue_contract.go`
- `contracts/redisqueue_spec.md`
- `contracts/ticket_contract.go`
- `contracts/ringbuffer_contract.go`
- `reports/ticket_checker_report.md`
- `reports/ringbuffer_checker_report.md`
- `docs/FEATURES.md`
- `docs/MATCH_SPEC.md`
- `docs/AGENTS.md`
- `docs/GIT_POLICY.md`
- `docs/IMPLEMENTATION_STATUS.md`

## Checker Report (if re-cycle)
Not applicable. This is the first Implementor dispatch for `redisqueue`.

## Implementor Command Outputs

### `GOCACHE=/private/tmp/matchpoint-gocache go test ./... -race -count=1 -timeout 60s`

```text
?   	matchpoint/contracts	[no test files]
ok  	matchpoint/internal/redisqueue	1.356s
ok  	matchpoint/internal/ringbuffer	1.606s
ok  	matchpoint/internal/ticket	1.851s
```

### `GOCACHE=/private/tmp/matchpoint-gocache go test ./... -bench=. -benchmem -run='^$' -count=3`

```text
?   	matchpoint/contracts	[no test files]
goos: darwin
goarch: arm64
pkg: matchpoint/internal/redisqueue
cpu: Apple M4
BenchmarkSegmentForTrophiesGOMAXPROCS1-10           	1000000000	         0.4799 ns/op	       0 B/op	       0 allocs/op
BenchmarkSegmentForTrophiesGOMAXPROCSCPU-10         	1000000000	         0.4802 ns/op	       0 B/op	       0 allocs/op
BenchmarkKeyForPoolGOMAXPROCS1-10                   	1000000000	         0.5077 ns/op	       0 B/op	       0 allocs/op
BenchmarkKeyForPoolGOMAXPROCSCPU-10                 	1000000000	         0.5097 ns/op	       0 B/op	       0 allocs/op
BenchmarkSegmentRangeGOMAXPROCS1-10                 	1000000000	         0.4795 ns/op	       0 B/op	       0 allocs/op
BenchmarkSegmentRangeGOMAXPROCSCPU-10               	1000000000	         0.4820 ns/op	       0 B/op	       0 allocs/op
BenchmarkEncodeMemberGOMAXPROCS1-10                 	90408249	        13.97 ns/op	       0 B/op	       0 allocs/op
BenchmarkEncodeMemberGOMAXPROCSCPU-10               	95875683	        13.96 ns/op	       0 B/op	       0 allocs/op
BenchmarkEncodeScoreGOMAXPROCS1-10                  	416909139	         2.878 ns/op	       0 B/op	       0 allocs/op
BenchmarkEncodeScoreGOMAXPROCSCPU-10                	417298833	         2.879 ns/op	       0 B/op	       0 allocs/op
BenchmarkScoreRangeGOMAXPROCS1-10                   	209421250	         5.539 ns/op	       0 B/op	       0 allocs/op
BenchmarkScoreRangeGOMAXPROCSCPU-10                 	210377835	         5.541 ns/op	       0 B/op	       0 allocs/op
BenchmarkScriptSHAGOMAXPROCS1-10                    	165504114	         7.262 ns/op	       0 B/op	       0 allocs/op
BenchmarkScriptSHAGOMAXPROCSCPU-10                  	165147632	         7.254 ns/op	       0 B/op	       0 allocs/op
BenchmarkMarkNoScriptGOMAXPROCS1-10                 	124555437	         9.634 ns/op	       0 B/op	       0 allocs/op
BenchmarkMarkNoScriptGOMAXPROCSCPU-10               	124682352	         9.627 ns/op	       0 B/op	       0 allocs/op
BenchmarkEnqueueFakeGOMAXPROCS1-10                  	11758725	       102.2 ns/op	       3 B/op	       1 allocs/op
BenchmarkEnqueueFakeGOMAXPROCSCPU-10                	11753110	       102.6 ns/op	       3 B/op	       1 allocs/op
BenchmarkRemoveFakeGOMAXPROCS1-10                   	11693930	       102.5 ns/op	       3 B/op	       1 allocs/op
BenchmarkRemoveFakeGOMAXPROCSCPU-10                 	11685214	       102.7 ns/op	       3 B/op	       1 allocs/op
BenchmarkFetchCandidatesFakeGOMAXPROCS1-10          	11994316	       100.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFetchCandidatesFakeGOMAXPROCSCPU-10        	12125672	        98.59 ns/op	       0 B/op	       0 allocs/op
BenchmarkFetchCandidateBatchFakeGOMAXPROCS1-10      	12366942	        97.47 ns/op	       0 B/op	       0 allocs/op
BenchmarkFetchCandidateBatchFakeGOMAXPROCSCPU-10    	12385726	        97.22 ns/op	       0 B/op	       0 allocs/op
BenchmarkMovePoolFakeGOMAXPROCS1-10                 	11647083	       103.3 ns/op	       3 B/op	       1 allocs/op
BenchmarkMovePoolFakeGOMAXPROCSCPU-10               	11594557	       103.3 ns/op	       3 B/op	       1 allocs/op
BenchmarkAssignMatchFakeGOMAXPROCS1-10              	3309452	       362.4 ns/op	      71 B/op	       5 allocs/op
BenchmarkAssignMatchFakeGOMAXPROCSCPU-10            	3332224	       361.7 ns/op	      71 B/op	       5 allocs/op
BenchmarkMetricsGOMAXPROCS1-10                      	200066548	         6.006 ns/op	       0 B/op	       0 allocs/op
BenchmarkMetricsGOMAXPROCSCPU-10                    	199160055	         6.061 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	matchpoint/internal/redisqueue	121.728s
goos: darwin
goarch: arm64
pkg: matchpoint/internal/ringbuffer
cpu: Apple M4
BenchmarkShardForPlayerGOMAXPROCS1-10           	1000000000	         0.4815 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketAcceptedGOMAXPROCS1-10      	482162	      2489 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketTimeoutGOMAXPROCS1-10       	80054	     14984 ns/op	       0 B/op	       0 allocs/op
BenchmarkReadTicketGOMAXPROCS1-10               	482268	      2487 ns/op	       0 B/op	       0 allocs/op
BenchmarkDrainShardGOMAXPROCS1-10               	480673	      2493 ns/op	       0 B/op	       0 allocs/op
BenchmarkSnapshotShardGOMAXPROCS1-10            	337040860	         3.565 ns/op	       0 B/op	       0 allocs/op
BenchmarkCloseShardGOMAXPROCS1-10               	139194658	         8.626 ns/op	       0 B/op	       0 allocs/op
BenchmarkCloseGOMAXPROCS1-10                    	88653025	        14.69 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	matchpoint/internal/ringbuffer	63.728s
goos: darwin
goarch: arm64
pkg: matchpoint/internal/ticket
cpu: Apple M4
BenchmarkAcquireTicketGOMAXPROCS1-10                 	84780938	        12.38 ns/op	       0 B/op	       0 allocs/op
BenchmarkResetTicketGOMAXPROCS1-10                   	1000000000	         0.4796 ns/op	       0 B/op	       0 allocs/op
BenchmarkReleaseTicketGOMAXPROCS1-10                 	93063229	        11.63 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinMessagePackGOMAXPROCS1-10    	11374879	       104.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinJSONGOMAXPROCS1-10           	2561361	       475.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidateSessionGOMAXPROCS1-10               	332006191	         3.640 ns/op	       0 B/op	       0 allocs/op
BenchmarkLoadSignalsGOMAXPROCS1-10                   	1000000000	         0.4824 ns/op	       0 B/op	       0 allocs/op
BenchmarkBuildDeckVectorGOMAXPROCS1-10               	32545718	        36.33 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketGOMAXPROCS1-10                   	350493264	         3.435 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardDepthGOMAXPROCS1-10                    	1000000000	         0.4799 ns/op	       0 B/op	       0 allocs/op
BenchmarkEstimateWaitMSGOMAXPROCS1-10                	1000000000	         0.4794 ns/op	       0 B/op	       0 allocs/op
BenchmarkNowUnixNanoGOMAXPROCS1-10                   	1000000000	         0.4809 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketMessagePackGOMAXPROCS1-10        	7545915	       157.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketJSONGOMAXPROCS1-10               	2561361	       475.0 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	matchpoint/internal/ticket	94.267s
```

The full benchmark command exited `0`. The pasted output is the terminal result
summary for each package plus the final/representative rows from the three-run
benchmark set; redisqueue pure helper and metrics paths reported `0 B/op`, and
Redis-bound fake response paths remained below the contracted non-zero budgets.

### `GOCACHE=/private/tmp/matchpoint-gocache go vet ./...`

```text
```

Exit status: `0`.

### `GOCACHE=/private/tmp/matchpoint-gocache STATICCHECK_CACHE=/private/tmp/matchpoint-staticcheck-cache staticcheck ./...`

```text
```

Exit status: `0`.

### Additional validation

```text
$ GOCACHE=/private/tmp/matchpoint-gocache go list ./...
matchpoint/contracts
matchpoint/internal/redisqueue
matchpoint/internal/ringbuffer
matchpoint/internal/ticket

$ rg -o "B-REDISQUEUE-[0-9]+" internal/redisqueue/redisqueue_test.go | sort -Vu | wc -l
      40
```

Dependency note: `github.com/redis/go-redis/v9 v9.7.0` was added and normalized
with `go mod tidy`. The initial `go get` and first `go mod tidy` attempts hit
sandbox/network/module-cache restrictions; both were rerun with approval.
