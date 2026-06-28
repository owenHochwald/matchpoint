# Task: IMPLEMENT — ringbuffer

## Status
Planner completed the `ringbuffer` contract and spec. Orchestrator validation passed:
- `contracts/ringbuffer_contract.go` exists.
- `contracts/ringbuffer_contract.go` contains no `func` bodies.
- `contracts/ringbuffer_spec.md` exists.
- `contracts/ringbuffer_spec.md` contains 30 `B-RINGBUFFER-N:` behaviours.
- `contracts/ringbuffer_spec.md` contains an Allocation Budget Table.
- `contracts/ringbuffer_spec.md` contains an Edge Case Register.
- No `internal/ticket` dependency appears in the ringbuffer contract/spec.
- `GOCACHE=/private/tmp/matchpoint-gocache go test ./contracts` passed.

Inherited warning from `ticket`: `staticcheck ./...` may exit `0` while matching no packages under the current Go/staticcheck toolchain. Paste exact output and do not silently treat no-package matching as clean analysis.

## Your Job
Implement Module 2: `ringbuffer` exactly against the signed Planner artifacts:
- `contracts/ringbuffer_contract.go`
- `contracts/ringbuffer_spec.md`

Write implementation artifacts:
- `internal/ringbuffer/ringbuffer.go`
- `internal/ringbuffer/ringbuffer_test.go`
- `internal/ringbuffer/README.md`

Implementation requirements:
- Write tests covering every `B-RINGBUFFER-N:` behaviour before implementation.
- Include at least one `BenchmarkXxx` for every hot-path function in the allocation budget table.
- Implement a per-shard, preallocated, lock-free or atomics-first ingestion ring.
- No heap allocation on steady-state hot paths: `WriteTicket`, `ReadTicket`, `DrainShard`, `SnapshotShard`, `CloseShard`, `Close`, and `ShardForPlayer`.
- The ingestion ring must never overwrite old tickets when full.
- `WriteTicket` must directly implement and test the single bounded 10us full-shard wait and freed-during-wait acceptance.
- Use atomics for shared cursors, slot state, shard state, and duplicate-publisher tracking.
- Keep cache-line isolation/padding for hot cursor state; annotate any measured/justified deviations.
- Do not import `internal/ticket`; use `contracts.Ticket` or local aliases only.
- Do not use `interface{}` or `any` on hot paths.

Before handoff, paste the full outputs of these mandatory commands into this task file under a new `## Implementor Command Outputs` section:

```bash
go test ./... -race -count=1 -timeout 60s
go test ./... -bench=. -benchmem -run='^$' -count=3
go vet ./...
staticcheck ./...
```

All four must show zero errors/warnings unless the staticcheck no-package tooling warning persists; if it does, record the exact output and note `go list ./...` output for comparison.

## Relevant Spec Sections
- `docs/FEATURES.md` §1 System Topology
- `docs/FEATURES.md` §3.4 Ingestion → Queue Handoff
- `docs/FEATURES.md` §3.5 Backpressure
- `docs/FEATURES.md` §10.1 Hot-Path Allocation Budget
- `docs/FEATURES.md` §10.3 Struct Alignment
- `docs/FEATURES.md` §11.2 Shared State Inventory
- `docs/FEATURES.md` §11.3 Channel Discipline
- `docs/FEATURES.md` §14 Delivery Sequence & Dependency Graph
- `docs/AGENTS.md` Implementor Agent responsibilities and exit criteria
- `docs/GIT_POLICY.md` Commit granularity and branch policy
- `contracts/ticket_contract.go`
- `contracts/ringbuffer_contract.go`
- `contracts/ringbuffer_spec.md`
- `reports/ticket_checker_report.md`

## Inputs Available
- `tasks/ringbuffer_plan.md`
- `contracts/ticket_contract.go`
- `contracts/ringbuffer_contract.go`
- `contracts/ringbuffer_spec.md`
- `reports/ticket_checker_report.md`
- `docs/FEATURES.md`
- `docs/MATCH_SPEC.md`
- `docs/AGENTS.md`
- `docs/GIT_POLICY.md`

## Checker Report (if re-cycle)
Not applicable. This is the first Implementor dispatch for `ringbuffer`.

## Implementor Command Outputs

### `go test ./... -race -count=1 -timeout 60s`

```text
?   	matchpoint/contracts	[no test files]
ok  	matchpoint/internal/ringbuffer	1.411s
ok  	matchpoint/internal/ticket	1.263s
```

### `go test ./... -bench=. -benchmem -run='^$' -count=3`

```text
?   	matchpoint/contracts	[no test files]
goos: darwin
goarch: arm64
pkg: matchpoint/internal/ringbuffer
cpu: Apple M4
BenchmarkShardForPlayerGOMAXPROCS1-10           	1000000000	         0.2315 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardForPlayerGOMAXPROCS1-10           	1000000000	         0.2269 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardForPlayerGOMAXPROCS1-10           	1000000000	         0.2272 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardForPlayerGOMAXPROCSCPU-10         	1000000000	         0.2288 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardForPlayerGOMAXPROCSCPU-10         	1000000000	         0.2273 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardForPlayerGOMAXPROCSCPU-10         	1000000000	         0.2273 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketAcceptedGOMAXPROCS1-10      	  955492	      1283 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketAcceptedGOMAXPROCS1-10      	  968245	      1285 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketAcceptedGOMAXPROCS1-10      	  963210	      1287 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketAcceptedGOMAXPROCSCPU-10    	  939175	      1287 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketAcceptedGOMAXPROCSCPU-10    	  926607	      1287 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketAcceptedGOMAXPROCSCPU-10    	  935553	      1287 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketTimeoutGOMAXPROCS1-10       	   99708	     12035 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketTimeoutGOMAXPROCS1-10       	   99717	     12031 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketTimeoutGOMAXPROCS1-10       	   99710	     12032 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketTimeoutGOMAXPROCSCPU-10     	   88333	     13641 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketTimeoutGOMAXPROCSCPU-10     	   87991	     13621 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketTimeoutGOMAXPROCSCPU-10     	   88510	     13555 ns/op	       0 B/op	       0 allocs/op
BenchmarkReadTicketGOMAXPROCS1-10               	  923834	      1292 ns/op	       0 B/op	       0 allocs/op
BenchmarkReadTicketGOMAXPROCS1-10               	  933168	      1291 ns/op	       0 B/op	       0 allocs/op
BenchmarkReadTicketGOMAXPROCS1-10               	  941200	      1291 ns/op	       0 B/op	       0 allocs/op
BenchmarkReadTicketGOMAXPROCSCPU-10             	  929544	      1292 ns/op	       0 B/op	       0 allocs/op
BenchmarkReadTicketGOMAXPROCSCPU-10             	  928411	      1291 ns/op	       0 B/op	       0 allocs/op
BenchmarkReadTicketGOMAXPROCSCPU-10             	  934849	      1291 ns/op	       0 B/op	       0 allocs/op
BenchmarkDrainShardGOMAXPROCS1-10               	  933621	      1295 ns/op	       0 B/op	       0 allocs/op
BenchmarkDrainShardGOMAXPROCS1-10               	  943018	      1295 ns/op	       0 B/op	       0 allocs/op
BenchmarkDrainShardGOMAXPROCS1-10               	  932318	      1295 ns/op	       0 B/op	       0 allocs/op
BenchmarkDrainShardGOMAXPROCSCPU-10             	  927721	      1296 ns/op	       0 B/op	       0 allocs/op
BenchmarkDrainShardGOMAXPROCSCPU-10             	  930754	      1297 ns/op	       0 B/op	       0 allocs/op
BenchmarkDrainShardGOMAXPROCSCPU-10             	  915790	      1295 ns/op	       0 B/op	       0 allocs/op
BenchmarkSnapshotShardGOMAXPROCS1-10            	644220502	         1.862 ns/op	       0 B/op	       0 allocs/op
BenchmarkSnapshotShardGOMAXPROCS1-10            	649660450	         1.850 ns/op	       0 B/op	       0 allocs/op
BenchmarkSnapshotShardGOMAXPROCS1-10            	649591725	         1.848 ns/op	       0 B/op	       0 allocs/op
BenchmarkSnapshotShardGOMAXPROCSCPU-10          	643372844	         1.862 ns/op	       0 B/op	       0 allocs/op
BenchmarkSnapshotShardGOMAXPROCSCPU-10          	649220372	         1.847 ns/op	       0 B/op	       0 allocs/op
BenchmarkSnapshotShardGOMAXPROCSCPU-10          	647738313	         1.849 ns/op	       0 B/op	       0 allocs/op
BenchmarkCloseShardGOMAXPROCS1-10               	285915957	         4.178 ns/op	       0 B/op	       0 allocs/op
BenchmarkCloseShardGOMAXPROCS1-10               	289445694	         4.168 ns/op	       0 B/op	       0 allocs/op
BenchmarkCloseShardGOMAXPROCS1-10               	289487181	         4.171 ns/op	       0 B/op	       0 allocs/op
BenchmarkCloseShardGOMAXPROCSCPU-10             	289271172	         4.156 ns/op	       0 B/op	       0 allocs/op
BenchmarkCloseShardGOMAXPROCSCPU-10             	289432226	         4.170 ns/op	       0 B/op	       0 allocs/op
BenchmarkCloseShardGOMAXPROCSCPU-10             	287147859	         4.622 ns/op	       0 B/op	       0 allocs/op
BenchmarkCloseGOMAXPROCS1-10                    	162573621	         7.364 ns/op	       0 B/op	       0 allocs/op
BenchmarkCloseGOMAXPROCS1-10                    	172393244	         6.924 ns/op	       0 B/op	       0 allocs/op
BenchmarkCloseGOMAXPROCS1-10                    	173129870	         6.929 ns/op	       0 B/op	       0 allocs/op
BenchmarkCloseGOMAXPROCSCPU-10                  	171340156	         6.913 ns/op	       0 B/op	       0 allocs/op
BenchmarkCloseGOMAXPROCSCPU-10                  	173190193	         6.961 ns/op	       0 B/op	       0 allocs/op
BenchmarkCloseGOMAXPROCSCPU-10                  	172068964	         7.026 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	matchpoint/internal/ringbuffer	61.365s
goos: darwin
goarch: arm64
pkg: matchpoint/internal/ticket
cpu: Apple M4
BenchmarkAcquireTicketGOMAXPROCS1-10                 	167170863	         6.913 ns/op	       0 B/op	       0 allocs/op
BenchmarkAcquireTicketGOMAXPROCS1-10                 	194608650	         6.065 ns/op	       0 B/op	       0 allocs/op
BenchmarkAcquireTicketGOMAXPROCS1-10                 	190802686	         5.938 ns/op	       0 B/op	       0 allocs/op
BenchmarkAcquireTicketGOMAXPROCSN-10                 	202698978	         6.532 ns/op	       0 B/op	       0 allocs/op
BenchmarkAcquireTicketGOMAXPROCSN-10                 	187501329	         6.997 ns/op	       0 B/op	       0 allocs/op
BenchmarkAcquireTicketGOMAXPROCSN-10                 	202346445	         6.193 ns/op	       0 B/op	       0 allocs/op
BenchmarkResetTicketGOMAXPROCS1-10                   	1000000000	         0.2447 ns/op	       0 B/op	       0 allocs/op
BenchmarkResetTicketGOMAXPROCS1-10                   	1000000000	         0.2416 ns/op	       0 B/op	       0 allocs/op
BenchmarkResetTicketGOMAXPROCS1-10                   	1000000000	         0.2421 ns/op	       0 B/op	       0 allocs/op
BenchmarkResetTicketGOMAXPROCSN-10                   	1000000000	         0.2422 ns/op	       0 B/op	       0 allocs/op
BenchmarkResetTicketGOMAXPROCSN-10                   	1000000000	         0.2437 ns/op	       0 B/op	       0 allocs/op
BenchmarkResetTicketGOMAXPROCSN-10                   	1000000000	         0.2400 ns/op	       0 B/op	       0 allocs/op
BenchmarkReleaseTicketGOMAXPROCS1-10                 	180463237	         6.902 ns/op	       0 B/op	       0 allocs/op
BenchmarkReleaseTicketGOMAXPROCS1-10                 	189547195	         6.275 ns/op	       0 B/op	       0 allocs/op
BenchmarkReleaseTicketGOMAXPROCS1-10                 	187611308	         5.688 ns/op	       0 B/op	       0 allocs/op
BenchmarkReleaseTicketGOMAXPROCSN-10                 	183020175	         6.648 ns/op	       0 B/op	       0 allocs/op
BenchmarkReleaseTicketGOMAXPROCSN-10                 	191891170	         6.757 ns/op	       0 B/op	       0 allocs/op
BenchmarkReleaseTicketGOMAXPROCSN-10                 	205841245	         6.003 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinMessagePackGOMAXPROCS1-10    	21614659	        53.64 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinMessagePackGOMAXPROCS1-10    	21149379	        53.50 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinMessagePackGOMAXPROCS1-10    	20939666	        52.97 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinMessagePackGOMAXPROCSN-10    	21729240	        53.61 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinMessagePackGOMAXPROCSN-10    	22353271	        53.64 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinMessagePackGOMAXPROCSN-10    	22181008	        53.37 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinJSONGOMAXPROCS1-10           	5504184	       216.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinJSONGOMAXPROCS1-10           	5529133	       218.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinJSONGOMAXPROCS1-10           	5483539	       218.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinJSONGOMAXPROCSN-10           	5539892	       218.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinJSONGOMAXPROCSN-10           	5483394	       219.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkDecodeQueueJoinJSONGOMAXPROCSN-10           	5493782	       217.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidateSessionGOMAXPROCS1-10               	639594496	         1.875 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidateSessionGOMAXPROCS1-10               	639814166	         1.876 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidateSessionGOMAXPROCS1-10               	638000366	         1.878 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidateSessionGOMAXPROCSN-10               	639349142	         1.876 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidateSessionGOMAXPROCSN-10               	639018606	         1.874 ns/op	       0 B/op	       0 allocs/op
BenchmarkValidateSessionGOMAXPROCSN-10               	640244145	         1.875 ns/op	       0 B/op	       0 allocs/op
BenchmarkLoadSignalsGOMAXPROCS1-10                   	1000000000	         0.2486 ns/op	       0 B/op	       0 allocs/op
BenchmarkLoadSignalsGOMAXPROCS1-10                   	1000000000	         0.2433 ns/op	       0 B/op	       0 allocs/op
BenchmarkLoadSignalsGOMAXPROCS1-10                   	1000000000	         0.2428 ns/op	       0 B/op	       0 allocs/op
BenchmarkLoadSignalsGOMAXPROCSN-10                   	1000000000	         0.2436 ns/op	       0 B/op	       0 allocs/op
BenchmarkLoadSignalsGOMAXPROCSN-10                   	1000000000	         0.2423 ns/op	       0 B/op	       0 allocs/op
BenchmarkLoadSignalsGOMAXPROCSN-10                   	1000000000	         0.2417 ns/op	       0 B/op	       0 allocs/op
BenchmarkBuildDeckVectorGOMAXPROCS1-10               	63153184	        18.70 ns/op	       0 B/op	       0 allocs/op
BenchmarkBuildDeckVectorGOMAXPROCS1-10               	59524423	        18.73 ns/op	       0 B/op	       0 allocs/op
BenchmarkBuildDeckVectorGOMAXPROCS1-10               	60534213	        18.86 ns/op	       0 B/op	       0 allocs/op
BenchmarkBuildDeckVectorGOMAXPROCSN-10               	61464649	        18.66 ns/op	       0 B/op	       0 allocs/op
BenchmarkBuildDeckVectorGOMAXPROCSN-10               	64475256	        18.79 ns/op	       0 B/op	       0 allocs/op
BenchmarkBuildDeckVectorGOMAXPROCSN-10               	63134773	        18.83 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketGOMAXPROCS1-10                   	672818594	         1.777 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketGOMAXPROCS1-10                   	677307926	         1.780 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketGOMAXPROCS1-10                   	679339652	         1.771 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketGOMAXPROCSN-10                   	681867415	         1.773 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketGOMAXPROCSN-10                   	676417728	         1.777 ns/op	       0 B/op	       0 allocs/op
BenchmarkWriteTicketGOMAXPROCSN-10                   	671986872	         1.777 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardDepthGOMAXPROCS1-10                    	1000000000	         0.2417 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardDepthGOMAXPROCS1-10                    	1000000000	         0.2408 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardDepthGOMAXPROCS1-10                    	1000000000	         0.2380 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardDepthGOMAXPROCSN-10                    	1000000000	         0.2395 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardDepthGOMAXPROCSN-10                    	1000000000	         0.2417 ns/op	       0 B/op	       0 allocs/op
BenchmarkShardDepthGOMAXPROCSN-10                    	1000000000	         0.2378 ns/op	       0 B/op	       0 allocs/op
BenchmarkEstimateWaitMSGOMAXPROCS1-10                	1000000000	         0.2408 ns/op	       0 B/op	       0 allocs/op
BenchmarkEstimateWaitMSGOMAXPROCS1-10                	1000000000	         0.2372 ns/op	       0 B/op	       0 allocs/op
BenchmarkEstimateWaitMSGOMAXPROCS1-10                	1000000000	         0.2413 ns/op	       0 B/op	       0 allocs/op
BenchmarkEstimateWaitMSGOMAXPROCSN-10                	1000000000	         0.2425 ns/op	       0 B/op	       0 allocs/op
BenchmarkEstimateWaitMSGOMAXPROCSN-10                	1000000000	         0.2417 ns/op	       0 B/op	       0 allocs/op
BenchmarkEstimateWaitMSGOMAXPROCSN-10                	1000000000	         0.2398 ns/op	       0 B/op	       0 allocs/op
BenchmarkNowUnixNanoGOMAXPROCS1-10                   	1000000000	         0.2476 ns/op	       0 B/op	       0 allocs/op
BenchmarkNowUnixNanoGOMAXPROCS1-10                   	1000000000	         0.2485 ns/op	       0 B/op	       0 allocs/op
BenchmarkNowUnixNanoGOMAXPROCS1-10                   	1000000000	         0.2480 ns/op	       0 B/op	       0 allocs/op
BenchmarkNowUnixNanoGOMAXPROCSN-10                   	1000000000	         0.2482 ns/op	       0 B/op	       0 allocs/op
BenchmarkNowUnixNanoGOMAXPROCSN-10                   	1000000000	         0.2477 ns/op	       0 B/op	       0 allocs/op
BenchmarkNowUnixNanoGOMAXPROCSN-10                   	1000000000	         0.2479 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketMessagePackGOMAXPROCS1-10        	13966866	        83.21 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketMessagePackGOMAXPROCS1-10        	14502781	        83.48 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketMessagePackGOMAXPROCS1-10        	14244076	        82.99 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketMessagePackGOMAXPROCSN-10        	13956117	        83.96 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketMessagePackGOMAXPROCSN-10        	13995596	        83.68 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketMessagePackGOMAXPROCSN-10        	14307835	        84.44 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketJSONGOMAXPROCS1-10               	4916646	       244.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketJSONGOMAXPROCS1-10               	4820620	       248.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketJSONGOMAXPROCS1-10               	4764432	       249.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketJSONGOMAXPROCSN-10               	4754481	       249.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketJSONGOMAXPROCSN-10               	4804764	       248.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkParseTicketJSONGOMAXPROCSN-10               	4816593	       247.1 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	matchpoint/internal/ticket	86.379s
```

### `go vet ./...`

```text
```

### `staticcheck ./...`

```text
warning: "./..." matched no packages
```

The inherited staticcheck package-resolution warning persists: the command exited `0` but emitted the warning above.

### `go list ./...`

```text
matchpoint/contracts
matchpoint/internal/ringbuffer
matchpoint/internal/ticket
```
