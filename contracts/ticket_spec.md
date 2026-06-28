# Ticket Module Planner Specification

Module: `ticket`  
Scope: `Ticket` struct, pool lifecycle, fixed-roster deck validation, queue-join intake, server-derived ticket population, acknowledgment, and ring-buffer handoff contract.

This specification resolves the trophy validation conflict in favor of `docs/MATCH_SPEC.md`: client intake trophies are valid only in `[0, 15000]`. The wider range in `docs/FEATURES.md` is not used for the ticket intake boundary.

## Public Contract Coverage

- `TicketPoolTag`: stable queue family values used by the ticket and later EOMM modules.
- `IntakeErrorCode`: stable observable intake error taxonomy.
- `QueueJoinStatus`: stable client-visible queue join status taxonomy.
- `WireFormat`: negotiated inbound frame format.
- `Ticket`: 64-byte target queue-entry layout.
- `QueueJoinPayload`: decoded client wire payload.
- `QueueJoinAck`: server response after a queue-join attempt.
- `CardDef`: fixed v1 roster entry shape.
- `DerivedSignals`: server-derived streak and risk values.
- `TicketPool`: pooled ticket acquire/reset/release lifecycle.
- `PayloadDecoder`: raw frame decoding boundary.
- `Authenticator`: server-side session validation boundary.
- `SignalStore`: server-side streak/risk lookup boundary.
- `DeckValidator`: deck validity and vector-normalization boundary.
- `RingBufferSink`: handoff boundary to the later ringbuffer module.
- `QueueEstimator`: queue wait estimate provider for acknowledgments.
- `Clock`: server-owned enqueue timestamp provider.
- `IntakeProcessor`: composed intake hot path.

## Behaviour Specification

B-TICKET-1: Given a `Ticket` value on a 64-bit Go implementation, when `unsafe.Sizeof(Ticket{})` is measured, then the size is exactly 64 bytes and field order matches `contracts/ticket_contract.go`.

B-TICKET-2: Given a `TicketPool` with warmed pooled entries, when `AcquireTicket` is called, then it returns a non-nil `*Ticket` whose fields are all zero values and which is exclusively owned by the caller until release or publication.

B-TICKET-3: Given a `Ticket` with every field set to a non-zero sentinel value, when `ResetTicket` is called, then `PlayerID`, `EnqueuedAt`, `DeckVector`, `Trophies`, `ChurnRisk`, `MonetizationP`, `ConsecLosses`, `ConsecWins`, and `PoolTag` are all reset to zero.

B-TICKET-4: Given a non-nil acquired `Ticket`, when `ReleaseTicket` is called, then the ticket is reset before being returned to the pool and may later be acquired without retaining prior player data.

B-TICKET-5: Given a nil `*Ticket`, when `ReleaseTicket` is called, then no panic occurs and no pool state visible to callers changes.

B-TICKET-6: Given raw MessagePack or JSON that cannot be decoded into `QueueJoinPayload`, when `DecodeQueueJoin` is called with the negotiated `WireFormat`, then it returns `ErrMalformedPayload` and does not mutate any server-derived ticket fields.

B-TICKET-7: Given a frame whose parsing exceeds 50 microseconds, when `ParseTicket` processes it, then the frame is rejected with `QueueStatusRejected`, `ErrParseTimeout`, no ticket is published, and any acquired ticket is released.

B-TICKET-8: Given a decoded payload with `PlayerID == 0`, when `ParseTicket` validates it, then it returns `QueueStatusRejected`, a non-OK validation error, and no ring-buffer write is attempted.

B-TICKET-9: Given a decoded payload with `SessionTokenLen == 0` or an invalid session token, when `ValidateSession` is called during `ParseTicket`, then the response is `QueueStatusRejected` with `ErrUnauthorized` and no ticket is acquired for publication.

B-TICKET-10: Given a decoded payload with trophies below `0` or above `15000`, when `ParseTicket` validates client ladder state, then it returns `QueueStatusRejected` with `ErrInvalidTrophies`; `docs/MATCH_SPEC.md` governs this bound over `docs/FEATURES.md`.

B-TICKET-11: Given a decoded payload whose `Tier` does not match the trophy range table in `MATCH_SPEC.md` section 1.1, when `ParseTicket` validates ladder state, then it returns `QueueStatusRejected` with `ErrTierMismatch`.

B-TICKET-12: Given a decoded payload with any duplicate card ID, any card ID outside `[0, 47]`, fewer or more than 8 cards, average elixir outside `[2.5, 5.0]`, or fewer than 2 distinct archetypes, when `BuildDeckVector` validates the deck, then it returns `ErrInvalidDeck`.

B-TICKET-13: Given a valid 8-card deck, when `BuildDeckVector` computes the raw archetype weights, then each primary archetype contributes `1.0`, each secondary archetype contributes `0.4`, and the output vector is L2-normalized.

B-TICKET-14: Given a deck whose raw archetype vector magnitude is below `1e-6`, when `BuildDeckVector` normalizes it, then it returns `ErrZeroVector` and does not publish a ticket.

B-TICKET-15: Given a valid payload and an unavailable `SignalStore`, when `LoadSignals` is called during `ParseTicket`, then `ConsecLosses` and `ConsecWins` may use client fallback values, while `ChurnRisk` and `MonetizationP` default to `0.1`.

B-TICKET-16: Given a valid payload and an available `SignalStore`, when `ParseTicket` populates the `Ticket`, then server-loaded `ConsecLosses`, `ConsecWins`, `ChurnRisk`, and `MonetizationP` override any client-supplied streak values.

B-TICKET-17: Given server-derived `ConsecLosses > 0` or `ConsecWins < 0`, when `ParseTicket` validates derived signals, then it rejects the payload with `QueueStatusRejected`, does not publish the ticket, and releases any acquired ticket.

B-TICKET-18: Given server-derived `ChurnRisk` or `MonetizationP` outside `[0.0, 1.0]`, when `ParseTicket` validates derived signals, then it rejects the payload with `QueueStatusRejected`, does not publish the ticket, and releases any acquired ticket.

B-TICKET-19: Given a fully valid payload, when `ParseTicket` constructs the ticket, then `EnqueuedAt` is assigned from `Clock.NowUnixNano`, `PoolTag` is set to `PoolMainstream`, and client input is never used for `ChurnRisk` or `MonetizationP`.

B-TICKET-20: Given a fully populated ticket, when `ParseTicket` publishes it, then `RingBufferSink.WriteTicket` is called exactly once for the shard selected by `PlayerID % NumShards` by the sink implementation.

B-TICKET-21: Given `RingBufferSink.WriteTicket` accepts the ticket, when `ParseTicket` returns, then the returned `QueueJoinAck` has `QueueStatusQueued`, `IntakeOK`, `ShardDepth(PlayerID)`, and `EstimateWaitMS(Trophies)`.

B-TICKET-22: Given `RingBufferSink.WriteTicket` returns `ErrRingBufferFull` after the allowed single 10 microsecond backpressure wait, when `ParseTicket` handles the failure, then it returns `QueueStatusRejected`, `ErrRingBufferFull`, includes `EstWaitMS`, and releases the ticket to the pool.

B-TICKET-23: Given a player already has an active ticket during the same queue window, when `ParseTicket` processes another valid join frame for that player, then it returns `QueueStatusAlreadyQueued`, `IntakeOK`, and does not publish a duplicate ticket.

B-TICKET-24: Given `WireFormatMessagePack`, when `DecodeQueueJoin` decodes a valid frame, then it fills only `QueueJoinPayload` fields permitted by `MATCH_SPEC.md` and ignores any attempted client-supplied churn or monetization fields.

B-TICKET-25: Given `WireFormatJSON`, when `DecodeQueueJoin` decodes a valid fallback frame, then it produces the same `QueueJoinPayload` values as the MessagePack form for equivalent input.

B-TICKET-26: Given a successful `ParseTicket` call, when ownership is inspected, then the returned `*Ticket` is not released by the ticket module after successful ring-buffer publication because ownership has moved to the ring-buffer consumer path.

## Allocation Budget Table

| Contracted function | Hot path | Max heap allocation budget | Rationale |
|---|---:|---:|---|
| `TicketPool.AcquireTicket` | Yes | `0 B/op` steady-state | `sync.Pool` warm entries; no per-acquire allocation after warm-up. |
| `TicketPool.ResetTicket` | Yes | `0 B/op` | Field zeroing only. |
| `TicketPool.ReleaseTicket` | Yes | `0 B/op` | Reset plus `sync.Pool.Put`; nil path is branch-only. |
| `PayloadDecoder.DecodeQueueJoin` | Yes | `0 B/op` for MessagePack, implementation-defined for JSON fallback but must not exceed `256 B/op` | Binary intake is the production hot path; JSON is fallback and may require decoder overhead. |
| `Authenticator.ValidateSession` | Yes | `0 B/op` | Token validation must avoid allocation on valid-token path. |
| `SignalStore.LoadSignals` | Yes | `0 B/op` | Server state lookup boundary must return value structs only. |
| `DeckValidator.BuildDeckVector` | Yes | `0 B/op` | Fixed `[8]uint8` input and `[8]float32` output pointer; stack-only math. |
| `RingBufferSink.WriteTicket` | Yes | `0 B/op` | Preallocated ring slots; ownership transfer only. |
| `RingBufferSink.ShardDepth` | Yes | `0 B/op` | Atomic/index read only. |
| `QueueEstimator.EstimateWaitMS` | Yes | `0 B/op` | Uses precomputed segment statistics. |
| `Clock.NowUnixNano` | Yes | `0 B/op` | Timestamp read only. |
| `IntakeProcessor.ParseTicket` | Yes | `0 B/op` for MessagePack success path after pool warm-up; JSON fallback must not exceed `256 B/op` | Production path decodes into stack/value fields, acquires pooled ticket, validates fixed arrays, and publishes pointer. |

## Edge Case Register

| ID | Category | Edge case | Required handling |
|---|---|---|---|
| E-TICKET-1 | Race | Ticket is released while still owned by ring buffer after successful publication. | Prohibited by ownership transfer; successful `ParseTicket` must not release the ticket. |
| E-TICKET-2 | Race | Same player reconnects and submits duplicate queue joins within one tick. | Active-ticket detection must return `QueueStatusAlreadyQueued` without duplicate ring-buffer publication. |
| E-TICKET-3 | Race | `PoolTag` is read by a later module while ticket publication is in progress. | `PoolTag` is initialized before publication; this module performs no concurrent mutation after publication. |
| E-TICKET-4 | Race | Server-derived state changes while intake is loading signals. | `LoadSignals` returns a value snapshot; ticket uses that snapshot consistently. |
| E-TICKET-5 | Starvation/backpressure | Ring-buffer shard is full. | Sink performs at most one 10 microsecond spin-wait; failure returns `ErrRingBufferFull`, releases ticket, and sends retry estimate. |
| E-TICKET-6 | Starvation/backpressure | Hot player repeatedly fills same shard because shard is `PlayerID % NumShards`. | Duplicate-ticket detection and backpressure counter semantics prevent unbounded per-connection blocking. |
| E-TICKET-7 | Numerical | Trophy validation conflict between docs. | Intake uses `[0, 15000]` from `MATCH_SPEC.md`, not `[-10000, 100000]` from `FEATURES.md`. |
| E-TICKET-8 | Numerical | Deck raw magnitude is zero or below `1e-6`. | Return `ErrZeroVector`; do not divide or publish. |
| E-TICKET-9 | Numerical | Floating point normalization produces magnitude outside tolerance. | Tests must assert normalized magnitude within a small epsilon of `1.0` for valid decks. |
| E-TICKET-10 | Numerical | Average elixir boundary values are exactly `2.5` or `5.0`. | Boundaries are inclusive and valid. |
| E-TICKET-11 | Malformed payload | Unknown `WireFormat` or syntactically invalid frame. | Return `ErrMalformedPayload`; no ticket acquisition for publication. |
| E-TICKET-12 | Invalid deck | Duplicate card IDs, out-of-range card IDs, invalid deck length, single archetype, or average elixir outside bounds. | Return `ErrInvalidDeck`; release acquired ticket if acquisition already occurred. |
| E-TICKET-13 | Security | Client sends `churn_risk` or `monetization_p` fields in JSON fallback. | Ignore those fields; server-derived values only. |
| E-TICKET-14 | Pool lifecycle | `ResetTicket` forgets a field and leaks prior player state. | Sentinel reset test must cover every `Ticket` field. |
| E-TICKET-15 | Pool lifecycle | `AcquireTicket` returns nil under pool pressure. | Prohibited; implementation must allocate via pool `New` if needed. |
| E-TICKET-16 | Parse budget | Large or adversarial payload consumes more than 50 microseconds. | Reject with `ErrParseTimeout`; increment drop metric in implementation. |
| E-TICKET-17 | Signal validity | Server store returns positive losses, negative wins, or risk values outside `[0, 1]`. | Reject and release ticket; never publish invalid invariants. |
| E-TICKET-18 | Acknowledgment | Rejected join lacks retry wait for ring-buffer full. | `ErrRingBufferFull` ack must include non-zero or estimator-derived `EstWaitMS`. |
| E-TICKET-19 | Ownership | `DecodeQueueJoin` or validation stores references to `raw`. | Prohibited on hot path; decoded payload must be value-owned after return. |
| E-TICKET-20 | Fixed roster | Runtime attempts to modify the 48-card roster. | Prohibited for v1; card table is compile-time constant in implementation. |

## Planner Signature

Signed-off Planner Agent: Project MatchPoint Planner  
Date: 2026-06-27

## Checker WARN Annotations

Appended by Orchestrator after `reports/ticket_checker_report.md` returned `CHECKER: WARN` on 2026-06-27.

- `STATICCHECK_WARN`: `staticcheck ./...` exits `0` but prints `warning: "./..." matched no packages`; explicit package paths also match no packages while `go list ./...` sees `matchpoint/contracts` and `matchpoint/internal/ticket`. This is treated as a tooling/package-resolution warning, not evidence that static analysis actually analyzed packages.
- `COVERAGE_WARN`: behaviour coverage exists for `B-TICKET-1` through `B-TICKET-26`, but some behaviours are only partially/directly covered: malformed JSON decode, fewer/more-than-8-card decode paths, exact primary/secondary weight assertions, and the 10us backpressure-wait responsibility delegated to the sink boundary.
