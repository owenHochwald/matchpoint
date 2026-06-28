# Task: PLAN — ringbuffer

## Status
Previous module `ticket` completed Checker with `CHECKER: WARN` in `reports/ticket_checker_report.md`.

Orchestrator appended the WARN annotations to `contracts/ticket_spec.md`:
- `STATICCHECK_WARN`: Staticcheck exits `0` but matches no packages under the current Go/staticcheck toolchain combination.
- `COVERAGE_WARN`: Ticket behaviour coverage exists but has partial direct-coverage caveats.

No Checker FAILs are present. Under the Checker verdict matrix, WARN permits handoff to the next module with annotations.

Current module: `ringbuffer` — lock-free ring buffer for WebSocket decoupling.

## Your Job
Produce the signed Planner contract for Module 2: `ringbuffer`.

Write exactly these output artifacts:
- `contracts/ringbuffer_contract.go`
- `contracts/ringbuffer_spec.md`

The contract must include only pure Go type/interface contracts: interface definitions, struct layouts with field-level invariant comments, method signatures with explicit parameter and return types, and no function bodies except trivial zero-value constructors if absolutely needed.

Contract scope:
- A lock-free ring buffer suitable for the ingestion-to-match-core handoff.
- Per-CPU shard contract and shard selection by player identity.
- Non-blocking write path with a single bounded 10us backpressure wait before rejection.
- Read/drain contract for match-core consumption.
- Fixed capacity / preallocated slot ownership semantics.
- Exact handling of full, empty, closed, and duplicate-publisher edge cases.
- Atomic head/tail ownership and memory-ordering invariants.
- No direct dependency on concrete `internal/ticket` types. Cross-module input should be expressed by an interface, generic typed slot, or explicitly contracted value shape that does not import downstream implementation packages.

The behaviour spec must include:
- At least 5 numbered behaviours in the exact format `B-RINGBUFFER-N: Given <precondition>, when <action>, then <observable outcome>.`
- Behaviour coverage for every public symbol in `contracts/ringbuffer_contract.go`.
- Allocation Budget Table for every hot-path function.
- Edge Case Register covering races, ABA/wraparound, full/empty transitions, backpressure timeout, overwrite prohibition for ingestion queues, close/drain semantics, false sharing/cache-line alignment, and shard selection.

Do not write implementation files under `internal/`.

## Relevant Spec Sections
- `docs/FEATURES.md` §1 System Topology
- `docs/FEATURES.md` §3.4 Ingestion → Queue Handoff
- `docs/FEATURES.md` §3.5 Backpressure
- `docs/FEATURES.md` §10.1 Hot-Path Allocation Budget (`RingBuffer.Write`, `RingBuffer.Read`)
- `docs/FEATURES.md` §10.3 Struct Alignment
- `docs/FEATURES.md` §11.2 Shared State Inventory
- `docs/FEATURES.md` §11.3 Channel Discipline
- `docs/FEATURES.md` §14 Delivery Sequence & Dependency Graph
- `docs/AGENTS.md` Planner Agent responsibilities and exit criteria
- `docs/GIT_POLICY.md` Commit granularity and branch policy
- `contracts/ticket_contract.go` for the upstream handoff vocabulary only; do not import `internal/ticket`
- `reports/ticket_checker_report.md` for inherited warnings

## Inputs Available
- `contracts/ticket_contract.go`
- `contracts/ticket_spec.md`
- `reports/ticket_checker_report.md`
- `docs/FEATURES.md`
- `docs/MATCH_SPEC.md`
- `docs/AGENTS.md`
- `docs/GIT_POLICY.md`

## Checker Report (if re-cycle)
Not applicable. This is the first Planner dispatch for `ringbuffer`.
