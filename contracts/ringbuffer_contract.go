// Package contracts defines the Planner-owned public contract for MatchPoint
// modules. This file is intentionally declarative: no implementation logic
// belongs here.
package contracts

import "sync/atomic"

// RingBackpressureWaitNanos is the maximum bounded wait on a full shard.
const RingBackpressureWaitNanos int64 = 10_000

// RingState identifies whether a shard accepts writes or is draining/closed.
type RingState uint8

const (
	// RingStateOpen accepts writes and reads.
	RingStateOpen RingState = 0
	// RingStateDraining rejects new writes while allowing already-published tickets to be read.
	RingStateDraining RingState = 1
	// RingStateClosed rejects writes and reports closed once all published tickets are drained.
	RingStateClosed RingState = 2
)

// RingWriteStatus is the observable result of a shard write attempt.
type RingWriteStatus uint8

const (
	// RingWriteAccepted means the ticket was published to its selected shard.
	RingWriteAccepted RingWriteStatus = 0
	// RingWriteFull means the shard was observed full before the bounded backpressure wait.
	RingWriteFull RingWriteStatus = 1
	// RingWriteBackpressureTimeout means the shard remained full after the 10 microsecond wait.
	RingWriteBackpressureTimeout RingWriteStatus = 2
	// RingWriteClosed means the shard was draining or closed and did not accept the ticket.
	RingWriteClosed RingWriteStatus = 3
	// RingWriteDuplicatePublisher means the player already has an undrained ticket in the selected shard.
	RingWriteDuplicatePublisher RingWriteStatus = 4
)

// RingReadStatus is the observable result of a read or drain attempt.
type RingReadStatus uint8

const (
	// RingReadOK means at least one ticket was read.
	RingReadOK RingReadStatus = 0
	// RingReadEmpty means the shard is open but has no readable tickets.
	RingReadEmpty RingReadStatus = 1
	// RingReadClosed means the shard is closed and no readable tickets remain.
	RingReadClosed RingReadStatus = 2
)

// RingShardID identifies one per-CPU ingestion ring shard.
type RingShardID uint16

// RingSequence is a monotonically increasing slot generation used to prevent ABA confusion.
type RingSequence uint64

// RingConfig defines immutable ring-buffer construction parameters.
type RingConfig struct {
	// Shards is the number of independent per-CPU shards and must be greater than zero.
	Shards uint16
	// CapacityPerShard is the fixed slot count per shard and must be a power of two.
	CapacityPerShard uint32
	// DuplicateWindowPerShard is the fixed player-key table size used to reject duplicate publishers.
	DuplicateWindowPerShard uint32
}

// RingCursor isolates a hot atomic cursor on its own cache line.
type RingCursor struct {
	// Value is the monotonically increasing head or tail position.
	Value atomic.Uint64
	// Padding prevents false sharing with adjacent cursor or shard metadata.
	Padding [56]byte
}

// RingSlot is one preallocated ticket slot in a shard.
type RingSlot struct {
	// Sequence is the slot generation; producers and consumers compare it to head/tail to avoid ABA.
	Sequence atomic.Uint64
	// PlayerID mirrors Ticket.PlayerID for duplicate detection and must be zero when the slot is free.
	PlayerID atomic.Uint64
	// Ticket is owned by the ring from successful write until read/drain returns it.
	Ticket *Ticket
	// Published is zero for free, one for readable, and two for consumer-owned during drain.
	Published atomic.Uint32
	// Padding reserves space so pointer-ticket slots are cache-line friendly on 64-bit platforms.
	Padding [36]byte
}

// RingShard is the per-CPU write/read boundary between ingestion and match core.
type RingShard struct {
	// Head is advanced only by the shard's match-core consumer after acquiring a published slot.
	Head RingCursor
	// Tail is advanced by producers when reserving a slot for publication.
	Tail RingCursor
	// Slots is the fixed preallocated circular slot array; it must never resize after construction.
	Slots []RingSlot
	// State is read by producers before reservation and by consumers during close/drain.
	State atomic.Uint32
	// Capacity is len(Slots) and remains immutable after construction.
	Capacity uint64
	// Mask is Capacity-1 and is valid only when Capacity is a power of two.
	Mask uint64
	// ShardID is the stable index derived from player identity.
	ShardID RingShardID
	// Padding prevents shard metadata from sharing a cache line with unrelated shards.
	Padding [78]byte
}

// RingSnapshot is an allocation-free diagnostic view of one shard.
type RingSnapshot struct {
	// ShardID is the observed shard.
	ShardID RingShardID
	// State is the observed open/draining/closed state.
	State RingState
	// Head is the observed consumer cursor.
	Head uint64
	// Tail is the observed producer cursor.
	Tail uint64
	// Capacity is the fixed shard capacity.
	Capacity uint64
	// Depth is an approximate tail-head queue depth clamped to Capacity.
	Depth uint64
}

// RingWriteResult describes one write attempt without allocating errors.
type RingWriteResult struct {
	// Status is the write outcome.
	Status RingWriteStatus
	// ShardID is the shard selected from ticket.PlayerID.
	ShardID RingShardID
	// Sequence is the reserved slot generation when Status is RingWriteAccepted.
	Sequence RingSequence
	// WaitedNanos is the bounded backpressure duration and must be <= RingBackpressureWaitNanos.
	WaitedNanos int64
}

// RingReadResult describes one single-ticket read attempt.
type RingReadResult struct {
	// Status is the read outcome.
	Status RingReadStatus
	// ShardID is the shard read by the consumer.
	ShardID RingShardID
	// Ticket is non-nil only when Status is RingReadOK.
	Ticket *Ticket
	// Sequence is the consumed slot generation when Status is RingReadOK.
	Sequence RingSequence
}

// RingDrainResult describes a bounded shard drain into caller-owned storage.
type RingDrainResult struct {
	// Status is RingReadOK when Count is greater than zero, RingReadEmpty for an open empty shard, or RingReadClosed.
	Status RingReadStatus
	// ShardID is the shard drained by the consumer.
	ShardID RingShardID
	// Count is the number of tickets written into the caller-provided destination slice.
	Count uint32
	// Remaining is an approximate post-drain depth clamped to shard capacity.
	Remaining uint64
}

// ShardSelector maps player identity to a stable per-CPU shard.
type ShardSelector interface {
	// ShardForPlayer returns playerID modulo the configured shard count.
	ShardForPlayer(playerID uint64) RingShardID
}

// TicketRingBuffer is the ingestion-to-match-core handoff contract.
type TicketRingBuffer interface {
	// WriteTicket publishes ticket to the shard selected by ticket.PlayerID with one bounded 10us wait on full.
	WriteTicket(ticket *Ticket, nowUnixNano int64) RingWriteResult
	// ReadTicket returns one ticket from shardID or a non-OK status without blocking.
	ReadTicket(shardID RingShardID) RingReadResult
	// DrainShard writes up to len(dst) tickets from shardID into dst without allocating or blocking.
	DrainShard(shardID RingShardID, dst []*Ticket) RingDrainResult
	// SnapshotShard returns an approximate allocation-free view of shard depth and cursors.
	SnapshotShard(shardID RingShardID) RingSnapshot
	// CloseShard transitions one shard to draining and eventually closed after all readable slots are consumed.
	CloseShard(shardID RingShardID) RingState
	// Close transitions every shard to draining; consumers complete closure by draining readable slots.
	Close() RingState
}
