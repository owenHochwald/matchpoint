# Task: PLAN — ticket

## Status
No module artifacts exist yet for `ticket`.

The governing documents are available at:
- `docs/FEATURES.md`
- `docs/MATCH_SPEC.md`
- `docs/AGENTS.md`
- `docs/GIT_POLICY.md`

The repository is on module branch `agent/ticket`. Treat `docs/MATCH_SPEC.md` as the tiebreaker when it conflicts with `docs/FEATURES.md`.

## Your Job
Produce the signed Planner contract for Module 1: `ticket` — Ticket struct, pool, and ingestion contract.

Write exactly these output artifacts:
- `contracts/ticket_contract.go`
- `contracts/ticket_spec.md`

The contract must include only pure Go type/interface contracts: interface definitions, struct layouts with field-level invariant comments, method signatures with explicit parameter and return types, and no function bodies except trivial zero-value constructors if absolutely needed.

The behaviour spec must include:
- At least 5 numbered behaviours in the exact format `B-TICKET-N: Given <precondition>, when <action>, then <observable outcome>.`
- Behaviour coverage for every public symbol in `contracts/ticket_contract.go`.
- Behaviour coverage for ticket pool acquire/reset/release lifecycle.
- Behaviour coverage for ingestion payload validation, deck validation, vector normalization, server-derived fields, queue acknowledgment, and ring-buffer handoff semantics.
- Explicit resolution of the `FEATURES.md` vs `MATCH_SPEC.md` trophy validation difference: `MATCH_SPEC.md` governs client intake validation.
- An Allocation Budget Table for every hot-path function in this module.
- An Edge Case Register covering race conditions, starvation/backpressure paths, numerical edge cases, malformed payloads, invalid decks, zero vectors, and pool lifecycle hazards.

Do not write implementation files under `internal/`.

## Relevant Spec Sections
- `docs/FEATURES.md` §2.1 `Ticket` — Primary Unit of Queue Entry
- `docs/FEATURES.md` §3 Module A — Ingestion Engine
- `docs/FEATURES.md` §6.1-§6.2 Deck vector normalization and cosine assumptions that ingestion must satisfy
- `docs/FEATURES.md` §10.1 Hot-Path Allocation Budget
- `docs/FEATURES.md` §10.2 `sync.Pool` Objects
- `docs/FEATURES.md` §10.3 Struct Alignment
- `docs/FEATURES.md` §11.2 Shared State Inventory
- `docs/FEATURES.md` §12.1 Error Taxonomy
- `docs/FEATURES.md` §13.3 Fuzz Targets for `FuzzParseTicket`
- `docs/FEATURES.md` §14 Delivery Sequence & Dependency Graph
- `docs/MATCH_SPEC.md` §1.1 Trophy Ladder
- `docs/MATCH_SPEC.md` §2.3 Deck Validity Rules
- `docs/MATCH_SPEC.md` §2.4 Vector Construction
- `docs/MATCH_SPEC.md` §2.5 Card Table
- `docs/MATCH_SPEC.md` §3 Player Intake Interface
- `docs/MATCH_SPEC.md` §5 Key Invariants for the Orchestrator
- `docs/AGENTS.md` Planner Agent responsibilities and exit criteria
- `docs/GIT_POLICY.md` Commit granularity and branch policy

## Inputs Available
- `docs/FEATURES.md`
- `docs/MATCH_SPEC.md`
- `docs/AGENTS.md`
- `docs/GIT_POLICY.md`

## Checker Report (if re-cycle)
Not applicable. This is the first Planner dispatch for `ticket`.
