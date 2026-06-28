# Ticket Module

This package implements the Module 1 ticket intake contract.

## Invariants

- `Ticket` is 64 bytes on 64-bit Go. Field order mirrors `contracts/ticket_contract.go` and is tested with `unsafe.Offsetof`.
- Tickets are reset before acquire/release reuse. Successful publication transfers ownership to the ring-buffer consumer, so `ParseTicket` does not release accepted tickets.
- Client trophies are validated against `MATCH_SPEC.md`: `[0, 15000]`.
- Client risk fields are ignored by decoders. `ChurnRisk` and `MonetizationP` only come from `SignalStore`, defaulting to `0.1` when that store is unavailable.
- Deck vectors are built from the fixed 48-card roster, primary weight `1.0`, secondary weight `0.4`, then L2-normalized.

## Hot Path Notes

- The production binary decoder is hand-written for the required MessagePack subset to avoid reflection and heap allocation.
- JSON fallback is also hand-scanned to stay below the fallback budget and avoid `encoding/json` reflection allocations.
- The concrete `intakeProcessor` owns scratch payload/vector storage so pointer-based contract methods do not force per-call heap escapes. The intended ownership model is one processor per connection goroutine.
- Duplicate active-ticket detection uses a fixed-size atomic open-addressed set. This prevents duplicate publication without a map allocation in the intake path.
- `sync.Pool` is used for the default ticket pool. Benchmarks model steady state by warming/returning tickets before measurement.
