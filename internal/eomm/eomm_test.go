package eomm

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"

	"matchpoint/contracts"
)

func TestConfigValidation(t *testing.T) {
	eng := mustEngine(t)
	if eng.config.TrophyWeight != contracts.EOMMDefaultTrophyWeight {
		t.Fatalf("default config not applied: %+v", eng.config)
	}
	bad := productionConfig()
	bad.RetentionWeight = 0.5
	if _, status := newEngine(bad); status != contracts.EOMMStatusWeightMismatch {
		t.Fatalf("B-EOMM-7 weight status=%d", status)
	}
	bad = productionConfig()
	bad.HighChurnThreshold = 2
	if _, status := newEngine(bad); status != contracts.EOMMStatusInvalidConfig {
		t.Fatalf("B-EOMM-7 threshold status=%d", status)
	}
}

func TestRouteTicketPriorityAndNoop(t *testing.T) {
	eng := mustEngine(t)
	entry := testEntry(10, 5000)
	entry.Ticket.ConsecLosses = -2
	entry.Ticket.ChurnRisk = 0.9
	entry.Ticket.MonetizationP = 0.9
	entry.Ticket.ConsecWins = 3
	var decision contracts.EOMMRouteDecision
	status := eng.RouteTicket(contracts.EOMMRoutingInput{Entry: entry, CurrentPool: contracts.RedisPoolSegment2, MainstreamPool: contracts.RedisPoolSegment2}, &decision)
	if status != contracts.EOMMStatusOK || !decision.Move || decision.To != contracts.RedisPoolLosers || decision.PoolTag != contracts.PoolLosers || decision.Reason != contracts.EOMMRouteLoser {
		t.Fatalf("B-EOMM-1/B-EOMM-4 decision=%+v status=%d", decision, status)
	}

	entry = testEntry(11, 5000)
	entry.Ticket.ConsecLosses = -1
	entry.Ticket.ChurnRisk = 0.9
	status = eng.RouteTicket(contracts.EOMMRoutingInput{Entry: entry, CurrentPool: contracts.RedisPoolSegment2, MainstreamPool: contracts.RedisPoolSegment2}, &decision)
	if status != contracts.EOMMStatusOK || decision.To != contracts.RedisPoolRetention || decision.PoolTag != contracts.PoolRetention || decision.Reason != contracts.EOMMRouteRetention {
		t.Fatalf("B-EOMM-2 decision=%+v status=%d", decision, status)
	}

	entry = testEntry(12, 5000)
	entry.Ticket.ConsecWins = 2
	entry.Ticket.MonetizationP = 0.9
	status = eng.RouteTicket(contracts.EOMMRoutingInput{Entry: entry, CurrentPool: contracts.RedisPoolSegment2, MainstreamPool: contracts.RedisPoolSegment2}, &decision)
	if status != contracts.EOMMStatusOK || decision.To != contracts.RedisPoolMonetize || decision.PoolTag != contracts.PoolMonetize || decision.Reason != contracts.EOMMRouteMonetize {
		t.Fatalf("B-EOMM-3 decision=%+v status=%d", decision, status)
	}

	entry = testEntry(13, 5000)
	status = eng.RouteTicket(contracts.EOMMRoutingInput{Entry: entry, CurrentPool: contracts.RedisPoolSegment2, MainstreamPool: contracts.RedisPoolSegment2}, &decision)
	if status != contracts.EOMMStatusOK || decision.Move || decision.To != contracts.RedisPoolSegment2 || decision.Reason != contracts.EOMMRouteMainstream {
		t.Fatalf("B-EOMM-5 decision=%+v status=%d", decision, status)
	}

	status = eng.RouteTicket(contracts.EOMMRoutingInput{Entry: entry, CurrentPool: contracts.RedisPoolRetention, MainstreamPool: contracts.RedisPoolSegment2}, &decision)
	if status != contracts.EOMMStatusOK || !decision.Move || decision.To != contracts.RedisPoolSegment2 || decision.Reason != contracts.EOMMRouteMainstream {
		t.Fatalf("B-EOMM-6 decision=%+v status=%d", decision, status)
	}
}

func TestLoserStarvationAndOutcomeEvacuation(t *testing.T) {
	eng := mustEngine(t)
	entry := testEntry(20, 3000)
	entry.Ticket.ConsecLosses = -2
	var decision contracts.EOMMRouteDecision
	status := eng.RouteTicket(contracts.EOMMRoutingInput{
		Entry:                 entry,
		CurrentPool:           contracts.RedisPoolLosers,
		MainstreamPool:        contracts.RedisPoolSegment1,
		WaitTicks:             contracts.EOMMLoserStarvationTicks + 1,
		OtherLoserPoolPlayers: 1,
	}, &decision)
	if status != contracts.EOMMStatusOK || !decision.Move || decision.To != contracts.RedisPoolSegment1 || decision.Reason != contracts.EOMMRouteStarvationEvacuate {
		t.Fatalf("B-EOMM-8/B-EOMM-9 decision=%+v status=%d", decision, status)
	}

	for _, tc := range []struct {
		source contracts.RedisQueuePool
		won    bool
		reason contracts.EOMMRouteReason
		move   bool
	}{
		{contracts.RedisPoolLosers, true, contracts.EOMMRouteWinEvacuate, true},
		{contracts.RedisPoolRetention, true, contracts.EOMMRouteWinEvacuate, true},
		{contracts.RedisPoolMonetize, false, contracts.EOMMRouteCompleteEvacuate, true},
		{contracts.RedisPoolLosers, false, contracts.EOMMRouteWinEvacuate, false},
	} {
		status = eng.RouteMatchOutcome(contracts.EOMMMatchOutcome{Entry: entry, SourcePool: tc.source, MainstreamPool: contracts.RedisPoolSegment1, Won: tc.won}, &decision)
		if status != contracts.EOMMStatusOK || decision.Move != tc.move || decision.Reason != tc.reason {
			t.Fatalf("B-EOMM-10/B-EOMM-11/B-EOMM-12 source=%d decision=%+v status=%d", tc.source, decision, status)
		}
	}
}

func TestApplyRouteStatusMapping(t *testing.T) {
	eng := mustEngine(t)
	store := &fakeStore{}
	decision := contracts.EOMMRouteDecision{Move: false, Member: member(1), From: contracts.RedisPoolSegment0, To: contracts.RedisPoolSegment0}
	if status := eng.ApplyRoute(context.Background(), store, decision); status != contracts.EOMMStatusNoop || store.moves.Load() != 0 {
		t.Fatalf("B-EOMM-13 status=%d moves=%d", status, store.moves.Load())
	}
	decision.Move = true
	decision.To = contracts.RedisPoolRetention
	decision.Score = testEntry(1, 1000).Score
	if status := eng.ApplyRoute(context.Background(), store, decision); status != contracts.EOMMStatusOK || store.lastMove.To != contracts.RedisPoolRetention || store.lastMove.Member.PlayerID != 1 {
		t.Fatalf("B-EOMM-14 status=%d move=%+v", status, store.lastMove)
	}
	store.status = contracts.RedisStatusTimeout
	if status := eng.ApplyRoute(context.Background(), store, decision); status != contracts.EOMMStatusRedisTimeout {
		t.Fatalf("B-EOMM-15 timeout status=%d", status)
	}
	store.status = contracts.RedisStatusPartial
	if status := eng.ApplyRoute(context.Background(), store, decision); status != contracts.EOMMStatusRedisUnavailable {
		t.Fatalf("B-EOMM-15 partial status=%d", status)
	}
	store.status = contracts.RedisStatusCanceled
	if status := eng.ApplyRoute(context.Background(), store, decision); status != contracts.EOMMStatusCanceled {
		t.Fatalf("B-EOMM-15 canceled status=%d", status)
	}
}

func TestScoreCandidateBreakdownAndTargets(t *testing.T) {
	eng := mustEngine(t)
	input := contextFor(30, 31, 5000, 4900, contracts.RedisPoolRetention)
	input.Anchor.ChurnRisk = 0.9
	var breakdown contracts.EOMMScoreBreakdown
	if status := eng.ScoreBreakdown(input, &breakdown); status != contracts.EOMMStatusOK || breakdown.RetentionModifier >= 0 || breakdown.PredictedWinP != contracts.EOMMRetentionTargetWinP {
		t.Fatalf("B-EOMM-16/B-EOMM-18/B-EOMM-19 breakdown=%+v status=%d", breakdown, status)
	}
	var score contracts.MatchCandidateScore
	if status := eng.ScoreCandidate(input, &score); status != contracts.MatchCoreStatusOK || score.Decision != contracts.MatchCandidateReplaceBest || score.Fitness != breakdown.Total {
		t.Fatalf("B-EOMM-17 score=%+v status=%d", score, status)
	}

	monetize := contextFor(32, 33, 5000, 5100, contracts.RedisPoolMonetize)
	monetize.Anchor.DeckVector = [8]float32{1}
	monetize.Candidate.Ticket.DeckVector = [8]float32{0, 1}
	if status := eng.ScoreBreakdown(monetize, &breakdown); status != contracts.EOMMStatusOK || breakdown.RetentionModifier <= 0 || breakdown.PredictedWinP != contracts.EOMMMonetizeTargetWinP {
		t.Fatalf("B-EOMM-20/B-EOMM-21/B-EOMM-22 breakdown=%+v status=%d", breakdown, status)
	}

	similar := contextFor(34, 35, 5000, 5005, contracts.RedisPoolSegment2)
	similar.Anchor.DeckVector = [8]float32{1}
	similar.Candidate.Ticket.DeckVector = [8]float32{1}
	if status := eng.ScoreBreakdown(similar, &breakdown); status != contracts.EOMMStatusOK || breakdown.VectorDistance != 0 || breakdown.TrophyPenalty <= 0 || breakdown.TrophyPenalty > 1 {
		t.Fatalf("B-EOMM-23 breakdown=%+v status=%d", breakdown, status)
	}
}

func TestScoreCandidateRejectsInvalidPairs(t *testing.T) {
	eng := mustEngine(t)
	input := contextFor(40, 40, 1000, 1000, contracts.RedisPoolSegment0)
	var score contracts.MatchCandidateScore
	if status := eng.ScoreCandidate(input, &score); status != contracts.MatchCoreStatusInvalidCandidate || score.Decision != contracts.MatchCandidateReject {
		t.Fatalf("B-EOMM-28 self status=%d score=%+v", status, score)
	}
	input = contextFor(40, 41, 1000, 2000, contracts.RedisPoolSegment0)
	input.ToleranceTrophies = 50
	if status := eng.ScoreCandidate(input, &score); status != contracts.MatchCoreStatusInvalidCandidate || score.TrophyDelta != 1000 {
		t.Fatalf("B-EOMM-28 delta status=%d score=%+v", status, score)
	}
}

func TestSpikeDetectorWindowAndAlert(t *testing.T) {
	eng := mustEngine(t)
	var state contracts.EOMMSpikeState
	var alert contracts.EOMMChurnAlertEvent
	for i := 0; i < int(contracts.EOMMSpikeWindowTicks)-1; i++ {
		status := eng.RecordOutcome(contracts.EOMMSpikeInput{PlayerID: 50, Won: false, PreviousChurnRisk: 0.2, CurrentChurnRisk: 0.8}, &state, &alert)
		if status != contracts.EOMMStatusNoop || alert.PlayerID != 0 {
			t.Fatalf("B-EOMM-25 status=%d alert=%+v", status, alert)
		}
	}
	status := eng.RecordOutcome(contracts.EOMMSpikeInput{PlayerID: 50, Won: false, PreviousChurnRisk: 0.2, CurrentChurnRisk: 0.8}, &state, &alert)
	if status != contracts.EOMMStatusOK || alert.PlayerID != 50 || alert.PoolTag != contracts.PoolRetention || alert.RollingWinRate >= contracts.EOMMSpikeWinRateThreshold {
		t.Fatalf("B-EOMM-24 status=%d state=%+v alert=%+v", status, state, alert)
	}
	status = eng.RecordOutcome(contracts.EOMMSpikeInput{PlayerID: 50, Won: true, PreviousChurnRisk: 0.8, CurrentChurnRisk: 0.9}, &state, &alert)
	if status != contracts.EOMMStatusNoop || state.Count != contracts.EOMMSpikeWindowTicks || state.Wins != 1 {
		t.Fatalf("B-EOMM-26 status=%d state=%+v alert=%+v", status, state, alert)
	}
	if status := eng.RecordOutcome(contracts.EOMMSpikeInput{}, &state, &alert); status != contracts.EOMMStatusInvalidTicket {
		t.Fatalf("B-EOMM-27 invalid status=%d", status)
	}
}

func BenchmarkRouteTicketGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkRouteTicket)
}

func BenchmarkRouteTicketGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkRouteTicket)
}

func BenchmarkRouteMatchOutcomeGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkRouteMatchOutcome)
}

func BenchmarkRouteMatchOutcomeGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkRouteMatchOutcome)
}

func BenchmarkApplyRouteGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkApplyRoute)
}

func BenchmarkApplyRouteGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkApplyRoute)
}

func BenchmarkScoreBreakdownGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkScoreBreakdown)
}

func BenchmarkScoreBreakdownGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkScoreBreakdown)
}

func BenchmarkScoreCandidateGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkScoreCandidate)
}

func BenchmarkScoreCandidateGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkScoreCandidate)
}

func BenchmarkRecordOutcomeGOMAXPROCS1(b *testing.B) {
	benchmarkWithProcs(b, 1, benchmarkRecordOutcome)
}

func BenchmarkRecordOutcomeGOMAXPROCSCPU(b *testing.B) {
	benchmarkWithProcs(b, runtime.NumCPU(), benchmarkRecordOutcome)
}

func benchmarkWithProcs(b *testing.B, procs int, fn func(*testing.B)) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	fn(b)
}

func benchmarkRouteTicket(b *testing.B) {
	eng := mustEngineB(b)
	input := contracts.EOMMRoutingInput{Entry: testEntry(1, 5000), CurrentPool: contracts.RedisPoolSegment2, MainstreamPool: contracts.RedisPoolSegment2}
	var out contracts.EOMMRouteDecision
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = eng.RouteTicket(input, &out)
	}
}

func benchmarkRouteMatchOutcome(b *testing.B) {
	eng := mustEngineB(b)
	input := contracts.EOMMMatchOutcome{Entry: testEntry(1, 5000), SourcePool: contracts.RedisPoolMonetize, MainstreamPool: contracts.RedisPoolSegment2}
	var out contracts.EOMMRouteDecision
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = eng.RouteMatchOutcome(input, &out)
	}
}

func benchmarkApplyRoute(b *testing.B) {
	eng := mustEngineB(b)
	store := &fakeStore{}
	decision := contracts.EOMMRouteDecision{Move: true, Member: member(1), From: contracts.RedisPoolSegment2, To: contracts.RedisPoolRetention, Score: testEntry(1, 5000).Score}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = eng.ApplyRoute(context.Background(), store, decision)
	}
}

func benchmarkScoreBreakdown(b *testing.B) {
	eng := mustEngineB(b)
	input := contextFor(1, 2, 5000, 4900, contracts.RedisPoolRetention)
	var out contracts.EOMMScoreBreakdown
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = eng.ScoreBreakdown(input, &out)
	}
}

func benchmarkScoreCandidate(b *testing.B) {
	eng := mustEngineB(b)
	input := contextFor(1, 2, 5000, 4900, contracts.RedisPoolRetention)
	var out contracts.MatchCandidateScore
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = eng.ScoreCandidate(input, &out)
	}
}

func benchmarkRecordOutcome(b *testing.B) {
	eng := mustEngineB(b)
	state := contracts.EOMMSpikeState{PlayerID: 1}
	var out contracts.EOMMChurnAlertEvent
	input := contracts.EOMMSpikeInput{PlayerID: 1, Won: false, PreviousChurnRisk: 0.2, CurrentChurnRisk: 0.8}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = eng.RecordOutcome(input, &state, &out)
	}
}

type fakeStore struct {
	status   contracts.RedisQueueStatus
	lastMove contracts.RedisMoveRequest
	moves    atomic.Uint64
}

func (s *fakeStore) Enqueue(context.Context, *contracts.RedisQueueEntry) contracts.RedisOperationResult {
	return contracts.RedisOperationResult{Status: contracts.RedisStatusOK}
}

func (s *fakeStore) Remove(context.Context, contracts.RedisQueuePool, contracts.RedisMember) contracts.RedisOperationResult {
	return contracts.RedisOperationResult{Status: contracts.RedisStatusOK}
}

func (s *fakeStore) FetchCandidates(context.Context, contracts.RedisScoreRange, []contracts.RedisCandidate) contracts.RedisOperationResult {
	return contracts.RedisOperationResult{Status: contracts.RedisStatusEmpty}
}

func (s *fakeStore) FetchCandidateBatch(context.Context, *contracts.RedisQueryBatch) contracts.RedisOperationResult {
	return contracts.RedisOperationResult{Status: contracts.RedisStatusEmpty}
}

func (s *fakeStore) MovePool(_ context.Context, move contracts.RedisMoveRequest) contracts.RedisOperationResult {
	s.moves.Add(1)
	s.lastMove = move
	status := s.status
	if status == 0 {
		status = contracts.RedisStatusOK
	}
	return contracts.RedisOperationResult{Status: status, Pool: move.To, Count: 1}
}

func (s *fakeStore) AssignMatch(context.Context, contracts.RedisAssignRequest) contracts.RedisAssignResult {
	return contracts.RedisAssignResult{Status: contracts.RedisStatusOK}
}

func mustEngine(t *testing.T) *engine {
	t.Helper()
	eng, status := newEngine(productionConfig())
	if status != contracts.EOMMStatusOK {
		t.Fatalf("newEngine status=%d", status)
	}
	return eng
}

func mustEngineB(b *testing.B) *engine {
	b.Helper()
	eng, status := newEngine(productionConfig())
	if status != contracts.EOMMStatusOK {
		b.Fatalf("newEngine status=%d", status)
	}
	return eng
}

func testEntry(playerID uint64, trophies int32) contracts.RedisQueueEntry {
	return contracts.RedisQueueEntry{
		Ticket: contracts.Ticket{
			PlayerID:      playerID,
			Trophies:      trophies,
			DeckVector:    [8]float32{1},
			ChurnRisk:     0.1,
			MonetizationP: 0.1,
			PoolTag:       contracts.PoolMainstream,
		},
		Member: member(playerID),
		Score: contracts.RedisScore{
			Value:    float64(int64(trophies) * contracts.RedisScoreTrophyScale),
			Trophies: trophies,
		},
		Pool: poolForTrophies(trophies),
	}
}

func contextFor(a uint64, b uint64, trophiesA int32, trophiesB int32, pool contracts.RedisQueuePool) contracts.MatchCandidateContext {
	anchor := testEntry(a, trophiesA)
	candidate := testEntry(b, trophiesB)
	anchor.Pool = pool
	candidate.Pool = pool
	return contracts.MatchCandidateContext{
		Anchor:            anchor.Ticket,
		Candidate:         candidate,
		AnchorPool:        pool,
		CandidatePool:     pool,
		ToleranceTrophies: 2000,
	}
}

func member(playerID uint64) contracts.RedisMember {
	return contracts.RedisMember{PlayerID: playerID, Bytes: [20]byte{'1'}, Len: 1}
}

func poolForTrophies(trophies int32) contracts.RedisQueuePool {
	switch {
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
