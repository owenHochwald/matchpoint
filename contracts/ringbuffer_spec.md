# Ringbuffer Contract Specification

Module: `ringbuffer`  
Phase: Planner  
Signed-off Planner Agent: Project MatchPoint Planner  
Date: 2026-06-27

## Scope

The ringbuffer module owns the lock-free ingestion-to-match-core handoff after
ticket parsing. It accepts pool-owned `*Ticket` values from the upstream ticket
contract, publishes them to per-CPU shards selected by player identity, and lets
match-core consumers read or drain those tickets without blocking the ingestion
connection goroutine.

This contract intentionally depends only on `contracts.Ticket` vocabulary and
does not import implementation packages.

## Public Symbols

- `RingBackpressureWaitNanos`: fixed 10 microsecond write wait budget.
- `RingState`, `RingStateOpen`, `RingStateDraining`, `RingStateClosed`: shard
  lifecycle state.
- `RingWriteStatus`, `RingWriteAccepted`, `RingWriteFull`,
  `RingWriteBackpressureTimeout`, `RingWriteClosed`,
  `RingWriteDuplicatePublisher`: write outcomes.
- `RingReadStatus`, `RingReadOK`, `RingReadEmpty`, `RingReadClosed`: read and
  drain outcomes.
- `RingShardID`: per-CPU shard identifier.
- `RingSequence`: monotonically increasing slot generation used against ABA.
- `RingConfig`: immutable construction parameters.
- `RingCursor`: cache-line isolated atomic head or tail cursor.
- `RingSlot`: preallocated queue slot holding one ticket pointer.
- `RingShard`: preallocated per-CPU shard layout.
- `RingSnapshot`: allocation-free shard diagnostic snapshot.
- `RingWriteResult`: write result value.
- `RingReadResult`: single-read result value.
- `RingDrainResult`: drain result value.
- `ShardSelector`: player-to-shard selection interface.
- `TicketRingBuffer`: ingestion-to-match-core handoff interface.

## Behaviour Specification

B-RINGBUFFER-1: Given a `RingConfig` with `Shards == 0`, when a ring buffer is constructed by the implementor, then construction fails deterministically before any shard is published.

B-RINGBUFFER-2: Given a `RingConfig` with `CapacityPerShard` not a power of two, when a ring buffer is constructed by the implementor, then construction fails deterministically because `RingShard.Mask` would not encode wraparound correctly.

B-RINGBUFFER-3: Given a valid `RingConfig`, when shards are initialized, then every `RingShard` has immutable `Capacity == len(Slots)`, immutable `Mask == Capacity-1`, unique `ShardID`, `RingStateOpen`, and preallocated `RingSlot` storage.

B-RINGBUFFER-4: Given any non-zero `playerID` and a configured shard count, when `ShardSelector.ShardForPlayer(playerID)` is called, then it returns `RingShardID(playerID % Shards)` and repeated calls for the same player return the same shard.

B-RINGBUFFER-5: Given `RingBackpressureWaitNanos`, when the implementor uses the backpressure budget, then the budget is exactly `10_000` nanoseconds and no write path waits longer than that value.

B-RINGBUFFER-6: Given an open shard with at least one free slot and a non-nil ticket with non-zero `Ticket.PlayerID`, when `TicketRingBuffer.WriteTicket(ticket, nowUnixNano)` is called, then exactly one slot is reserved, the ticket pointer is published, `RingWriteResult.Status == RingWriteAccepted`, `ShardID` equals `ticket.PlayerID % Shards`, and ownership transfers from caller to ring.

B-RINGBUFFER-7: Given a successful write, when the producer publishes the slot, then the slot's `RingSlot.Sequence` generation is stored before the readable state is visible and the consumer never observes a partially populated `RingSlot.Ticket`.

B-RINGBUFFER-8: Given an open shard whose readable count equals capacity, when `WriteTicket` is called, then the write performs one bounded wait of at most `RingBackpressureWaitNanos`; if no slot becomes free, the returned status is `RingWriteBackpressureTimeout` and the caller retains ticket ownership.

B-RINGBUFFER-9: Given a full shard where a consumer frees a slot during the bounded wait, when `WriteTicket` retries before the 10 microsecond deadline, then it may return `RingWriteAccepted` and `RingWriteResult.WaitedNanos` reports a value in `[0, RingBackpressureWaitNanos]`.

B-RINGBUFFER-10: Given a shard observed full before the bounded wait begins, when `WriteTicket` returns after the retry path, then `RingWriteFull` is observable only as an intermediate or diagnostic status and final rejection uses `RingWriteBackpressureTimeout`.

B-RINGBUFFER-11: Given a player already has an undrained ticket in the selected shard, when another publisher calls `WriteTicket` with the same `Ticket.PlayerID`, then the second write returns `RingWriteDuplicatePublisher`, publishes no slot, and leaves ownership with the caller.

B-RINGBUFFER-12: Given two publishers concurrently call `WriteTicket` with the same `Ticket.PlayerID`, when they race on the duplicate-publisher table and slot reservation, then at most one call returns `RingWriteAccepted`; all losers return `RingWriteDuplicatePublisher` or a full/closed rejection without corrupting queue order.

B-RINGBUFFER-13: Given one or more producers write distinct tickets to the same open shard, when the shard consumer reads from that shard, then tickets are observed in reservation order by `RingSequence` and no ticket is skipped or duplicated.

B-RINGBUFFER-14: Given an open shard with at least one readable ticket, when `TicketRingBuffer.ReadTicket(shardID)` is called by the shard's consumer, then it returns `RingReadOK`, a non-nil `Ticket`, the consumed `RingSequence`, clears the duplicate-publisher key for that player, and transfers ticket ownership to the consumer.

B-RINGBUFFER-15: Given an open shard with no readable tickets, when `ReadTicket(shardID)` is called, then it returns `RingReadEmpty`, `Ticket == nil`, and does not mutate producer-owned cursor state.

B-RINGBUFFER-16: Given a caller-owned destination slice with capacity for `N` ticket pointers and an open shard with at least `N` readable tickets, when `TicketRingBuffer.DrainShard(shardID, dst)` is called, then it writes exactly `N` ticket pointers into `dst`, returns `RingReadOK`, advances the consumer cursor by `N`, and performs zero heap allocations.

B-RINGBUFFER-17: Given a caller-owned destination slice with capacity for `N` ticket pointers and an open shard with fewer than `N` readable tickets, when `DrainShard(shardID, dst)` is called, then it writes only the available tickets, returns `RingReadOK` if count is greater than zero or `RingReadEmpty` if count is zero, and never blocks waiting for future writes.

B-RINGBUFFER-18: Given an invalid `RingShardID` outside the configured shard range, when `ReadTicket`, `DrainShard`, `SnapshotShard`, or `CloseShard` is called, then the implementation returns a deterministic closed/empty diagnostic result without panicking.

B-RINGBUFFER-19: Given an open shard, when `TicketRingBuffer.CloseShard(shardID)` is called, then the shard transitions to `RingStateDraining`, rejects new writes with `RingWriteClosed`, and allows existing readable tickets to drain.

B-RINGBUFFER-20: Given a draining shard with all readable tickets consumed, when `ReadTicket` or `DrainShard` observes the empty state, then the shard transitions or reports `RingStateClosed` / `RingReadClosed`.

B-RINGBUFFER-21: Given `TicketRingBuffer.Close()` is called, when any shard is still readable, then all shards reject new writes immediately while consumers may continue draining already-published tickets.

B-RINGBUFFER-22: Given a closed shard with no readable tickets, when `ReadTicket` or `DrainShard` is called, then the result status is `RingReadClosed`, the count is zero, and no cursor advances.

B-RINGBUFFER-23: Given `TicketRingBuffer.SnapshotShard(shardID)` is called concurrently with writes and reads, when it loads cursor and state fields, then it returns an approximate `RingSnapshot` with `Depth` clamped to `Capacity` and performs no synchronization beyond atomic loads.

B-RINGBUFFER-24: Given a `RingCursor`, when two adjacent cursors or shard metadata are updated by different goroutines, then its padding keeps the hot atomic `Value` isolated from false sharing on a 64-byte cache-line target.

B-RINGBUFFER-25: Given a `RingSlot`, when its position wraps after many producer/consumer cycles, then the consumer uses `RingSequence` rather than slot index alone and does not confuse a newly published ticket with an earlier generation of the same slot.

B-RINGBUFFER-26: Given `RingShard.Head` and `RingShard.Tail`, when producers reserve and consumers release slots, then both cursors are monotonically increasing `uint64` counters and wraparound is treated as a theoretical exhaustion edge that must not occur under practical test durations.

B-RINGBUFFER-27: Given `RingWriteResult`, when a write is accepted, then `Status`, `ShardID`, `Sequence`, and `WaitedNanos` are fully populated without returning heap-allocated errors.

B-RINGBUFFER-28: Given `RingReadResult`, when a read is not `RingReadOK`, then `Ticket` is nil and `Sequence` is zero-value so callers cannot accidentally process stale ticket pointers.

B-RINGBUFFER-29: Given `RingDrainResult`, when drain completes, then `Count` is the number of valid entries written to the caller slice and `Remaining` is an approximate post-drain depth clamped to shard capacity.

B-RINGBUFFER-30: Given the upstream ticket checker warning that the 10 microsecond wait was previously delegated to the sink boundary, when the ringbuffer module is implemented, then tests directly exercise full-shard timeout and freed-during-wait acceptance at this module boundary.

## Allocation Budget Table

| Hot-path function | Budget (B/op) | Justification |
| --- | ---: | --- |
| `ShardSelector.ShardForPlayer` | 0 | Pure modulo arithmetic over `uint64` and configured shard count. |
| `TicketRingBuffer.WriteTicket` accepted path | 0 | Uses preallocated slots, atomics, and duplicate table; returns value result. |
| `TicketRingBuffer.WriteTicket` full/timeout path | 0 | Performs bounded spin/yield wait only; caller retains ticket ownership. |
| `TicketRingBuffer.ReadTicket` | 0 | Returns pointer from preallocated slot and value result. |
| `TicketRingBuffer.DrainShard` | 0 | Writes into caller-owned `[]*Ticket`; does not allocate destination storage. |
| `TicketRingBuffer.SnapshotShard` | 0 | Atomic loads into value result. |
| `TicketRingBuffer.CloseShard` | 0 | Atomic state transition only. |
| `TicketRingBuffer.Close` | 0 | Iterates fixed shard slice and performs atomic state transitions only. |

## Edge Case Register

| Edge | Risk | Contracted handling |
| --- | --- | --- |
| Full shard | Connection goroutine stalls or allocates while waiting. | `WriteTicket` performs one bounded wait of at most `RingBackpressureWaitNanos`, then rejects with `RingWriteBackpressureTimeout`. |
| Freed during wait | Ticket is dropped even though capacity appears before deadline. | `WriteTicket` retries during the bounded wait and may accept if a slot frees before the deadline. |
| Empty shard | Consumer spins on stale readable state. | `ReadTicket` and `DrainShard` return `RingReadEmpty` for open-empty shards and do not block. |
| Closed shard | New tickets publish after shutdown starts. | `CloseShard` and `Close` move shards to draining/closed states; writes return `RingWriteClosed`. |
| Close while readable | Tickets are leaked during shutdown. | Draining state rejects writes but permits consumers to read already-published slots. |
| Duplicate publisher | Reconnection creates two active tickets for the same player in one tick window. | Duplicate table keyed by `Ticket.PlayerID` permits at most one undrained accepted ticket per player per shard. |
| Concurrent duplicate race | Two producers pass duplicate check before either publishes. | Duplicate claim and slot reservation must be ordered atomically so at most one write accepts. |
| Producer/consumer race | Consumer observes partially initialized ticket pointer. | Slot sequence and published state require release/acquire ordering; readable state becomes visible only after ticket and player fields are stored. |
| ABA/wraparound | Slot index reuse is mistaken for an old publish. | Every slot has a monotonically increasing `RingSequence`; consumers compare sequence/generation rather than index only. |
| `uint64` cursor overflow | Head/tail arithmetic wraps after extreme uptime. | Cursors are monotonically increasing `uint64`; overflow is documented as theoretical and must not occur in benchmarks or simulations. |
| False sharing | Adjacent shard/cursor updates bounce cache lines under load. | `RingCursor` and shard padding isolate hot atomics on 64-byte cache-line targets. |
| Invalid shard ID | Caller panic in match-core drain loop. | Read/drain/snapshot/close APIs return deterministic closed/empty diagnostics without panicking. |
| Overwrite prohibition | Ingestion queue silently loses oldest ticket like telemetry ring. | Ingestion ring never overwrites readable tickets; full shards reject after the bounded wait. |
| Nil ticket | Producer publishes invalid pointer. | Implementor must reject nil tickets deterministically without publishing or panicking. |
| Zero player ID | Shard selection and duplicate table admit invalid intake data. | Implementor must reject zero-player tickets deterministically; upstream ticket validation should prevent this before handoff. |
| Destination slice too small | Drain allocates replacement storage. | `DrainShard` writes only up to `len(dst)` and never allocates. |
| Tooling warning inheritance | Staticcheck may exit zero without analyzing packages. | Ringbuffer handoff must paste command output and Checker must independently verify package matching. |

## Shared State Inventory

| Shared resource | Access mechanism | Rationale |
| --- | --- | --- |
| `RingShard.Head.Value` | `atomic.Uint64` | Exclusive consumer advances head; producers may load for capacity checks. |
| `RingShard.Tail.Value` | `atomic.Uint64` | Producers reserve slots with atomic ordering; consumer may load for empty checks. |
| `RingSlot.Sequence` | `atomic.Uint64` | Generation check prevents ABA on wraparound. |
| `RingSlot.PlayerID` | `atomic.Uint64` | Duplicate release and snapshot checks avoid data races. |
| `RingSlot.Published` | `atomic.Uint32` | Release/acquire publication state between producers and consumer. |
| `RingShard.State` | `atomic.Uint32` | Close/drain transition is observed by producers and consumers. |
| Duplicate-publisher table | Preallocated atomic player-key cells | Prevents concurrent duplicate active tickets without heap allocation. |

## Implementation Notes For Handoff

- The implementation should provide red tests for every `B-RINGBUFFER-N`
  behaviour before implementation.
- Benchmarks must include `WriteTicket`, `ReadTicket`, `DrainShard`,
  `SnapshotShard`, `CloseShard`, and `ShardForPlayer`, each with
  `b.ReportAllocs()`.
- The ringbuffer module owns direct tests for the 10 microsecond backpressure
  wait; this closes the inherited ticket checker caveat.
- The ingestion ring is not a telemetry ring. It must not overwrite old tickets
  when full.
- `DrainShard` receives caller-owned storage because match core owns tick-budget
  batching and must decide destination capacity.

## Checker WARN Annotations

Appended by Orchestrator after `reports/ringbuffer_checker_report.md` returned
`CHECKER: WARN` on 2026-06-28.

- `STATICCHECK_WARN`: `staticcheck ./...` exits `0` but prints
  `warning: "./..." matched no packages`; `go list ./...` sees the expected Go
  packages, so static analysis did not actually run under the current
  Go/staticcheck toolchain combination.
- `COVERAGE_WARN`: tests contain markers for `B-RINGBUFFER-1` through
  `B-RINGBUFFER-30`, but some multi-clause behaviours are only partially
  asserted, especially post-read duplicate-key reuse, concurrent duplicate
  queue-order preservation, and fully external construction usability.
