// Package redisqueue implements the MatchPoint Redis ZSET queue boundary.
package redisqueue

import (
	"context"
	"errors"
	"math"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"matchpoint/contracts"
)

const (
	RedisCommandTimeoutNanos = contracts.RedisCommandTimeoutNanos
	RedisScoreTrophyScale    = contracts.RedisScoreTrophyScale
	RedisCandidateLimit      = contracts.RedisCandidateLimit
	RedisMatchTTLSeconds     = contracts.RedisMatchTTLSeconds
	RedisQueueSegmentCount   = contracts.RedisQueueSegmentCount
)

const (
	RedisSegment0Key      = contracts.RedisSegment0Key
	RedisSegment1Key      = contracts.RedisSegment1Key
	RedisSegment2Key      = contracts.RedisSegment2Key
	RedisSegment3Key      = contracts.RedisSegment3Key
	RedisSegment4Key      = contracts.RedisSegment4Key
	RedisLosersKey        = contracts.RedisLosersKey
	RedisRetentionKey     = contracts.RedisRetentionKey
	RedisMonetizeKey      = contracts.RedisMonetizeKey
	RedisLocksKey         = contracts.RedisLocksKey
	RedisPlayerKeyPrefix  = contracts.RedisPlayerKeyPrefix
	RedisMatchKeyPrefix   = contracts.RedisMatchKeyPrefix
	RedisAssignMatchLua   = contracts.RedisAssignMatchLua
	exactIntegerFloat64   = int64(1 << 53)
	invalidRedisQueuePool = contracts.RedisQueuePool(math.MaxUint8)
)

const (
	RedisPoolSegment0  = contracts.RedisPoolSegment0
	RedisPoolSegment1  = contracts.RedisPoolSegment1
	RedisPoolSegment2  = contracts.RedisPoolSegment2
	RedisPoolSegment3  = contracts.RedisPoolSegment3
	RedisPoolSegment4  = contracts.RedisPoolSegment4
	RedisPoolLosers    = contracts.RedisPoolLosers
	RedisPoolRetention = contracts.RedisPoolRetention
	RedisPoolMonetize  = contracts.RedisPoolMonetize
)

const (
	RedisStatusOK              = contracts.RedisStatusOK
	RedisStatusEmpty           = contracts.RedisStatusEmpty
	RedisStatusTimeout         = contracts.RedisStatusTimeout
	RedisStatusUnavailable     = contracts.RedisStatusUnavailable
	RedisStatusScriptNotLoaded = contracts.RedisStatusScriptNotLoaded
	RedisStatusNoScript        = contracts.RedisStatusNoScript
	RedisStatusDualBooking     = contracts.RedisStatusDualBooking
	RedisStatusInvalidSegment  = contracts.RedisStatusInvalidSegment
	RedisStatusInvalidScore    = contracts.RedisStatusInvalidScore
	RedisStatusCanceled        = contracts.RedisStatusCanceled
	RedisStatusPartial         = contracts.RedisStatusPartial
)

const (
	RedisScriptAssignMatch = contracts.RedisScriptAssignMatch
)

type RedisQueuePool = contracts.RedisQueuePool
type RedisQueueStatus = contracts.RedisQueueStatus
type RedisScriptKind = contracts.RedisScriptKind
type RedisQueueConfig = contracts.RedisQueueConfig
type RedisSegmentRange = contracts.RedisSegmentRange
type RedisScriptSHA = contracts.RedisScriptSHA
type RedisMember = contracts.RedisMember
type RedisScore = contracts.RedisScore
type RedisScoreRange = contracts.RedisScoreRange
type RedisQueueEntry = contracts.RedisQueueEntry
type RedisCandidate = contracts.RedisCandidate
type RedisQueryBatch = contracts.RedisQueryBatch
type RedisMoveRequest = contracts.RedisMoveRequest
type RedisAssignRequest = contracts.RedisAssignRequest
type RedisOperationResult = contracts.RedisOperationResult
type RedisAssignResult = contracts.RedisAssignResult
type RedisQueueMetrics = contracts.RedisQueueMetrics
type RedisQueueKeyer = contracts.RedisQueueKeyer
type RedisScoreCodec = contracts.RedisScoreCodec
type RedisScriptCache = contracts.RedisScriptCache
type RedisQueueStore = contracts.RedisQueueStore

var segmentRanges = [contracts.RedisQueueSegmentCount]contracts.RedisSegmentRange{
	{Pool: contracts.RedisPoolSegment0, Key: contracts.RedisSegment0Key, MinTrophies: 0, MaxTrophies: 1000},
	{Pool: contracts.RedisPoolSegment1, Key: contracts.RedisSegment1Key, MinTrophies: 1001, MaxTrophies: 3000},
	{Pool: contracts.RedisPoolSegment2, Key: contracts.RedisSegment2Key, MinTrophies: 3001, MaxTrophies: 6000},
	{Pool: contracts.RedisPoolSegment3, Key: contracts.RedisSegment3Key, MinTrophies: 6001, MaxTrophies: 10000},
	{Pool: contracts.RedisPoolSegment4, Key: contracts.RedisSegment4Key, MinTrophies: 10001, MaxTrophies: -1},
}

type keyer struct{}

type scoreCodec struct {
	keyer keyer
}

type queueMetrics struct {
	latencyCount    atomic.Int64
	latencyNanos    atomic.Int64
	scriptReloads   atomic.Int64
	dualBookings    atomic.Int64
	pipelinePartial atomic.Int64
}

type scriptCache struct {
	mu      sync.RWMutex
	loader  redisScriptLoader
	metrics contracts.RedisQueueMetrics
	shas    [1]contracts.RedisScriptSHA
}

type redisStore struct {
	adapter         redisAdapter
	keyer           keyer
	codec           scoreCodec
	cache           *scriptCache
	metrics         contracts.RedisQueueMetrics
	timeout         time.Duration
	limit           int64
	ttl             int64
	deadlineContext bool
}

type redisAdapter interface {
	zadd(ctx context.Context, key string, score float64, member string, entry *contracts.RedisQueueEntry) redisAdapterResult
	zrem(ctx context.Context, key string, member string) (uint16, redisAdapterResult)
	zrangeByScore(ctx context.Context, key string, min float64, max float64, limit int64, dst []contracts.RedisCandidate, pool contracts.RedisQueuePool) (uint16, redisAdapterResult)
	zrangeByScoreBatch(ctx context.Context, ranges []contracts.RedisScoreRange, results [][]contracts.RedisCandidate) (uint16, redisAdapterResult)
	move(ctx context.Context, fromKey string, toKey string, member string, score float64) (uint16, redisAdapterResult)
	evalAssign(ctx context.Context, sha string, sourceA string, sourceB string, playerA string, playerB string, matchID string, ttlSeconds int64) (int64, redisAdapterResult)
}

type redisScriptLoader interface {
	scriptLoad(ctx context.Context, script string) (contracts.RedisScriptSHA, redisAdapterResult)
}

type redisAdapterResult struct {
	status contracts.RedisQueueStatus
}

type goRedisAdapter struct {
	client redis.UniversalClient
}

func productionConfig() contracts.RedisQueueConfig {
	return contracts.RedisQueueConfig{
		CommandTimeoutNanos: contracts.RedisCommandTimeoutNanos,
		PoolSize:            uint16(runtime.NumCPU() * 4),
		CandidateLimit:      contracts.RedisCandidateLimit,
		MatchTTLSeconds:     contracts.RedisMatchTTLSeconds,
	}
}

func newKeyer() keyer {
	return keyer{}
}

func newScoreCodec() scoreCodec {
	return scoreCodec{keyer: keyer{}}
}

func newMetrics() *queueMetrics {
	return &queueMetrics{}
}

func newScriptCache(loader redisScriptLoader, metrics contracts.RedisQueueMetrics) *scriptCache {
	return &scriptCache{loader: loader, metrics: metrics}
}

func newStore(adapter redisAdapter, cache *scriptCache, metrics contracts.RedisQueueMetrics, config contracts.RedisQueueConfig) *redisStore {
	if config.CommandTimeoutNanos <= 0 {
		config.CommandTimeoutNanos = contracts.RedisCommandTimeoutNanos
	}
	if config.CandidateLimit <= 0 {
		config.CandidateLimit = contracts.RedisCandidateLimit
	}
	if config.MatchTTLSeconds <= 0 {
		config.MatchTTLSeconds = contracts.RedisMatchTTLSeconds
	}
	if metrics == nil {
		metrics = newMetrics()
	}
	if cache == nil {
		cache = newScriptCache(nil, metrics)
	}
	store := &redisStore{
		adapter: adapter,
		keyer:   keyer{},
		codec:   scoreCodec{keyer: keyer{}},
		cache:   cache,
		metrics: metrics,
		timeout: time.Duration(config.CommandTimeoutNanos),
		limit:   config.CandidateLimit,
		ttl:     config.MatchTTLSeconds,
	}
	switch adapter.(type) {
	case goRedisAdapter, *goRedisAdapter:
		store.deadlineContext = true
	}
	return store
}

func newGoRedisAdapter(client redis.UniversalClient) goRedisAdapter {
	return goRedisAdapter{client: client}
}

// NewKeyer creates the Redis pool/key mapper.
func NewKeyer() contracts.RedisQueueKeyer {
	return newKeyer()
}

// NewScoreCodec creates the Redis score and member codec.
func NewScoreCodec() contracts.RedisScoreCodec {
	return newScoreCodec()
}

// NewMetrics creates Redis queue metrics counters.
func NewMetrics() contracts.RedisQueueMetrics {
	return newMetrics()
}

// NewUniversalStore creates a Redis-backed queue store and script cache.
func NewUniversalStore(client redis.UniversalClient, config contracts.RedisQueueConfig, metrics contracts.RedisQueueMetrics) (contracts.RedisQueueStore, contracts.RedisScriptCache) {
	adapter := newGoRedisAdapter(client)
	if metrics == nil {
		metrics = newMetrics()
	}
	cache := newScriptCache(adapter, metrics)
	return newStore(adapter, cache, metrics, config), cache
}

func (keyer) SegmentForTrophies(trophies int32) contracts.RedisQueuePool {
	switch {
	case trophies < 0:
		return invalidRedisQueuePool
	case trophies <= 1000:
		return contracts.RedisPoolSegment0
	case trophies <= 3000:
		return contracts.RedisPoolSegment1
	case trophies <= 6000:
		return contracts.RedisPoolSegment2
	case trophies <= 10000:
		return contracts.RedisPoolSegment3
	default:
		return contracts.RedisPoolSegment4
	}
}

func (keyer) KeyForPool(pool contracts.RedisQueuePool) (string, contracts.RedisQueueStatus) {
	switch pool {
	case contracts.RedisPoolSegment0:
		return contracts.RedisSegment0Key, contracts.RedisStatusOK
	case contracts.RedisPoolSegment1:
		return contracts.RedisSegment1Key, contracts.RedisStatusOK
	case contracts.RedisPoolSegment2:
		return contracts.RedisSegment2Key, contracts.RedisStatusOK
	case contracts.RedisPoolSegment3:
		return contracts.RedisSegment3Key, contracts.RedisStatusOK
	case contracts.RedisPoolSegment4:
		return contracts.RedisSegment4Key, contracts.RedisStatusOK
	case contracts.RedisPoolLosers:
		return contracts.RedisLosersKey, contracts.RedisStatusOK
	case contracts.RedisPoolRetention:
		return contracts.RedisRetentionKey, contracts.RedisStatusOK
	case contracts.RedisPoolMonetize:
		return contracts.RedisMonetizeKey, contracts.RedisStatusOK
	default:
		return "", contracts.RedisStatusInvalidSegment
	}
}

func (keyer) SegmentRange(pool contracts.RedisQueuePool) (contracts.RedisSegmentRange, contracts.RedisQueueStatus) {
	if pool > contracts.RedisPoolSegment4 {
		return contracts.RedisSegmentRange{}, contracts.RedisStatusInvalidSegment
	}
	return segmentRanges[pool], contracts.RedisStatusOK
}

func (scoreCodec) EncodeMember(playerID uint64, out *contracts.RedisMember) contracts.RedisQueueStatus {
	if out == nil || playerID == 0 {
		if out != nil {
			*out = contracts.RedisMember{}
		}
		return contracts.RedisStatusInvalidScore
	}
	*out = contracts.RedisMember{PlayerID: playerID}
	bytes := strconv.AppendUint(out.Bytes[:0], playerID, 10)
	out.Len = uint8(len(bytes))
	return contracts.RedisStatusOK
}

func (scoreCodec) EncodeScore(trophies int32, enqueuedAtUnixNano int64, out *contracts.RedisScore) contracts.RedisQueueStatus {
	if out == nil || trophies < 0 || enqueuedAtUnixNano < 0 {
		if out != nil {
			*out = contracts.RedisScore{}
		}
		return contracts.RedisStatusInvalidScore
	}
	micros := (enqueuedAtUnixNano / 1_000) % contracts.RedisScoreTrophyScale
	trophyComponent := int64(trophies) * contracts.RedisScoreTrophyScale
	if micros > math.MaxInt64-trophyComponent {
		*out = contracts.RedisScore{}
		return contracts.RedisStatusInvalidScore
	}
	scoreInt := trophyComponent + micros
	if scoreInt < 0 || scoreInt > exactIntegerFloat64 {
		*out = contracts.RedisScore{}
		return contracts.RedisStatusInvalidScore
	}
	*out = contracts.RedisScore{
		Value:            float64(scoreInt),
		Trophies:         trophies,
		EnqueuedAtMicros: micros,
	}
	return contracts.RedisStatusOK
}

func (c scoreCodec) ScoreRange(trophies int32, enqueuedAtUnixNano int64, toleranceTrophies int32, pool contracts.RedisQueuePool, out *contracts.RedisScoreRange) contracts.RedisQueueStatus {
	if out == nil || toleranceTrophies < 0 {
		if out != nil {
			*out = contracts.RedisScoreRange{}
		}
		return contracts.RedisStatusInvalidScore
	}
	if _, status := c.keyer.KeyForPool(pool); status != contracts.RedisStatusOK {
		*out = contracts.RedisScoreRange{}
		return status
	}
	var score contracts.RedisScore
	if status := c.EncodeScore(trophies, enqueuedAtUnixNano, &score); status != contracts.RedisStatusOK {
		*out = contracts.RedisScoreRange{}
		return status
	}
	delta := float64(int64(toleranceTrophies) * contracts.RedisScoreTrophyScale)
	*out = contracts.RedisScoreRange{
		Pool:  pool,
		Min:   score.Value - delta,
		Max:   score.Value + delta,
		Limit: contracts.RedisCandidateLimit,
	}
	return contracts.RedisStatusOK
}

func buildQueueEntry(ticket *contracts.Ticket, out *contracts.RedisQueueEntry) contracts.RedisQueueStatus {
	if ticket == nil || out == nil {
		return contracts.RedisStatusInvalidScore
	}
	codec := newScoreCodec()
	pool := queuePoolForTicket(*ticket)
	if pool == invalidRedisQueuePool {
		*out = contracts.RedisQueueEntry{}
		return contracts.RedisStatusInvalidSegment
	}
	*out = contracts.RedisQueueEntry{Ticket: *ticket, Pool: pool}
	if status := codec.EncodeMember(ticket.PlayerID, &out.Member); status != contracts.RedisStatusOK {
		*out = contracts.RedisQueueEntry{}
		return status
	}
	if status := codec.EncodeScore(ticket.Trophies, ticket.EnqueuedAt, &out.Score); status != contracts.RedisStatusOK {
		*out = contracts.RedisQueueEntry{}
		return status
	}
	return contracts.RedisStatusOK
}

func queuePoolForTicket(ticket contracts.Ticket) contracts.RedisQueuePool {
	switch ticket.PoolTag {
	case contracts.PoolMainstream:
		return keyer{}.SegmentForTrophies(ticket.Trophies)
	case contracts.PoolLosers:
		return contracts.RedisPoolLosers
	case contracts.PoolRetention:
		return contracts.RedisPoolRetention
	case contracts.PoolMonetize:
		return contracts.RedisPoolMonetize
	default:
		return invalidRedisQueuePool
	}
}

func (m *queueMetrics) IncLatency(_ contracts.RedisQueueStatus, elapsedNanos int64) {
	if m == nil {
		return
	}
	m.latencyCount.Add(1)
	m.latencyNanos.Add(elapsedNanos)
}

func (m *queueMetrics) IncScriptReload() {
	if m != nil {
		m.scriptReloads.Add(1)
	}
}

func (m *queueMetrics) IncDualBooking() {
	if m != nil {
		m.dualBookings.Add(1)
	}
}

func (m *queueMetrics) IncPipelinePartial() {
	if m != nil {
		m.pipelinePartial.Add(1)
	}
}

func (c *scriptCache) LoadScripts(ctx context.Context) contracts.RedisOperationResult {
	start := time.Now()
	if c == nil || c.loader == nil {
		return contracts.RedisOperationResult{Status: contracts.RedisStatusUnavailable}
	}
	sha, result := c.loader.scriptLoad(ctx, contracts.RedisAssignMatchLua)
	elapsed := time.Since(start).Nanoseconds()
	status := statusFromContext(ctx, result.status)
	if status != contracts.RedisStatusOK {
		if status == contracts.RedisStatusTimeout && c.metrics != nil {
			c.metrics.IncLatency(status, elapsed)
		}
		return contracts.RedisOperationResult{Status: status, ElapsedNanos: elapsed}
	}
	if sha.Len != 40 {
		return contracts.RedisOperationResult{Status: contracts.RedisStatusUnavailable, ElapsedNanos: elapsed}
	}
	c.mu.Lock()
	c.shas[contracts.RedisScriptAssignMatch] = sha
	c.mu.Unlock()
	return contracts.RedisOperationResult{Status: contracts.RedisStatusOK, Count: 1, ElapsedNanos: elapsed}
}

func (c *scriptCache) ScriptSHA(kind contracts.RedisScriptKind, out *contracts.RedisScriptSHA) contracts.RedisQueueStatus {
	if c == nil || out == nil || kind != contracts.RedisScriptAssignMatch {
		if out != nil {
			*out = contracts.RedisScriptSHA{}
		}
		return contracts.RedisStatusScriptNotLoaded
	}
	c.mu.RLock()
	sha := c.shas[kind]
	c.mu.RUnlock()
	if sha.Len != 40 {
		*out = contracts.RedisScriptSHA{}
		return contracts.RedisStatusScriptNotLoaded
	}
	*out = sha
	return contracts.RedisStatusOK
}

func (c *scriptCache) MarkNoScript(kind contracts.RedisScriptKind) contracts.RedisQueueStatus {
	if c == nil || kind != contracts.RedisScriptAssignMatch {
		return contracts.RedisStatusScriptNotLoaded
	}
	c.mu.Lock()
	c.shas[kind] = contracts.RedisScriptSHA{Kind: kind}
	c.mu.Unlock()
	return contracts.RedisStatusOK
}

func (s *redisStore) Enqueue(ctx context.Context, entry *contracts.RedisQueueEntry) contracts.RedisOperationResult {
	start := time.Now()
	if s == nil || s.adapter == nil || entry == nil {
		return contracts.RedisOperationResult{Status: contracts.RedisStatusUnavailable}
	}
	key, status := s.keyer.KeyForPool(entry.Pool)
	if status != contracts.RedisStatusOK {
		return contracts.RedisOperationResult{Status: status, Pool: entry.Pool}
	}
	if entry.Member.Len == 0 {
		return contracts.RedisOperationResult{Status: contracts.RedisStatusInvalidScore, Pool: entry.Pool}
	}
	opCtx, cancel, status := s.operationContext(ctx)
	if status != contracts.RedisStatusOK {
		return contracts.RedisOperationResult{Status: status, Pool: entry.Pool}
	}
	defer cancel()
	member := memberString(entry.Member)
	result := s.adapter.zadd(opCtx, key, entry.Score.Value, member, entry)
	elapsed := time.Since(start).Nanoseconds()
	status = s.finalStatus(opCtx, result.status, elapsed)
	if status != contracts.RedisStatusOK {
		return contracts.RedisOperationResult{Status: status, Pool: entry.Pool, ElapsedNanos: elapsed}
	}
	return contracts.RedisOperationResult{Status: contracts.RedisStatusOK, Pool: entry.Pool, Count: 1, ElapsedNanos: elapsed}
}

func (s *redisStore) Remove(ctx context.Context, pool contracts.RedisQueuePool, member contracts.RedisMember) contracts.RedisOperationResult {
	start := time.Now()
	if s == nil || s.adapter == nil {
		return contracts.RedisOperationResult{Status: contracts.RedisStatusUnavailable}
	}
	key, status := s.keyer.KeyForPool(pool)
	if status != contracts.RedisStatusOK {
		return contracts.RedisOperationResult{Status: status, Pool: pool}
	}
	opCtx, cancel, status := s.operationContext(ctx)
	if status != contracts.RedisStatusOK {
		return contracts.RedisOperationResult{Status: status, Pool: pool}
	}
	defer cancel()
	count, result := s.adapter.zrem(opCtx, key, memberString(member))
	elapsed := time.Since(start).Nanoseconds()
	status = s.finalStatus(opCtx, result.status, elapsed)
	return contracts.RedisOperationResult{Status: status, Pool: pool, Count: count, ElapsedNanos: elapsed}
}

func (s *redisStore) FetchCandidates(ctx context.Context, query contracts.RedisScoreRange, dst []contracts.RedisCandidate) contracts.RedisOperationResult {
	start := time.Now()
	if s == nil || s.adapter == nil {
		return contracts.RedisOperationResult{Status: contracts.RedisStatusUnavailable}
	}
	key, status := s.keyer.KeyForPool(query.Pool)
	if status != contracts.RedisStatusOK {
		return contracts.RedisOperationResult{Status: status, Pool: query.Pool}
	}
	if len(dst) == 0 {
		return contracts.RedisOperationResult{Status: contracts.RedisStatusEmpty, Pool: query.Pool}
	}
	limit := query.Limit
	if limit <= 0 || limit > s.limit {
		limit = s.limit
	}
	if int64(len(dst)) < limit {
		limit = int64(len(dst))
	}
	opCtx, cancel, status := s.operationContext(ctx)
	if status != contracts.RedisStatusOK {
		return contracts.RedisOperationResult{Status: status, Pool: query.Pool}
	}
	defer cancel()
	count, result := s.adapter.zrangeByScore(opCtx, key, query.Min, query.Max, limit, dst, query.Pool)
	elapsed := time.Since(start).Nanoseconds()
	status = s.finalStatus(opCtx, result.status, elapsed)
	if status == contracts.RedisStatusOK && count == 0 {
		status = contracts.RedisStatusEmpty
	}
	return contracts.RedisOperationResult{Status: status, Pool: query.Pool, Count: count, ElapsedNanos: elapsed}
}

func (s *redisStore) FetchCandidateBatch(ctx context.Context, batch *contracts.RedisQueryBatch) contracts.RedisOperationResult {
	start := time.Now()
	if s == nil || s.adapter == nil {
		return contracts.RedisOperationResult{Status: contracts.RedisStatusUnavailable}
	}
	if batch == nil || int(batch.Count) > len(batch.Ranges) || int(batch.Count) > len(batch.Results) {
		return contracts.RedisOperationResult{Status: contracts.RedisStatusInvalidSegment}
	}
	for i := 0; i < int(batch.Count); i++ {
		if _, status := s.keyer.KeyForPool(batch.Ranges[i].Pool); status != contracts.RedisStatusOK {
			return contracts.RedisOperationResult{Status: status}
		}
	}
	opCtx, cancel, status := s.operationContext(ctx)
	if status != contracts.RedisStatusOK {
		return contracts.RedisOperationResult{Status: status}
	}
	defer cancel()
	count, result := s.adapter.zrangeByScoreBatch(opCtx, batch.Ranges[:batch.Count], batch.Results[:batch.Count])
	elapsed := time.Since(start).Nanoseconds()
	status = s.finalStatus(opCtx, result.status, elapsed)
	if status == contracts.RedisStatusPartial && s.metrics != nil {
		s.metrics.IncPipelinePartial()
	}
	if status == contracts.RedisStatusOK && count == 0 {
		status = contracts.RedisStatusEmpty
	}
	return contracts.RedisOperationResult{Status: status, Count: count, ElapsedNanos: elapsed}
}

func (s *redisStore) MovePool(ctx context.Context, move contracts.RedisMoveRequest) contracts.RedisOperationResult {
	start := time.Now()
	if s == nil || s.adapter == nil {
		return contracts.RedisOperationResult{Status: contracts.RedisStatusUnavailable}
	}
	fromKey, status := s.keyer.KeyForPool(move.From)
	if status != contracts.RedisStatusOK {
		return contracts.RedisOperationResult{Status: status, Pool: move.From}
	}
	toKey, status := s.keyer.KeyForPool(move.To)
	if status != contracts.RedisStatusOK {
		return contracts.RedisOperationResult{Status: status, Pool: move.To}
	}
	opCtx, cancel, status := s.operationContext(ctx)
	if status != contracts.RedisStatusOK {
		return contracts.RedisOperationResult{Status: status, Pool: move.From}
	}
	defer cancel()
	count, result := s.adapter.move(opCtx, fromKey, toKey, memberString(move.Member), move.Score.Value)
	elapsed := time.Since(start).Nanoseconds()
	status = s.finalStatus(opCtx, result.status, elapsed)
	return contracts.RedisOperationResult{Status: status, Pool: move.To, Count: count, ElapsedNanos: elapsed}
}

func (s *redisStore) AssignMatch(ctx context.Context, req contracts.RedisAssignRequest) contracts.RedisAssignResult {
	start := time.Now()
	result := contracts.RedisAssignResult{MatchID: req.MatchID, PlayerA: req.PlayerA, PlayerB: req.PlayerB}
	if s == nil || s.adapter == nil || s.cache == nil {
		result.Status = contracts.RedisStatusUnavailable
		return result
	}
	if req.PlayerA.PlayerID == 0 || req.PlayerA.PlayerID == req.PlayerB.PlayerID {
		result.Status = contracts.RedisStatusInvalidScore
		return result
	}
	sourceA, status := s.keyer.KeyForPool(req.SourceA)
	if status != contracts.RedisStatusOK {
		result.Status = status
		return result
	}
	sourceB, status := s.keyer.KeyForPool(req.SourceB)
	if status != contracts.RedisStatusOK {
		result.Status = status
		return result
	}
	var sha contracts.RedisScriptSHA
	if status = s.cache.ScriptSHA(contracts.RedisScriptAssignMatch, &sha); status != contracts.RedisStatusOK {
		result.Status = status
		return result
	}
	opCtx, cancel, status := s.operationContext(ctx)
	if status != contracts.RedisStatusOK {
		result.Status = status
		return result
	}
	defer cancel()
	luaResult, adapterResult := s.adapter.evalAssign(
		opCtx,
		shaString(sha),
		sourceA,
		sourceB,
		memberString(req.PlayerA),
		memberString(req.PlayerB),
		strconv.FormatUint(req.MatchID, 10),
		s.ttl,
	)
	elapsed := time.Since(start).Nanoseconds()
	status = s.finalStatus(opCtx, adapterResult.status, elapsed)
	result.ElapsedNanos = elapsed
	switch status {
	case contracts.RedisStatusOK:
		if luaResult == 1 {
			result.Status = contracts.RedisStatusOK
			return result
		}
		result.Status = contracts.RedisStatusDualBooking
		if s.metrics != nil {
			s.metrics.IncDualBooking()
		}
		return result
	case contracts.RedisStatusNoScript:
		s.cache.MarkNoScript(contracts.RedisScriptAssignMatch)
		if s.metrics != nil {
			s.metrics.IncScriptReload()
		}
		result.Status = contracts.RedisStatusNoScript
		return result
	default:
		result.Status = status
		return result
	}
}

func (s *redisStore) operationContext(ctx context.Context) (context.Context, context.CancelFunc, contracts.RedisQueueStatus) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ctx, func() {}, contextStatus(err)
	}
	if s.deadlineContext {
		opCtx, cancel := context.WithTimeout(ctx, s.timeout)
		return opCtx, cancel, contracts.RedisStatusOK
	}
	return ctx, func() {}, contracts.RedisStatusOK
}

func (s *redisStore) finalStatus(ctx context.Context, status contracts.RedisQueueStatus, elapsed int64) contracts.RedisQueueStatus {
	status = statusFromContext(ctx, status)
	if status == contracts.RedisStatusOK && elapsed > int64(s.timeout) {
		status = contracts.RedisStatusTimeout
	}
	if status == contracts.RedisStatusTimeout && s.metrics != nil {
		s.metrics.IncLatency(status, elapsed)
	}
	return status
}

func statusFromContext(ctx context.Context, status contracts.RedisQueueStatus) contracts.RedisQueueStatus {
	if ctx == nil {
		return status
	}
	if err := ctx.Err(); err != nil {
		return contextStatus(err)
	}
	return status
}

func contextStatus(err error) contracts.RedisQueueStatus {
	if errors.Is(err, context.Canceled) {
		return contracts.RedisStatusCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return contracts.RedisStatusTimeout
	}
	return contracts.RedisStatusUnavailable
}

func memberString(member contracts.RedisMember) string {
	return string(member.Bytes[:member.Len])
}

func shaString(sha contracts.RedisScriptSHA) string {
	return string(sha.Hex[:sha.Len])
}

func makePlayerKey(member string) string {
	var buf [len(contracts.RedisPlayerKeyPrefix) + 20]byte
	n := copy(buf[:], contracts.RedisPlayerKeyPrefix)
	n += copy(buf[n:], member)
	return string(buf[:n])
}

func decodeCandidate(member string, score float64, pool contracts.RedisQueuePool, out *contracts.RedisCandidate) bool {
	playerID, err := strconv.ParseUint(member, 10, 64)
	if err != nil || playerID == 0 {
		return false
	}
	if newScoreCodec().EncodeMember(playerID, &out.Member) != contracts.RedisStatusOK {
		return false
	}
	scoreInt := int64(score)
	out.Score = contracts.RedisScore{
		Value:            score,
		Trophies:         int32(scoreInt / contracts.RedisScoreTrophyScale),
		EnqueuedAtMicros: scoreInt % contracts.RedisScoreTrophyScale,
	}
	out.Pool = pool
	return true
}

func (a goRedisAdapter) scriptLoad(ctx context.Context, script string) (contracts.RedisScriptSHA, redisAdapterResult) {
	if a.client == nil {
		return contracts.RedisScriptSHA{}, redisAdapterResult{status: contracts.RedisStatusUnavailable}
	}
	hex, err := a.client.ScriptLoad(ctx, script).Result()
	if err != nil {
		return contracts.RedisScriptSHA{}, redisAdapterResult{status: statusFromRedisError(ctx, err)}
	}
	var sha contracts.RedisScriptSHA
	sha.Kind = contracts.RedisScriptAssignMatch
	if len(hex) != len(sha.Hex) {
		return contracts.RedisScriptSHA{}, redisAdapterResult{status: contracts.RedisStatusUnavailable}
	}
	copy(sha.Hex[:], hex)
	sha.Len = uint8(len(hex))
	return sha, redisAdapterResult{status: contracts.RedisStatusOK}
}

func (a goRedisAdapter) zadd(ctx context.Context, key string, score float64, member string, entry *contracts.RedisQueueEntry) redisAdapterResult {
	if a.client == nil {
		return redisAdapterResult{status: contracts.RedisStatusUnavailable}
	}
	if err := a.client.ZAdd(ctx, key, redis.Z{Score: score, Member: member}).Err(); err != nil {
		return redisAdapterResult{status: statusFromRedisError(ctx, err)}
	}
	playerKey := makePlayerKey(member)
	if err := a.client.HSet(ctx, playerKey,
		"trophies", entry.Ticket.Trophies,
		"churn", entry.Ticket.ChurnRisk,
		"monetization", entry.Ticket.MonetizationP,
		"losses", entry.Ticket.ConsecLosses,
		"wins", entry.Ticket.ConsecWins,
		"pool", uint8(entry.Pool),
	).Err(); err != nil {
		return redisAdapterResult{status: statusFromRedisError(ctx, err)}
	}
	return redisAdapterResult{status: contracts.RedisStatusOK}
}

func (a goRedisAdapter) zrem(ctx context.Context, key string, member string) (uint16, redisAdapterResult) {
	if a.client == nil {
		return 0, redisAdapterResult{status: contracts.RedisStatusUnavailable}
	}
	count, err := a.client.ZRem(ctx, key, member).Result()
	if err != nil {
		return 0, redisAdapterResult{status: statusFromRedisError(ctx, err)}
	}
	return uint16(count), redisAdapterResult{status: contracts.RedisStatusOK}
}

func (a goRedisAdapter) zrangeByScore(ctx context.Context, key string, min float64, max float64, limit int64, dst []contracts.RedisCandidate, pool contracts.RedisQueuePool) (uint16, redisAdapterResult) {
	if a.client == nil {
		return 0, redisAdapterResult{status: contracts.RedisStatusUnavailable}
	}
	values, err := a.client.ZRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
		Min:    strconv.FormatFloat(min, 'f', -1, 64),
		Max:    strconv.FormatFloat(max, 'f', -1, 64),
		Offset: 0,
		Count:  limit,
	}).Result()
	if err != nil {
		return 0, redisAdapterResult{status: statusFromRedisError(ctx, err)}
	}
	count := uint16(0)
	for i := range values {
		if int(count) >= len(dst) {
			break
		}
		member, ok := values[i].Member.(string)
		if !ok {
			continue
		}
		if decodeCandidate(member, values[i].Score, pool, &dst[count]) {
			count++
		}
	}
	return count, redisAdapterResult{status: contracts.RedisStatusOK}
}

func (a goRedisAdapter) zrangeByScoreBatch(ctx context.Context, ranges []contracts.RedisScoreRange, results [][]contracts.RedisCandidate) (uint16, redisAdapterResult) {
	if a.client == nil {
		return 0, redisAdapterResult{status: contracts.RedisStatusUnavailable}
	}
	keyer := newKeyer()
	commands := make([]*redis.ZSliceCmd, len(ranges))
	_, err := a.client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for i := range ranges {
			key, status := keyer.KeyForPool(ranges[i].Pool)
			if status != contracts.RedisStatusOK {
				continue
			}
			limit := ranges[i].Limit
			if limit <= 0 || limit > contracts.RedisCandidateLimit {
				limit = contracts.RedisCandidateLimit
			}
			commands[i] = pipe.ZRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
				Min:    strconv.FormatFloat(ranges[i].Min, 'f', -1, 64),
				Max:    strconv.FormatFloat(ranges[i].Max, 'f', -1, 64),
				Offset: 0,
				Count:  limit,
			})
		}
		return nil
	})
	if err != nil && !errors.Is(err, redis.Nil) {
		return 0, redisAdapterResult{status: statusFromRedisError(ctx, err)}
	}
	var total uint16
	var okCount uint16
	var failCount uint16
	for i, cmd := range commands {
		if cmd == nil {
			failCount++
			continue
		}
		values, cmdErr := cmd.Result()
		if cmdErr != nil {
			failCount++
			continue
		}
		okCount++
		dst := results[i]
		for j := range values {
			if total == math.MaxUint16 || j >= len(dst) {
				break
			}
			member, ok := values[j].Member.(string)
			if !ok {
				continue
			}
			if decodeCandidate(member, values[j].Score, ranges[i].Pool, &dst[j]) {
				total++
			}
		}
	}
	if okCount > 0 && failCount > 0 {
		return total, redisAdapterResult{status: contracts.RedisStatusPartial}
	}
	if failCount > 0 {
		return total, redisAdapterResult{status: contracts.RedisStatusUnavailable}
	}
	return total, redisAdapterResult{status: contracts.RedisStatusOK}
}

func (a goRedisAdapter) move(ctx context.Context, fromKey string, toKey string, member string, score float64) (uint16, redisAdapterResult) {
	if a.client == nil {
		return 0, redisAdapterResult{status: contracts.RedisStatusUnavailable}
	}
	_, err := a.client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.ZRem(ctx, fromKey, member)
		pipe.ZAdd(ctx, toKey, redis.Z{Score: score, Member: member})
		return nil
	})
	if err != nil {
		return 0, redisAdapterResult{status: statusFromRedisError(ctx, err)}
	}
	return 1, redisAdapterResult{status: contracts.RedisStatusOK}
}

func (a goRedisAdapter) evalAssign(ctx context.Context, sha string, sourceA string, sourceB string, playerA string, playerB string, matchID string, _ int64) (int64, redisAdapterResult) {
	if a.client == nil {
		return 0, redisAdapterResult{status: contracts.RedisStatusUnavailable}
	}
	value, err := a.client.EvalSha(ctx, sha, []string{sourceA, sourceB}, playerA, playerB, matchID).Int64()
	if err != nil {
		return 0, redisAdapterResult{status: statusFromRedisError(ctx, err)}
	}
	return value, redisAdapterResult{status: contracts.RedisStatusOK}
}

func statusFromRedisError(ctx context.Context, err error) contracts.RedisQueueStatus {
	if err == nil {
		return statusFromContext(ctx, contracts.RedisStatusOK)
	}
	if status := statusFromContext(ctx, contracts.RedisStatusOK); status != contracts.RedisStatusOK {
		return status
	}
	if strings.Contains(err.Error(), "NOSCRIPT") {
		return contracts.RedisStatusNoScript
	}
	if errors.Is(err, redis.Nil) {
		return contracts.RedisStatusEmpty
	}
	return contracts.RedisStatusUnavailable
}
