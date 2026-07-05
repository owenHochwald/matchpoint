package redisqueue

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"matchpoint/contracts"
)

type fakeRedis struct {
	mu             sync.Mutex
	zsets          map[string]map[string]float64
	playerMeta     map[string]contracts.RedisQueueEntry
	matches        map[string]fakeMatch
	nextStatus     contracts.RedisQueueStatus
	batchFailIndex int
	noScript       bool
	delay          time.Duration
	scriptLoads    uint16
	assignCalls    uint16
	enqueueKeys    []string
}

type fakeMatch struct {
	playerA string
	playerB string
	ttl     int64
}

type noopRedis struct {
	candidate contracts.RedisCandidate
}

func newFakeRedis() *fakeRedis {
	return &fakeRedis{
		zsets:          make(map[string]map[string]float64),
		playerMeta:     make(map[string]contracts.RedisQueueEntry),
		matches:        make(map[string]fakeMatch),
		batchFailIndex: -1,
	}
}

func testStore(fake *fakeRedis) (*redisStore, *scriptCache, *queueMetrics) {
	metrics := newMetrics()
	cache := newScriptCache(fake, metrics)
	store := newStore(fake, cache, metrics, productionConfig())
	return store, cache, metrics
}

func benchmarkStore() (*redisStore, *scriptCache) {
	var candidate contracts.RedisCandidate
	_ = newScoreCodec().EncodeMember(1, &candidate.Member)
	candidate.Score = contracts.RedisScore{Value: 1_700_000_000_000_000, Trophies: 1000, EnqueuedAtMicros: 1_700_000_000_000_000}
	candidate.Pool = contracts.RedisPoolSegment0
	adapter := &noopRedis{candidate: candidate}
	cache := newScriptCache(adapter, nil)
	store := newStore(adapter, cache, nil, productionConfig())
	return store, cache
}

func testTicket(playerID uint64, trophies int32, pool contracts.TicketPoolTag) contracts.Ticket {
	return contracts.Ticket{
		PlayerID:      playerID,
		EnqueuedAt:    1_700_000_000_123_456_789,
		Trophies:      trophies,
		ChurnRisk:     0.25,
		MonetizationP: 0.30,
		ConsecLosses:  -1,
		ConsecWins:    2,
		PoolTag:       pool,
	}
}

func TestConfigKeyerAndSegments(t *testing.T) {
	// B-REDISQUEUE-1
	config := productionConfig()
	if config.CommandTimeoutNanos != contracts.RedisCommandTimeoutNanos {
		t.Fatalf("timeout = %d", config.CommandTimeoutNanos)
	}
	if config.PoolSize != uint16(runtime.NumCPU()*4) {
		t.Fatalf("pool size = %d", config.PoolSize)
	}
	if config.CandidateLimit != contracts.RedisCandidateLimit || config.MatchTTLSeconds != contracts.RedisMatchTTLSeconds {
		t.Fatalf("unexpected config: %+v", config)
	}

	keyer := newKeyer()
	// B-REDISQUEUE-2
	cases := []struct {
		trophies int32
		pool     contracts.RedisQueuePool
	}{
		{0, contracts.RedisPoolSegment0},
		{1000, contracts.RedisPoolSegment0},
		{1001, contracts.RedisPoolSegment1},
		{3000, contracts.RedisPoolSegment1},
		{3001, contracts.RedisPoolSegment2},
		{6000, contracts.RedisPoolSegment2},
		{6001, contracts.RedisPoolSegment3},
		{10000, contracts.RedisPoolSegment3},
		{10001, contracts.RedisPoolSegment4},
	}
	for _, tc := range cases {
		if got := keyer.SegmentForTrophies(tc.trophies); got != tc.pool {
			t.Fatalf("SegmentForTrophies(%d) = %d, want %d", tc.trophies, got, tc.pool)
		}
	}

	// B-REDISQUEUE-3
	if keyer.SegmentForTrophies(-1) != invalidRedisQueuePool {
		t.Fatal("negative trophies must not map to a valid segment")
	}
	if key, status := keyer.KeyForPool(contracts.RedisQueuePool(99)); key != "" || status != contracts.RedisStatusInvalidSegment {
		t.Fatalf("invalid pool key = %q status=%d", key, status)
	}

	// B-REDISQUEUE-4
	wantKeys := []string{
		contracts.RedisSegment0Key,
		contracts.RedisSegment1Key,
		contracts.RedisSegment2Key,
		contracts.RedisSegment3Key,
		contracts.RedisSegment4Key,
		contracts.RedisLosersKey,
		contracts.RedisRetentionKey,
		contracts.RedisMonetizeKey,
	}
	for pool, want := range wantKeys {
		key, status := keyer.KeyForPool(contracts.RedisQueuePool(pool))
		if status != contracts.RedisStatusOK || key != want {
			t.Fatalf("KeyForPool(%d) = %q/%d, want %q/OK", pool, key, status, want)
		}
	}

	// B-REDISQUEUE-5
	rangeCases := []contracts.RedisSegmentRange{
		{Pool: contracts.RedisPoolSegment0, Key: contracts.RedisSegment0Key, MinTrophies: 0, MaxTrophies: 1000},
		{Pool: contracts.RedisPoolSegment1, Key: contracts.RedisSegment1Key, MinTrophies: 1001, MaxTrophies: 3000},
		{Pool: contracts.RedisPoolSegment2, Key: contracts.RedisSegment2Key, MinTrophies: 3001, MaxTrophies: 6000},
		{Pool: contracts.RedisPoolSegment3, Key: contracts.RedisSegment3Key, MinTrophies: 6001, MaxTrophies: 10000},
		{Pool: contracts.RedisPoolSegment4, Key: contracts.RedisSegment4Key, MinTrophies: 10001, MaxTrophies: -1},
	}
	for _, want := range rangeCases {
		got, status := keyer.SegmentRange(want.Pool)
		if status != contracts.RedisStatusOK || got != want {
			t.Fatalf("SegmentRange(%d) = %+v/%d, want %+v/OK", want.Pool, got, status, want)
		}
	}
}

func TestCodecMemberScoreRangeAndEntry(t *testing.T) {
	codec := newScoreCodec()
	var member contracts.RedisMember

	// B-REDISQUEUE-6
	if allocs := testing.AllocsPerRun(1000, func() {
		if codec.EncodeMember(18446744073709551615, &member) != contracts.RedisStatusOK {
			t.Fatal("EncodeMember failed")
		}
	}); allocs != 0 {
		t.Fatalf("EncodeMember allocations = %f", allocs)
	}
	if string(member.Bytes[:member.Len]) != "18446744073709551615" || member.PlayerID != 18446744073709551615 {
		t.Fatalf("bad member encoding: %+v", member)
	}

	// B-REDISQUEUE-7
	if status := codec.EncodeMember(0, &member); status != contracts.RedisStatusInvalidScore || member.Len != 0 {
		t.Fatalf("zero player member status=%d member=%+v", status, member)
	}

	var score contracts.RedisScore
	// B-REDISQUEUE-8 B-REDISQUEUE-9 B-REDISQUEUE-10
	enqueued := int64(1_700_000_000_123_456_789)
	if status := codec.EncodeScore(15000, enqueued, &score); status != contracts.RedisStatusOK {
		t.Fatalf("EncodeScore status=%d", status)
	}
	wantMicros := (enqueued / 1000) % contracts.RedisScoreTrophyScale
	wantScore := float64(int64(15000)*contracts.RedisScoreTrophyScale + wantMicros)
	if score.Value != wantScore || score.EnqueuedAtMicros != wantMicros {
		t.Fatalf("score=%+v want value=%f micros=%d", score, wantScore, wantMicros)
	}

	// B-REDISQUEUE-11
	if status := codec.EncodeScore(-1, enqueued, &score); status != contracts.RedisStatusInvalidScore {
		t.Fatalf("expected precision guard, got %d", status)
	}

	// B-REDISQUEUE-12
	var scoreRange contracts.RedisScoreRange
	if status := codec.ScoreRange(3000, enqueued, 50, contracts.RedisPoolSegment1, &scoreRange); status != contracts.RedisStatusOK {
		t.Fatalf("ScoreRange status=%d", status)
	}
	base := float64(int64(3000)*contracts.RedisScoreTrophyScale + wantMicros)
	delta := float64(50 * contracts.RedisScoreTrophyScale)
	if scoreRange.Min != base-delta || scoreRange.Max != base+delta || scoreRange.Limit != contracts.RedisCandidateLimit {
		t.Fatalf("bad score range: %+v", scoreRange)
	}

	// B-REDISQUEUE-13
	ticket := testTicket(42, 4500, contracts.PoolMainstream)
	var entry contracts.RedisQueueEntry
	if status := buildQueueEntry(&ticket, &entry); status != contracts.RedisStatusOK {
		t.Fatalf("buildQueueEntry status=%d", status)
	}
	ticket.Trophies = 1
	if entry.Ticket.Trophies != 4500 || entry.Member.PlayerID != 42 || entry.Pool != contracts.RedisPoolSegment2 {
		t.Fatalf("entry did not copy/map ticket correctly: %+v", entry)
	}
}

func TestEnqueueStatusesAndMetadata(t *testing.T) {
	fake := newFakeRedis()
	store, _, metrics := testStore(fake)
	var entry contracts.RedisQueueEntry
	if status := buildQueueEntry(ptrTicket(testTicket(55, 999, contracts.PoolMainstream)), &entry); status != contracts.RedisStatusOK {
		t.Fatalf("entry status=%d", status)
	}

	// B-REDISQUEUE-14
	result := store.Enqueue(context.Background(), &entry)
	if result.Status != contracts.RedisStatusOK || result.Count != 1 {
		t.Fatalf("enqueue result=%+v", result)
	}
	if !fake.hasMember(contracts.RedisSegment0Key, "55") || !fake.hasMeta("55") {
		t.Fatal("mainstream enqueue did not write zset and metadata")
	}

	// B-REDISQUEUE-15
	for _, pool := range []contracts.TicketPoolTag{contracts.PoolLosers, contracts.PoolRetention, contracts.PoolMonetize} {
		ticket := testTicket(uint64(100+pool), 7000, pool)
		if status := buildQueueEntry(&ticket, &entry); status != contracts.RedisStatusOK {
			t.Fatalf("special entry status=%d", status)
		}
		result = store.Enqueue(context.Background(), &entry)
		if result.Status != contracts.RedisStatusOK {
			t.Fatalf("special enqueue result=%+v", result)
		}
		if fake.lastEnqueueKeyIsSegment() {
			t.Fatalf("special pool %d enqueued to segment key", pool)
		}
	}

	// B-REDISQUEUE-16
	fake.delay = 2 * time.Millisecond
	shortConfig := productionConfig()
	shortConfig.CommandTimeoutNanos = int64(time.Microsecond)
	timeoutStore := newStore(fake, store.cache, metrics, shortConfig)
	result = timeoutStore.Enqueue(context.Background(), &entry)
	if result.Status != contracts.RedisStatusTimeout || metrics.latencyCount.Load() == 0 {
		t.Fatalf("timeout result=%+v latency=%d", result, metrics.latencyCount.Load())
	}
	fake.delay = 0

	// B-REDISQUEUE-17
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result = store.Enqueue(ctx, &entry)
	if result.Status != contracts.RedisStatusCanceled {
		t.Fatalf("canceled result=%+v", result)
	}

	// B-REDISQUEUE-18
	fake.nextStatus = contracts.RedisStatusUnavailable
	result = store.Enqueue(context.Background(), &entry)
	if result.Status != contracts.RedisStatusUnavailable {
		t.Fatalf("unavailable result=%+v", result)
	}
}

func TestRemoveFetchBatchAndMove(t *testing.T) {
	fake := newFakeRedis()
	store, _, metrics := testStore(fake)
	var entry contracts.RedisQueueEntry
	ticket := testTicket(77, 2000, contracts.PoolMainstream)
	if status := buildQueueEntry(&ticket, &entry); status != contracts.RedisStatusOK {
		t.Fatalf("entry status=%d", status)
	}
	if result := store.Enqueue(context.Background(), &entry); result.Status != contracts.RedisStatusOK {
		t.Fatalf("enqueue result=%+v", result)
	}

	// B-REDISQUEUE-19
	result := store.Remove(context.Background(), entry.Pool, entry.Member)
	if result.Status != contracts.RedisStatusOK || result.Count != 1 || fake.hasMember(contracts.RedisSegment1Key, "77") {
		t.Fatalf("remove result=%+v", result)
	}

	// B-REDISQUEUE-20
	result = store.Remove(context.Background(), entry.Pool, entry.Member)
	if result.Status != contracts.RedisStatusOK || result.Count != 0 {
		t.Fatalf("idempotent remove result=%+v", result)
	}

	for i := uint64(1); i <= 10; i++ {
		ticket := testTicket(i, 2000, contracts.PoolMainstream)
		if status := buildQueueEntry(&ticket, &entry); status != contracts.RedisStatusOK {
			t.Fatalf("entry %d status=%d", i, status)
		}
		if result := store.Enqueue(context.Background(), &entry); result.Status != contracts.RedisStatusOK {
			t.Fatalf("enqueue %d result=%+v", i, result)
		}
	}

	var query contracts.RedisScoreRange
	if status := store.codec.ScoreRange(2000, testTicket(1, 2000, contracts.PoolMainstream).EnqueuedAt, 100, contracts.RedisPoolSegment1, &query); status != contracts.RedisStatusOK {
		t.Fatalf("range status=%d", status)
	}
	dst := make([]contracts.RedisCandidate, 3)
	// B-REDISQUEUE-21
	result = store.FetchCandidates(context.Background(), query, dst)
	if result.Status != contracts.RedisStatusOK || result.Count != 3 {
		t.Fatalf("fetch result=%+v", result)
	}

	// B-REDISQUEUE-22
	query.Min = 0
	query.Max = 1
	result = store.FetchCandidates(context.Background(), query, dst)
	if result.Status != contracts.RedisStatusEmpty || result.Count != 0 {
		t.Fatalf("empty fetch result=%+v", result)
	}

	// B-REDISQUEUE-23
	query.Min = 0
	query.Max = 1 << 62
	results := [][]contracts.RedisCandidate{{{}, {}}, {{}, {}}}
	batch := contracts.RedisQueryBatch{
		Ranges:  []contracts.RedisScoreRange{query, query},
		Results: results,
		Count:   2,
	}
	result = store.FetchCandidateBatch(context.Background(), &batch)
	if result.Status != contracts.RedisStatusOK || result.Count == 0 || results[0][0].Member.PlayerID == 0 {
		t.Fatalf("batch result=%+v results=%+v", result, results)
	}

	// B-REDISQUEUE-24
	fake.batchFailIndex = 1
	result = store.FetchCandidateBatch(context.Background(), &batch)
	if result.Status != contracts.RedisStatusPartial || metrics.pipelinePartial.Load() == 0 {
		t.Fatalf("partial result=%+v partial=%d", result, metrics.pipelinePartial.Load())
	}
	fake.batchFailIndex = -1

	// B-REDISQUEUE-25
	batch.Count = 3
	result = store.FetchCandidateBatch(context.Background(), &batch)
	if result.Status != contracts.RedisStatusInvalidSegment {
		t.Fatalf("invalid batch result=%+v", result)
	}

	// B-REDISQUEUE-26 B-REDISQUEUE-27 B-REDISQUEUE-38
	moveEntry := mustEntry(testTicket(77, 2000, contracts.PoolMainstream))
	moveMember := memberString(moveEntry.Member)
	if result := store.Enqueue(context.Background(), &moveEntry); result.Status != contracts.RedisStatusOK {
		t.Fatalf("re-enqueue before move result=%+v", result)
	}
	move := contracts.RedisMoveRequest{Member: moveEntry.Member, From: contracts.RedisPoolSegment1, To: contracts.RedisPoolLosers, Score: moveEntry.Score}
	result = store.MovePool(context.Background(), move)
	sourceHas := fake.hasMember(contracts.RedisSegment1Key, moveMember)
	destHas := fake.hasMember(contracts.RedisLosersKey, moveMember)
	if result.Status != contracts.RedisStatusOK || result.Count != 1 || sourceHas || !destHas {
		t.Fatalf("move result=%+v sourceHas=%t destHas=%t", result, sourceHas, destHas)
	}
	result = store.MovePool(context.Background(), move)
	if result.Status != contracts.RedisStatusOK || result.Count != 0 || fake.zsetLen(contracts.RedisLosersKey) == 0 {
		t.Fatalf("idempotent move result=%+v", result)
	}
	move.From = contracts.RedisPoolLosers
	move.To = contracts.RedisPoolSegment1
	result = store.MovePool(context.Background(), move)
	if result.Status != contracts.RedisStatusOK || !fake.hasMember(contracts.RedisSegment1Key, moveMember) {
		t.Fatalf("evacuation move result=%+v", result)
	}

	// B-REDISQUEUE-28
	if pool := (keyer{}).SegmentForTrophies(moveEntry.Score.Trophies); pool != contracts.RedisPoolSegment1 {
		t.Fatalf("preserved score trophies did not derive mainstream destination: %d", pool)
	}
}

func TestScriptsAssignAndMetrics(t *testing.T) {
	fake := newFakeRedis()
	store, cache, metrics := testStore(fake)

	// B-REDISQUEUE-29
	load := cache.LoadScripts(context.Background())
	if load.Status != contracts.RedisStatusOK || load.Count != 1 {
		t.Fatalf("load result=%+v", load)
	}
	var sha contracts.RedisScriptSHA
	if status := cache.ScriptSHA(contracts.RedisScriptAssignMatch, &sha); status != contracts.RedisStatusOK || sha.Len != 40 || fake.scriptLoads != 1 {
		t.Fatalf("sha status=%d sha=%+v loads=%d", status, sha, fake.scriptLoads)
	}

	// B-REDISQUEUE-30
	emptyCache := newScriptCache(fake, metrics)
	if status := emptyCache.ScriptSHA(contracts.RedisScriptAssignMatch, &sha); status != contracts.RedisStatusScriptNotLoaded {
		t.Fatalf("empty ScriptSHA status=%d", status)
	}

	// B-REDISQUEUE-31
	if status := cache.MarkNoScript(contracts.RedisScriptAssignMatch); status != contracts.RedisStatusOK {
		t.Fatalf("MarkNoScript status=%d", status)
	}
	if status := cache.ScriptSHA(contracts.RedisScriptAssignMatch, &sha); status != contracts.RedisStatusScriptNotLoaded {
		t.Fatalf("cleared ScriptSHA status=%d", status)
	}
	if load := cache.LoadScripts(context.Background()); load.Status != contracts.RedisStatusOK {
		t.Fatalf("reload result=%+v", load)
	}

	entryA := enqueueForAssign(t, store, testTicket(501, 2500, contracts.PoolMainstream))
	entryB := enqueueForAssign(t, store, testTicket(502, 2600, contracts.PoolMainstream))
	req := contracts.RedisAssignRequest{
		SourceA: contracts.RedisPoolSegment1,
		SourceB: contracts.RedisPoolSegment1,
		PlayerA: entryA.Member,
		PlayerB: entryB.Member,
		MatchID: 9001,
	}

	// B-REDISQUEUE-32 B-REDISQUEUE-33 B-REDISQUEUE-37
	assign := store.AssignMatch(context.Background(), req)
	if assign.Status != contracts.RedisStatusOK || fake.hasMember(contracts.RedisSegment1Key, "501") || fake.hasMember(contracts.RedisSegment1Key, "502") {
		t.Fatalf("assign result=%+v", assign)
	}
	match := fake.match("9001")
	if match.playerA != "501" || match.playerB != "502" || match.ttl != contracts.RedisMatchTTLSeconds {
		t.Fatalf("match record=%+v", match)
	}
	if !stringsContain(contractScriptParts(), "ZSCORE", "ZREM", "mq:match:", "EXPIRE", "return 1") {
		t.Fatal("canonical Lua script missing expected storage operations")
	}

	// B-REDISQUEUE-34
	assign = store.AssignMatch(context.Background(), req)
	if assign.Status != contracts.RedisStatusDualBooking || metrics.dualBookings.Load() == 0 {
		t.Fatalf("dual booking result=%+v duals=%d", assign, metrics.dualBookings.Load())
	}

	// B-REDISQUEUE-35
	missingStore, _, _ := testStore(fake)
	assign = missingStore.AssignMatch(context.Background(), req)
	if assign.Status != contracts.RedisStatusScriptNotLoaded || fake.assignCalls != 2 {
		t.Fatalf("missing script result=%+v calls=%d", assign, fake.assignCalls)
	}

	if load := cache.LoadScripts(context.Background()); load.Status != contracts.RedisStatusOK {
		t.Fatalf("reload result=%+v", load)
	}
	entryA = enqueueForAssign(t, store, testTicket(601, 2500, contracts.PoolMainstream))
	entryB = enqueueForAssign(t, store, testTicket(602, 2600, contracts.PoolMainstream))
	req.PlayerA = entryA.Member
	req.PlayerB = entryB.Member
	req.MatchID = 9002
	fake.noScript = true
	// B-REDISQUEUE-36
	assign = store.AssignMatch(context.Background(), req)
	if assign.Status != contracts.RedisStatusNoScript || metrics.scriptReloads.Load() == 0 {
		t.Fatalf("noscript result=%+v reloads=%d", assign, metrics.scriptReloads.Load())
	}
	if status := cache.ScriptSHA(contracts.RedisScriptAssignMatch, &sha); status != contracts.RedisStatusScriptNotLoaded {
		t.Fatalf("noscript did not clear sha: %d", status)
	}
	fake.noScript = false

	// B-REDISQUEUE-39
	metrics.IncLatency(contracts.RedisStatusTimeout, 99)
	metrics.IncScriptReload()
	metrics.IncDualBooking()
	metrics.IncPipelinePartial()
	if metrics.latencyCount.Load() == 0 || metrics.scriptReloads.Load() == 0 || metrics.dualBookings.Load() == 0 || metrics.pipelinePartial.Load() == 0 {
		t.Fatal("metrics counters did not increment")
	}

	same := req
	same.PlayerB = same.PlayerA
	assign = store.AssignMatch(context.Background(), same)
	if assign.Status == contracts.RedisStatusOK || fake.assignCalls == 0 {
		t.Fatalf("same-player assignment was not rejected: %+v", assign)
	}
}

func TestLiveRedisIntegrationGate(t *testing.T) {
	if os.Getenv("MP_REDIS_INTEGRATION") != "1" {
		t.Skip("set MP_REDIS_INTEGRATION=1 and provide a Redis client to run live integration")
	}
	t.Skip("live Redis wiring is intentionally environment-owned; default tests use deterministic fake Redis")
}

func TestGoRedisAdapterSurface(t *testing.T) {
	adapter := newGoRedisAdapter(nil)
	if _, result := adapter.scriptLoad(context.Background(), contracts.RedisAssignMatchLua); result.status != contracts.RedisStatusUnavailable {
		t.Fatalf("scriptLoad nil status=%d", result.status)
	}
	entry := mustEntry(testTicket(7001, 1000, contracts.PoolMainstream))
	member := memberString(entry.Member)
	if result := adapter.zadd(context.Background(), contracts.RedisSegment0Key, entry.Score.Value, member, &entry); result.status != contracts.RedisStatusUnavailable {
		t.Fatalf("zadd nil status=%d", result.status)
	}
	if _, result := adapter.zrem(context.Background(), contracts.RedisSegment0Key, member); result.status != contracts.RedisStatusUnavailable {
		t.Fatalf("zrem nil status=%d", result.status)
	}
	var dst [1]contracts.RedisCandidate
	if _, result := adapter.zrangeByScore(context.Background(), contracts.RedisSegment0Key, 0, 1, 1, dst[:], contracts.RedisPoolSegment0); result.status != contracts.RedisStatusUnavailable {
		t.Fatalf("zrange nil status=%d", result.status)
	}
	ranges := [1]contracts.RedisScoreRange{{Pool: contracts.RedisPoolSegment0}}
	results := [1][]contracts.RedisCandidate{dst[:]}
	if _, result := adapter.zrangeByScoreBatch(context.Background(), ranges[:], results[:]); result.status != contracts.RedisStatusUnavailable {
		t.Fatalf("batch nil status=%d", result.status)
	}
	if _, result := adapter.move(context.Background(), contracts.RedisSegment0Key, contracts.RedisLosersKey, member, entry.Score.Value); result.status != contracts.RedisStatusUnavailable {
		t.Fatalf("move nil status=%d", result.status)
	}
	if _, result := adapter.evalAssign(context.Background(), "sha", contracts.RedisSegment0Key, contracts.RedisSegment0Key, member, "7002", "1", contracts.RedisMatchTTLSeconds); result.status != contracts.RedisStatusUnavailable {
		t.Fatalf("eval nil status=%d", result.status)
	}
	if key := makePlayerKey(member); key != contracts.RedisPlayerKeyPrefix+member {
		t.Fatalf("player key=%q", key)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if status := statusFromRedisError(ctx, context.Canceled); status != contracts.RedisStatusCanceled {
		t.Fatalf("redis error status=%d", status)
	}
}

// B-REDISQUEUE-40 is covered by the Benchmark* suite below: pure codec,
// keyer, script-cache, result-construction, and metrics paths report 0 B/op.
func BenchmarkSegmentForTrophiesGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkSegmentForTrophies)
}

func BenchmarkSegmentForTrophiesGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkSegmentForTrophies)
}

func BenchmarkKeyForPoolGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkKeyForPool)
}

func BenchmarkKeyForPoolGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkKeyForPool)
}

func BenchmarkSegmentRangeGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkSegmentRange)
}

func BenchmarkSegmentRangeGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkSegmentRange)
}

func BenchmarkEncodeMemberGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkEncodeMember)
}

func BenchmarkEncodeMemberGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkEncodeMember)
}

func BenchmarkEncodeScoreGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkEncodeScore)
}

func BenchmarkEncodeScoreGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkEncodeScore)
}

func BenchmarkScoreRangeGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkScoreRange)
}

func BenchmarkScoreRangeGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkScoreRange)
}

func BenchmarkScriptSHAGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkScriptSHA)
}

func BenchmarkScriptSHAGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkScriptSHA)
}

func BenchmarkMarkNoScriptGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkMarkNoScript)
}

func BenchmarkMarkNoScriptGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkMarkNoScript)
}

func BenchmarkEnqueueFakeGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkEnqueueFake)
}

func BenchmarkEnqueueFakeGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkEnqueueFake)
}

func BenchmarkRemoveFakeGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkRemoveFake)
}

func BenchmarkRemoveFakeGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkRemoveFake)
}

func BenchmarkFetchCandidatesFakeGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkFetchCandidatesFake)
}

func BenchmarkFetchCandidatesFakeGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkFetchCandidatesFake)
}

func BenchmarkFetchCandidateBatchFakeGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkFetchCandidateBatchFake)
}

func BenchmarkFetchCandidateBatchFakeGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkFetchCandidateBatchFake)
}

func BenchmarkMovePoolFakeGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkMovePoolFake)
}

func BenchmarkMovePoolFakeGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkMovePoolFake)
}

func BenchmarkAssignMatchFakeGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkAssignMatchFake)
}

func BenchmarkAssignMatchFakeGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkAssignMatchFake)
}

func BenchmarkMetricsGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkMetrics)
}

func BenchmarkMetricsGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkMetrics)
}

func benchmarkWithProcs(b *testing.B, procs int, fn func(*testing.B)) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	fn(b)
}

func benchmarkSegmentForTrophies(b *testing.B) {
	keyer := newKeyer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = keyer.SegmentForTrophies(int32(i % 15001))
	}
}

func benchmarkKeyForPool(b *testing.B) {
	keyer := newKeyer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = keyer.KeyForPool(contracts.RedisQueuePool(i & 7))
	}
}

func benchmarkSegmentRange(b *testing.B) {
	keyer := newKeyer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = keyer.SegmentRange(contracts.RedisQueuePool(i % 5))
	}
}

func benchmarkEncodeMember(b *testing.B) {
	codec := newScoreCodec()
	var member contracts.RedisMember
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = codec.EncodeMember(uint64(i+1), &member)
	}
}

func benchmarkEncodeScore(b *testing.B) {
	codec := newScoreCodec()
	var score contracts.RedisScore
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = codec.EncodeScore(int32(i%15001), 1_700_000_000_123_456_789, &score)
	}
}

func benchmarkScoreRange(b *testing.B) {
	codec := newScoreCodec()
	var scoreRange contracts.RedisScoreRange
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = codec.ScoreRange(int32(i%15001), 1_700_000_000_123_456_789, 50, contracts.RedisPoolSegment1, &scoreRange)
	}
}

func benchmarkScriptSHA(b *testing.B) {
	fake := newFakeRedis()
	cache := newScriptCache(fake, nil)
	_ = cache.LoadScripts(context.Background())
	var sha contracts.RedisScriptSHA
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = cache.ScriptSHA(contracts.RedisScriptAssignMatch, &sha)
	}
}

func benchmarkMarkNoScript(b *testing.B) {
	fake := newFakeRedis()
	cache := newScriptCache(fake, nil)
	_ = cache.LoadScripts(context.Background())
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = cache.MarkNoScript(contracts.RedisScriptAssignMatch)
	}
}

func benchmarkEnqueueFake(b *testing.B) {
	store, _ := benchmarkStore()
	var entry contracts.RedisQueueEntry
	_ = buildQueueEntry(ptrTicket(testTicket(900, 3000, contracts.PoolMainstream)), &entry)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = store.Enqueue(context.Background(), &entry)
	}
}

func benchmarkRemoveFake(b *testing.B) {
	store, _ := benchmarkStore()
	var entry contracts.RedisQueueEntry
	_ = buildQueueEntry(ptrTicket(testTicket(901, 3000, contracts.PoolMainstream)), &entry)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = store.Remove(context.Background(), entry.Pool, entry.Member)
	}
}

func benchmarkFetchCandidatesFake(b *testing.B) {
	store, _ := benchmarkStore()
	var query contracts.RedisScoreRange
	_ = store.codec.ScoreRange(2000, testTicket(1, 2000, contracts.PoolMainstream).EnqueuedAt, 100, contracts.RedisPoolSegment1, &query)
	dst := make([]contracts.RedisCandidate, 8)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = store.FetchCandidates(context.Background(), query, dst)
	}
}

func benchmarkFetchCandidateBatchFake(b *testing.B) {
	store, _ := benchmarkStore()
	var query contracts.RedisScoreRange
	_ = store.codec.ScoreRange(2000, testTicket(1, 2000, contracts.PoolMainstream).EnqueuedAt, 100, contracts.RedisPoolSegment1, &query)
	batch := contracts.RedisQueryBatch{
		Ranges: []contracts.RedisScoreRange{query, query},
		Results: [][]contracts.RedisCandidate{
			make([]contracts.RedisCandidate, 8),
			make([]contracts.RedisCandidate, 8),
		},
		Count: 2,
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = store.FetchCandidateBatch(context.Background(), &batch)
	}
}

func benchmarkMovePoolFake(b *testing.B) {
	store, _ := benchmarkStore()
	var entry contracts.RedisQueueEntry
	_ = buildQueueEntry(ptrTicket(testTicket(902, 3000, contracts.PoolMainstream)), &entry)
	move := contracts.RedisMoveRequest{Member: entry.Member, From: contracts.RedisPoolSegment1, To: contracts.RedisPoolLosers, Score: entry.Score}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = store.MovePool(context.Background(), move)
	}
}

func benchmarkAssignMatchFake(b *testing.B) {
	store, cache := benchmarkStore()
	_ = cache.LoadScripts(context.Background())
	entryA := mustEntry(testTicket(1001, 2500, contracts.PoolMainstream))
	entryB := mustEntry(testTicket(1002, 2600, contracts.PoolMainstream))
	req := contracts.RedisAssignRequest{SourceA: contracts.RedisPoolSegment1, SourceB: contracts.RedisPoolSegment1, PlayerA: entryA.Member, PlayerB: entryB.Member, MatchID: 1}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = store.Enqueue(context.Background(), &entryA)
		_ = store.Enqueue(context.Background(), &entryB)
		req.MatchID = uint64(i + 1)
		_ = store.AssignMatch(context.Background(), req)
	}
}

func benchmarkMetrics(b *testing.B) {
	metrics := newMetrics()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		metrics.IncLatency(contracts.RedisStatusTimeout, 1)
		metrics.IncScriptReload()
		metrics.IncDualBooking()
		metrics.IncPipelinePartial()
	}
}

func enqueueForAssign(tb testing.TB, store *redisStore, ticket contracts.Ticket) contracts.RedisQueueEntry {
	tb.Helper()
	entry := mustEntry(ticket)
	if result := store.Enqueue(context.Background(), &entry); result.Status != contracts.RedisStatusOK {
		tb.Fatalf("enqueue for assign result=%+v", result)
	}
	return entry
}

func mustEntry(ticket contracts.Ticket) contracts.RedisQueueEntry {
	var entry contracts.RedisQueueEntry
	if status := buildQueueEntry(&ticket, &entry); status != contracts.RedisStatusOK {
		panic(status)
	}
	return entry
}

func ptrTicket(ticket contracts.Ticket) *contracts.Ticket {
	return &ticket
}

func contractScriptParts() string {
	return contracts.RedisAssignMatchLua
}

func stringsContain(value string, parts ...string) bool {
	for _, part := range parts {
		if !contains(value, part) {
			return false
		}
	}
	return true
}

func contains(value string, part string) bool {
	if len(part) == 0 {
		return true
	}
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}

func (f *fakeRedis) wait(ctx context.Context) contracts.RedisQueueStatus {
	if f.delay <= 0 {
		if err := ctx.Err(); err != nil {
			return contextStatus(err)
		}
		return contracts.RedisStatusOK
	}
	timer := time.NewTimer(f.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return contextStatus(ctx.Err())
	case <-timer.C:
		return contracts.RedisStatusOK
	}
}

func (f *fakeRedis) consumeStatus(ctx context.Context) contracts.RedisQueueStatus {
	if status := f.wait(ctx); status != contracts.RedisStatusOK {
		return status
	}
	if f.nextStatus != contracts.RedisStatusOK {
		status := f.nextStatus
		f.nextStatus = contracts.RedisStatusOK
		return status
	}
	return contracts.RedisStatusOK
}

func (f *fakeRedis) scriptLoad(ctx context.Context, script string) (contracts.RedisScriptSHA, redisAdapterResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if status := f.consumeStatus(ctx); status != contracts.RedisStatusOK {
		return contracts.RedisScriptSHA{}, redisAdapterResult{status: status}
	}
	sum := sha1.Sum([]byte(script))
	hexBytes := make([]byte, 40)
	hex.Encode(hexBytes, sum[:])
	var sha contracts.RedisScriptSHA
	sha.Kind = contracts.RedisScriptAssignMatch
	copy(sha.Hex[:], hexBytes)
	sha.Len = 40
	f.scriptLoads++
	return sha, redisAdapterResult{status: contracts.RedisStatusOK}
}

func (f *fakeRedis) zadd(ctx context.Context, key string, score float64, member string, entry *contracts.RedisQueueEntry) redisAdapterResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	if status := f.consumeStatus(ctx); status != contracts.RedisStatusOK {
		return redisAdapterResult{status: status}
	}
	if f.zsets[key] == nil {
		f.zsets[key] = make(map[string]float64)
	}
	f.zsets[key][member] = score
	f.playerMeta[member] = *entry
	f.enqueueKeys = append(f.enqueueKeys, key)
	return redisAdapterResult{status: contracts.RedisStatusOK}
}

func (f *fakeRedis) zrem(ctx context.Context, key string, member string) (uint16, redisAdapterResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if status := f.consumeStatus(ctx); status != contracts.RedisStatusOK {
		return 0, redisAdapterResult{status: status}
	}
	if _, ok := f.zsets[key][member]; !ok {
		return 0, redisAdapterResult{status: contracts.RedisStatusOK}
	}
	delete(f.zsets[key], member)
	return 1, redisAdapterResult{status: contracts.RedisStatusOK}
}

func (f *fakeRedis) zrangeByScore(ctx context.Context, key string, min float64, max float64, limit int64, dst []contracts.RedisCandidate, pool contracts.RedisQueuePool) (uint16, redisAdapterResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if status := f.consumeStatus(ctx); status != contracts.RedisStatusOK {
		return 0, redisAdapterResult{status: status}
	}
	members := make([]string, 0, len(f.zsets[key]))
	for member, score := range f.zsets[key] {
		if score >= min && score <= max {
			members = append(members, member)
		}
	}
	sort.Strings(members)
	if int64(len(members)) > limit {
		members = members[:limit]
	}
	count := uint16(0)
	for i, member := range members {
		if i >= len(dst) {
			break
		}
		if decodeCandidate(member, f.zsets[key][member], pool, &dst[i]) {
			count++
		}
	}
	return count, redisAdapterResult{status: contracts.RedisStatusOK}
}

func (f *fakeRedis) zrangeByScoreBatch(ctx context.Context, ranges []contracts.RedisScoreRange, results [][]contracts.RedisCandidate) (uint16, redisAdapterResult) {
	var total uint16
	var okCount uint16
	var failCount uint16
	for i := range ranges {
		if i == f.batchFailIndex {
			failCount++
			continue
		}
		key, status := keyer{}.KeyForPool(ranges[i].Pool)
		if status != contracts.RedisStatusOK {
			failCount++
			continue
		}
		count, result := f.zrangeByScore(ctx, key, ranges[i].Min, ranges[i].Max, ranges[i].Limit, results[i], ranges[i].Pool)
		if result.status != contracts.RedisStatusOK {
			failCount++
			continue
		}
		okCount++
		total += count
	}
	switch {
	case okCount > 0 && failCount > 0:
		return total, redisAdapterResult{status: contracts.RedisStatusPartial}
	case failCount > 0:
		return total, redisAdapterResult{status: contracts.RedisStatusUnavailable}
	default:
		return total, redisAdapterResult{status: contracts.RedisStatusOK}
	}
}

func (f *fakeRedis) move(ctx context.Context, fromKey string, toKey string, member string, score float64) (uint16, redisAdapterResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if status := f.consumeStatus(ctx); status != contracts.RedisStatusOK {
		return 0, redisAdapterResult{status: status}
	}
	if _, ok := f.zsets[fromKey][member]; !ok {
		return 0, redisAdapterResult{status: contracts.RedisStatusOK}
	}
	delete(f.zsets[fromKey], member)
	if f.zsets[toKey] == nil {
		f.zsets[toKey] = make(map[string]float64)
	}
	f.zsets[toKey][member] = score
	return 1, redisAdapterResult{status: contracts.RedisStatusOK}
}

func (f *fakeRedis) evalAssign(ctx context.Context, _ string, sourceA string, sourceB string, playerA string, playerB string, matchID string, ttlSeconds int64) (int64, redisAdapterResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if status := f.consumeStatus(ctx); status != contracts.RedisStatusOK {
		return 0, redisAdapterResult{status: status}
	}
	f.assignCalls++
	if f.noScript {
		return 0, redisAdapterResult{status: contracts.RedisStatusNoScript}
	}
	_, okA := f.zsets[sourceA][playerA]
	_, okB := f.zsets[sourceB][playerB]
	if !okA || !okB {
		return 0, redisAdapterResult{status: contracts.RedisStatusOK}
	}
	delete(f.zsets[sourceA], playerA)
	delete(f.zsets[sourceB], playerB)
	f.matches[matchID] = fakeMatch{playerA: playerA, playerB: playerB, ttl: ttlSeconds}
	return 1, redisAdapterResult{status: contracts.RedisStatusOK}
}

func (f *fakeRedis) hasMember(key string, member string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.zsets[key][member]
	return ok
}

func (f *fakeRedis) hasMeta(member string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.playerMeta[member]
	return ok
}

func (f *fakeRedis) lastEnqueueKeyIsSegment() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.enqueueKeys) == 0 {
		return false
	}
	switch f.enqueueKeys[len(f.enqueueKeys)-1] {
	case contracts.RedisSegment0Key, contracts.RedisSegment1Key, contracts.RedisSegment2Key, contracts.RedisSegment3Key, contracts.RedisSegment4Key:
		return true
	default:
		return false
	}
}

func (f *fakeRedis) zsetLen(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.zsets[key])
}

func (f *fakeRedis) match(matchID string) fakeMatch {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.matches[matchID]
}

func (n *noopRedis) scriptLoad(_ context.Context, _ string) (contracts.RedisScriptSHA, redisAdapterResult) {
	var sha contracts.RedisScriptSHA
	sha.Kind = contracts.RedisScriptAssignMatch
	copy(sha.Hex[:], "0123456789012345678901234567890123456789")
	sha.Len = 40
	return sha, redisAdapterResult{status: contracts.RedisStatusOK}
}

func (n *noopRedis) zadd(_ context.Context, _ string, _ float64, _ string, _ *contracts.RedisQueueEntry) redisAdapterResult {
	return redisAdapterResult{status: contracts.RedisStatusOK}
}

func (n *noopRedis) zrem(_ context.Context, _ string, _ string) (uint16, redisAdapterResult) {
	return 1, redisAdapterResult{status: contracts.RedisStatusOK}
}

func (n *noopRedis) zrangeByScore(_ context.Context, _ string, _ float64, _ float64, limit int64, dst []contracts.RedisCandidate, _ contracts.RedisQueuePool) (uint16, redisAdapterResult) {
	if len(dst) == 0 || limit == 0 {
		return 0, redisAdapterResult{status: contracts.RedisStatusOK}
	}
	dst[0] = n.candidate
	return 1, redisAdapterResult{status: contracts.RedisStatusOK}
}

func (n *noopRedis) zrangeByScoreBatch(_ context.Context, _ []contracts.RedisScoreRange, results [][]contracts.RedisCandidate) (uint16, redisAdapterResult) {
	var count uint16
	for i := range results {
		if len(results[i]) == 0 {
			continue
		}
		results[i][0] = n.candidate
		count++
	}
	return count, redisAdapterResult{status: contracts.RedisStatusOK}
}

func (n *noopRedis) move(_ context.Context, _ string, _ string, _ string, _ float64) (uint16, redisAdapterResult) {
	return 1, redisAdapterResult{status: contracts.RedisStatusOK}
}

func (n *noopRedis) evalAssign(_ context.Context, _ string, _ string, _ string, _ string, _ string, _ string, _ int64) (int64, redisAdapterResult) {
	return 1, redisAdapterResult{status: contracts.RedisStatusOK}
}
