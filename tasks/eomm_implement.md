# Task: IMPLEMENT — eomm

## Status

Planner completed the `eomm` contract and spec:
- `contracts/eomm_contract.go`
- `contracts/eomm_spec.md`

## Your Job

Implement Module 5: `eomm` exactly against the signed Planner artifacts.

Write implementation artifacts:
- `internal/eomm/eomm.go`
- `internal/eomm/eomm_test.go`
- `internal/eomm/README.md`

Implementation requirements:
- Tests cover every `B-EOMM-N:` behaviour before handoff.
- Benchmarks cover every hot-path row in the allocation budget table.
- Default tests use fakes only and require no Redis, Docker, network, or timers.
- Keep all pure routing, scoring, and rolling-window paths at `0 B/op`.
- Use only `matchpoint/contracts` for cross-module types.
- Do not change matchcore construction in this module.

## Implementor Command Outputs

### `GOCACHE=/private/tmp/matchpoint-gocache go test ./...`

```text
?   	matchpoint/contracts	[no test files]
ok  	matchpoint/internal/eomm	(cached)
ok  	matchpoint/internal/matchcore	0.273s
ok  	matchpoint/internal/redisqueue	0.225s
ok  	matchpoint/internal/ringbuffer	0.355s
ok  	matchpoint/internal/ticket	(cached)
```

### `GOCACHE=/private/tmp/matchpoint-gocache go vet ./...`

```text
```

### `GOCACHE=/private/tmp/matchpoint-gocache STATICCHECK_CACHE=/private/tmp/matchpoint-staticcheck-cache staticcheck ./...`

```text
```

### `GOCACHE=/private/tmp/matchpoint-gocache go test ./internal/eomm -bench=. -benchmem -run='^$' -count=3`

```text
BenchmarkRouteTicketGOMAXPROCS1-10             0 B/op  0 allocs/op
BenchmarkRouteTicketGOMAXPROCSCPU-10           0 B/op  0 allocs/op
BenchmarkRouteMatchOutcomeGOMAXPROCS1-10       0 B/op  0 allocs/op
BenchmarkRouteMatchOutcomeGOMAXPROCSCPU-10     0 B/op  0 allocs/op
BenchmarkApplyRouteGOMAXPROCS1-10              0 B/op  0 allocs/op
BenchmarkApplyRouteGOMAXPROCSCPU-10            0 B/op  0 allocs/op
BenchmarkScoreBreakdownGOMAXPROCS1-10          0 B/op  0 allocs/op
BenchmarkScoreBreakdownGOMAXPROCSCPU-10        0 B/op  0 allocs/op
BenchmarkScoreCandidateGOMAXPROCS1-10          0 B/op  0 allocs/op
BenchmarkScoreCandidateGOMAXPROCSCPU-10        0 B/op  0 allocs/op
BenchmarkRecordOutcomeGOMAXPROCS1-10           0 B/op  0 allocs/op
BenchmarkRecordOutcomeGOMAXPROCSCPU-10         0 B/op  0 allocs/op
PASS
ok  	matchpoint/internal/eomm	58.517s
```
