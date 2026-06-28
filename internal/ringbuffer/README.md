# Ringbuffer Module

The ringbuffer module implements the pre-contracted ingestion handoff using
per-shard fixed-capacity rings. Shards are selected by `PlayerID % Shards`, so a
player's reconnect attempts target one duplicate window and one queue.

## Invariants

- `Head` and `Tail` are monotonic `atomic.Uint64` cursors. Producers reserve by
  CAS on `Tail`; consumers advance `Head` after acquiring a published slot.
- Every slot starts with `Sequence == index`. A producer stores
  `Sequence == tail + 1` before setting `Published == 1`; a consumer releases
  the slot by storing `Sequence == oldHead + Capacity`.
- Full shards are never overwritten. `WriteTicket` performs one bounded retry
  window and returns `RingWriteBackpressureTimeout` if no slot is freed.
- Duplicate publishers are rejected by a preallocated per-shard player table.
  An atomic spin gate guards exact table updates; this keeps the hot path
  allocation-free while avoiding collision-driven false duplicate accepts.
- Close transitions are write-rejecting but drain-preserving. `CloseShard` and
  `Close` move open shards to draining; consumers report closed after the
  readable range is empty.

## Cache-Line Notes

The contracted `RingCursor` is exactly 64 bytes on the target Go/arm64 layout.
`RingSlot` also fits one 64-byte line under the current contract. The concrete
shard wrapper keeps duplicate tracking outside the contracted shard layout so
cursor and slot storage remain stable after construction.

## Allocation Reasoning

Construction allocates shard, slot, and duplicate-table storage once. Steady
state `WriteTicket`, `ReadTicket`, `DrainShard`, `SnapshotShard`, `CloseShard`,
`Close`, and `ShardForPlayer` only touch preallocated arrays and return value
results. Benchmarks call `b.ReportAllocs()` for each contracted hot path.
