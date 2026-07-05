// Package contracts defines the Planner-owned public contract for MatchPoint
// modules. This file is intentionally declarative: no implementation logic
// belongs here.
package contracts

import "context"

// RedisCommandTimeoutNanos is the per-command latency budget for Redis calls.
const RedisCommandTimeoutNanos int64 = 5_000_000

// RedisScoreTrophyScale is the trophy multiplier and timestamp tie-breaker modulus.
const RedisScoreTrophyScale int64 = 1_000_000

// RedisCandidateLimit is the ZRANGEBYSCORE LIMIT count used by match ticks.
const RedisCandidateLimit int64 = 8

// RedisMatchTTLSeconds is the TTL for mq:match:<id> hashes.
const RedisMatchTTLSeconds int64 = 3600

// RedisQueueSegmentCount is the number of mainstream trophy ZSET queues.
const RedisQueueSegmentCount uint8 = 5

const (
	// RedisSegment0Key stores players with trophies in [0, 1000].
	RedisSegment0Key = "mq:seg:0"
	// RedisSegment1Key stores players with trophies in [1001, 3000].
	RedisSegment1Key = "mq:seg:1"
	// RedisSegment2Key stores players with trophies in [3001, 6000].
	RedisSegment2Key = "mq:seg:2"
	// RedisSegment3Key stores players with trophies in [6001, 10000].
	RedisSegment3Key = "mq:seg:3"
	// RedisSegment4Key stores players with trophies in [10001, +inf).
	RedisSegment4Key = "mq:seg:4"
	// RedisLosersKey stores loser-pool players matched only against the same pool.
	RedisLosersKey = "mq:losers"
	// RedisRetentionKey stores high-churn retention-pool players.
	RedisRetentionKey = "mq:retention"
	// RedisMonetizeKey stores monetization-trigger players.
	RedisMonetizeKey = "mq:monetize"
	// RedisLocksKey stores active match locks reserved for Redis-side guards.
	RedisLocksKey = "mq:locks"
	// RedisPlayerKeyPrefix prefixes mq:player:<id> hashes.
	RedisPlayerKeyPrefix = "mq:player:"
	// RedisMatchKeyPrefix prefixes mq:match:<id> hashes.
	RedisMatchKeyPrefix = "mq:match:"
)

// RedisAssignMatchLua is the canonical Lua source loaded with SCRIPT LOAD.
const RedisAssignMatchLua = `
local a = redis.call('ZSCORE', KEYS[1], ARGV[1])
local b = redis.call('ZSCORE', KEYS[2], ARGV[2])
if a == false or b == false then return 0 end
redis.call('ZREM', KEYS[1], ARGV[1])
redis.call('ZREM', KEYS[2], ARGV[2])
redis.call('HSET', 'mq:match:' .. ARGV[3], 'playerA', ARGV[1], 'playerB', ARGV[2])
redis.call('EXPIRE', 'mq:match:' .. ARGV[3], 3600)
return 1
`

// RedisQueuePool identifies a Redis ZSET queue family.
type RedisQueuePool uint8

const (
	// RedisPoolSegment0 maps to mq:seg:0 for trophies [0, 1000].
	RedisPoolSegment0 RedisQueuePool = 0
	// RedisPoolSegment1 maps to mq:seg:1 for trophies [1001, 3000].
	RedisPoolSegment1 RedisQueuePool = 1
	// RedisPoolSegment2 maps to mq:seg:2 for trophies [3001, 6000].
	RedisPoolSegment2 RedisQueuePool = 2
	// RedisPoolSegment3 maps to mq:seg:3 for trophies [6001, 10000].
	RedisPoolSegment3 RedisQueuePool = 3
	// RedisPoolSegment4 maps to mq:seg:4 for trophies [10001, +inf).
	RedisPoolSegment4 RedisQueuePool = 4
	// RedisPoolLosers maps to mq:losers.
	RedisPoolLosers RedisQueuePool = 5
	// RedisPoolRetention maps to mq:retention.
	RedisPoolRetention RedisQueuePool = 6
	// RedisPoolMonetize maps to mq:monetize.
	RedisPoolMonetize RedisQueuePool = 7
)

// RedisQueueStatus is the stable non-allocating status taxonomy for Redis queue operations.
type RedisQueueStatus uint8

const (
	// RedisStatusOK means the operation completed successfully.
	RedisStatusOK RedisQueueStatus = 0
	// RedisStatusEmpty means the queue or query contained no matching members.
	RedisStatusEmpty RedisQueueStatus = 1
	// RedisStatusTimeout means a Redis command exceeded RedisCommandTimeoutNanos.
	RedisStatusTimeout RedisQueueStatus = 2
	// RedisStatusUnavailable means Redis was unreachable or returned a connection-level failure.
	RedisStatusUnavailable RedisQueueStatus = 3
	// RedisStatusScriptNotLoaded means EVALSHA was requested before a SHA was cached.
	RedisStatusScriptNotLoaded RedisQueueStatus = 4
	// RedisStatusNoScript means Redis returned NOSCRIPT and the caller must reload scripts.
	RedisStatusNoScript RedisQueueStatus = 5
	// RedisStatusDualBooking means the Lua script returned 0 because one player was already removed.
	RedisStatusDualBooking RedisQueueStatus = 6
	// RedisStatusInvalidSegment means the trophy or pool mapping has no legal Redis key.
	RedisStatusInvalidSegment RedisQueueStatus = 7
	// RedisStatusInvalidScore means score encoding would be lossy or outside the contracted range.
	RedisStatusInvalidScore RedisQueueStatus = 8
	// RedisStatusCanceled means the caller's context was canceled before the Redis operation completed.
	RedisStatusCanceled RedisQueueStatus = 9
	// RedisStatusPartial means a Redis pipeline had at least one failed command and one completed command.
	RedisStatusPartial RedisQueueStatus = 10
)

// RedisScriptKind identifies Redis-side script contracts.
type RedisScriptKind uint8

const (
	// RedisScriptAssignMatch identifies RedisAssignMatchLua.
	RedisScriptAssignMatch RedisScriptKind = 0
)

// RedisQueueConfig defines immutable Redis queue construction settings.
type RedisQueueConfig struct {
	// CommandTimeoutNanos must be RedisCommandTimeoutNanos unless tests explicitly lower it.
	CommandTimeoutNanos int64
	// PoolSize must be runtime.NumCPU()*4 in production.
	PoolSize uint16
	// CandidateLimit must be RedisCandidateLimit for match-core tick queries.
	CandidateLimit int64
	// MatchTTLSeconds must be RedisMatchTTLSeconds for mq:match:<id> hashes.
	MatchTTLSeconds int64
}

// RedisSegmentRange describes one key-schema trophy boundary.
type RedisSegmentRange struct {
	// Pool is one of RedisPoolSegment0 through RedisPoolSegment4.
	Pool RedisQueuePool
	// Key is the exact ZSET key for the segment.
	Key string
	// MinTrophies is inclusive.
	MinTrophies int32
	// MaxTrophies is inclusive; -1 means unbounded above.
	MaxTrophies int32
}

// RedisScriptSHA stores a fixed-size hexadecimal SHA1 returned by SCRIPT LOAD.
type RedisScriptSHA struct {
	// Kind identifies the Lua source this SHA belongs to.
	Kind RedisScriptKind
	// Hex contains the 40 ASCII hex bytes of the SHA1.
	Hex [40]byte
	// Len must be 40 when the SHA is loaded and 0 when unset.
	Len uint8
}

// RedisMember is the allocation-free player member representation for ZSETs and Lua ARGVs.
type RedisMember struct {
	// PlayerID is the numeric identity represented by Bytes.
	PlayerID uint64
	// Bytes contains the decimal player ID without allocation.
	Bytes [20]byte
	// Len is the valid prefix length in Bytes and must be in [1, 20].
	Len uint8
}

// RedisScore is the composite ZSET score Trophies*1e6+(EnqueuedAtMicros%1e6).
type RedisScore struct {
	// Value is the exact float64 submitted to Redis ZADD/ZRANGEBYSCORE.
	Value float64
	// Trophies is the integer trophy component.
	Trophies int32
	// EnqueuedAtMicros is EnqueuedAt truncated from nanoseconds to microseconds and bounded to the trophy bucket.
	EnqueuedAtMicros int64
}

// RedisScoreRange is the score window used for one ZRANGEBYSCORE query.
type RedisScoreRange struct {
	// Pool is the source ZSET to query.
	Pool RedisQueuePool
	// Min is inclusive score lower bound.
	Min float64
	// Max is inclusive score upper bound.
	Max float64
	// Limit is the maximum members returned and must be RedisCandidateLimit for match ticks.
	Limit int64
}

// RedisQueueEntry is one player ticket persisted in Redis.
type RedisQueueEntry struct {
	// Ticket is copied from the ringbuffer drain output and must not be mutated after enqueue.
	Ticket Ticket
	// Member is the ZSET member string for Ticket.PlayerID.
	Member RedisMember
	// Score is the encoded ZSET priority score.
	Score RedisScore
	// Pool is the current queue family containing this entry.
	Pool RedisQueuePool
}

// RedisCandidate is one candidate returned from a ZRANGEBYSCORE query.
type RedisCandidate struct {
	// Member identifies the candidate player.
	Member RedisMember
	// Score is the Redis ZSET score returned with the member.
	Score RedisScore
	// Pool is the ZSET key family read by the query.
	Pool RedisQueuePool
}

// RedisQueryBatch is one tick's pipelined candidate query boundary.
type RedisQueryBatch struct {
	// Ranges is caller-owned storage for ZRANGEBYSCORE ranges and must not be retained.
	Ranges []RedisScoreRange
	// Results is caller-owned storage for per-range candidates and must not be retained.
	Results [][]RedisCandidate
	// Count is the number of valid Ranges entries to execute.
	Count uint16
}

// RedisMoveRequest describes idempotent movement between segment and special-pool ZSETs.
type RedisMoveRequest struct {
	// Member identifies the player being moved.
	Member RedisMember
	// From is the currently expected source pool.
	From RedisQueuePool
	// To is the destination pool.
	To RedisQueuePool
	// Score is preserved exactly across the move.
	Score RedisScore
}

// RedisAssignRequest describes one atomic Lua match assignment.
type RedisAssignRequest struct {
	// SourceA is the ZSET key family containing PlayerA.
	SourceA RedisQueuePool
	// SourceB is the ZSET key family containing PlayerB and may equal SourceA.
	SourceB RedisQueuePool
	// PlayerA is ARGV[1] for RedisAssignMatchLua.
	PlayerA RedisMember
	// PlayerB is ARGV[2] for RedisAssignMatchLua.
	PlayerB RedisMember
	// MatchID is ARGV[3] and forms mq:match:<MatchID>.
	MatchID uint64
}

// RedisOperationResult is a non-allocating status record for queue mutations.
type RedisOperationResult struct {
	// Status is the deterministic operation outcome.
	Status RedisQueueStatus
	// Pool is the primary ZSET affected by the operation.
	Pool RedisQueuePool
	// Count is the number of Redis members affected or returned.
	Count uint16
	// ElapsedNanos is the measured Redis round-trip duration.
	ElapsedNanos int64
}

// RedisAssignResult is the observable result of RedisAssignMatchLua.
type RedisAssignResult struct {
	// Status is RedisStatusOK on Lua return 1 or RedisStatusDualBooking on Lua return 0.
	Status RedisQueueStatus
	// MatchID is the committed or attempted match ID.
	MatchID uint64
	// PlayerA is the first attempted player.
	PlayerA RedisMember
	// PlayerB is the second attempted player.
	PlayerB RedisMember
	// ElapsedNanos is the measured EVALSHA round-trip duration.
	ElapsedNanos int64
}

// RedisQueueMetrics is a caller-owned counter sink used without formatting errors on hot paths.
type RedisQueueMetrics interface {
	// IncLatency increments the Redis latency counter for timeout or over-budget commands.
	IncLatency(status RedisQueueStatus, elapsedNanos int64)
	// IncScriptReload increments when NOSCRIPT forces a SCRIPT LOAD retry path.
	IncScriptReload()
	// IncDualBooking increments when RedisAssignMatchLua returns 0.
	IncDualBooking()
	// IncPipelinePartial increments when a batched tick query has mixed success and failure.
	IncPipelinePartial()
}

// RedisQueueKeyer maps trophies and pools to Redis key schema without allocation.
type RedisQueueKeyer interface {
	// SegmentForTrophies returns the mainstream segment for trophies.
	SegmentForTrophies(trophies int32) RedisQueuePool
	// KeyForPool returns the exact Redis key for a segment or special pool.
	KeyForPool(pool RedisQueuePool) (string, RedisQueueStatus)
	// SegmentRange returns the inclusive trophy boundaries for a mainstream segment.
	SegmentRange(pool RedisQueuePool) (RedisSegmentRange, RedisQueueStatus)
}

// RedisScoreCodec encodes queue scores and member strings.
type RedisScoreCodec interface {
	// EncodeMember writes playerID as the Redis ZSET member string into out.
	EncodeMember(playerID uint64, out *RedisMember) RedisQueueStatus
	// EncodeScore computes Trophies*1e6+(truncated Unix microseconds % 1e6) into out.
	EncodeScore(trophies int32, enqueuedAtUnixNano int64, out *RedisScore) RedisQueueStatus
	// ScoreRange computes the inclusive Redis score bounds for a tolerance query.
	ScoreRange(trophies int32, enqueuedAtUnixNano int64, toleranceTrophies int32, pool RedisQueuePool, out *RedisScoreRange) RedisQueueStatus
}

// RedisScriptCache owns startup SCRIPT LOAD and EVALSHA SHA state.
type RedisScriptCache interface {
	// LoadScripts loads every required Lua source and caches each SHA before tick processing starts.
	LoadScripts(ctx context.Context) RedisOperationResult
	// ScriptSHA copies the cached SHA for kind into out.
	ScriptSHA(kind RedisScriptKind, out *RedisScriptSHA) RedisQueueStatus
	// MarkNoScript clears the cached SHA for kind after Redis reports NOSCRIPT.
	MarkNoScript(kind RedisScriptKind) RedisQueueStatus
}

// RedisQueueStore is the Redis ZSET and Lua boundary consumed by matchcore and EOMM.
type RedisQueueStore interface {
	// Enqueue inserts entry into its current pool with ZADD and updates mq:player:<id> metadata.
	Enqueue(ctx context.Context, entry *RedisQueueEntry) RedisOperationResult
	// Remove deletes member from pool without touching match records.
	Remove(ctx context.Context, pool RedisQueuePool, member RedisMember) RedisOperationResult
	// FetchCandidates executes one ZRANGEBYSCORE query into caller-owned dst.
	FetchCandidates(ctx context.Context, query RedisScoreRange, dst []RedisCandidate) RedisOperationResult
	// FetchCandidateBatch pipelines one tick's ZRANGEBYSCORE queries and writes into caller-owned result storage.
	FetchCandidateBatch(ctx context.Context, batch *RedisQueryBatch) RedisOperationResult
	// MovePool moves one member from source to destination pool while preserving the exact score.
	MovePool(ctx context.Context, move RedisMoveRequest) RedisOperationResult
	// AssignMatch atomically removes two players and writes mq:match:<id> via EVALSHA.
	AssignMatch(ctx context.Context, req RedisAssignRequest) RedisAssignResult
}
