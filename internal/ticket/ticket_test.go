package ticket

import (
	"bytes"
	"math"
	"runtime"
	"sync/atomic"
	"testing"
	"unsafe"
)

func TestTicketLayoutIsOneCacheLine(t *testing.T) {
	if size := unsafe.Sizeof(Ticket{}); size != 64 {
		t.Fatalf("Ticket size = %d, want 64", size)
	}
	assertOffset(t, "PlayerID", unsafe.Offsetof(Ticket{}.PlayerID), 0)
	assertOffset(t, "EnqueuedAt", unsafe.Offsetof(Ticket{}.EnqueuedAt), 8)
	assertOffset(t, "DeckVector", unsafe.Offsetof(Ticket{}.DeckVector), 16)
	assertOffset(t, "Trophies", unsafe.Offsetof(Ticket{}.Trophies), 48)
	assertOffset(t, "ChurnRisk", unsafe.Offsetof(Ticket{}.ChurnRisk), 52)
	assertOffset(t, "MonetizationP", unsafe.Offsetof(Ticket{}.MonetizationP), 56)
	assertOffset(t, "ConsecLosses", unsafe.Offsetof(Ticket{}.ConsecLosses), 60)
	assertOffset(t, "ConsecWins", unsafe.Offsetof(Ticket{}.ConsecWins), 61)
	assertOffset(t, "PoolTag", unsafe.Offsetof(Ticket{}.PoolTag), 62)
}

func TestTicketPoolLifecycle(t *testing.T) {
	pool := newTicketPool()
	ticket := pool.AcquireTicket()
	if ticket == nil {
		t.Fatal("AcquireTicket returned nil")
	}
	assertZeroTicket(t, ticket)

	fillTicket(ticket)
	pool.ResetTicket(ticket)
	assertZeroTicket(t, ticket)

	fillTicket(ticket)
	pool.ReleaseTicket(ticket)
	reused := pool.AcquireTicket()
	assertZeroTicket(t, reused)

	pool.ReleaseTicket(nil)
}

func TestDecodeRejectsMalformedPayload(t *testing.T) {
	decoder := payloadDecoder{}
	payload := QueueJoinPayload{PlayerID: 99}
	code := decoder.DecodeQueueJoin([]byte{0xff}, WireFormatMessagePack, &payload)
	if code != ErrMalformedPayload {
		t.Fatalf("DecodeQueueJoin malformed code = %v, want %v", code, ErrMalformedPayload)
	}
	if payload.PlayerID != 99 {
		t.Fatalf("DecodeQueueJoin mutated payload on malformed input")
	}
}

func TestParseTimeoutReleasesNoTicketAndPublishesNothing(t *testing.T) {
	env := newParseEnv()
	env.clock.step = int64(51_000)
	ticket, ack := env.processor.ParseTicket(validMsgpackPayload(42), WireFormatMessagePack)
	if ticket != nil {
		t.Fatalf("ParseTicket returned ticket on timeout")
	}
	if ack.Status != QueueStatusRejected || ack.ErrorCode != ErrParseTimeout {
		t.Fatalf("ack = %+v, want timeout rejection", ack)
	}
	if env.sink.writeCount.Load() != 0 {
		t.Fatalf("ring buffer write attempted on timeout")
	}
}

func TestParseRejectsZeroPlayerID(t *testing.T) {
	env := newParseEnv()
	payload := validPayload(0)
	_, ack := env.processor.ParseTicket(msgpackPayload(payload), WireFormatMessagePack)
	if ack.Status != QueueStatusRejected || ack.ErrorCode == IntakeOK {
		t.Fatalf("ack = %+v, want non-OK rejection", ack)
	}
	if env.sink.writeCount.Load() != 0 {
		t.Fatalf("ring buffer write attempted for zero player")
	}
}

func TestParseRejectsMissingOrInvalidSession(t *testing.T) {
	env := newParseEnv()
	payload := validPayload(42)
	payload.token = ""
	_, ack := env.processor.ParseTicket(msgpackPayload(payload), WireFormatMessagePack)
	if ack.Status != QueueStatusRejected || ack.ErrorCode != ErrUnauthorized {
		t.Fatalf("empty wire token ack = %+v, want unauthorized", ack)
	}

	env = newParseEnv()
	env.auth.ok = false
	_, ack = env.processor.ParseTicket(validMsgpackPayload(42), WireFormatMessagePack)
	if ack.Status != QueueStatusRejected || ack.ErrorCode != ErrUnauthorized {
		t.Fatalf("invalid auth ack = %+v, want unauthorized", ack)
	}
	if env.pool.acquired.Load() != 0 || env.sink.writeCount.Load() != 0 {
		t.Fatalf("ticket acquired or published on unauthorized payload")
	}
}

func TestParseRejectsTrophiesOutsideMatchSpecBounds(t *testing.T) {
	for _, trophies := range []int32{-1, 15001} {
		env := newParseEnv()
		payload := validPayload(42)
		payload.trophies = trophies
		payload.tier = tierForTrophies(max(trophies, 0))
		_, ack := env.processor.ParseTicket(msgpackPayload(payload), WireFormatMessagePack)
		if ack.Status != QueueStatusRejected || ack.ErrorCode != ErrInvalidTrophies {
			t.Fatalf("trophies %d ack = %+v, want invalid trophies", trophies, ack)
		}
	}
}

func TestParseRejectsTierMismatch(t *testing.T) {
	env := newParseEnv()
	payload := validPayload(42)
	payload.trophies = 3000
	payload.tier = 2
	_, ack := env.processor.ParseTicket(msgpackPayload(payload), WireFormatMessagePack)
	if ack.Status != QueueStatusRejected || ack.ErrorCode != ErrTierMismatch {
		t.Fatalf("ack = %+v, want tier mismatch", ack)
	}
}

func TestDeckValidatorRejectsInvalidDecks(t *testing.T) {
	validator := newDeckValidator()
	tests := []struct {
		name string
		deck [8]uint8
	}{
		{name: "duplicate", deck: [8]uint8{0, 0, 8, 9, 14, 20, 26, 32}},
		{name: "out of range", deck: [8]uint8{0, 8, 14, 20, 26, 32, 38, 48}},
		{name: "too heavy", deck: [8]uint8{0, 1, 3, 4, 7, 8, 11, 13}},
		{name: "single archetype", deck: [8]uint8{0, 1, 2, 3, 4, 5, 6, 7}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out [8]float32
			if code := validator.BuildDeckVector(tc.deck, &out); code != ErrInvalidDeck {
				t.Fatalf("BuildDeckVector code = %v, want ErrInvalidDeck", code)
			}
		})
	}
}

func TestDeckValidatorNormalizesValidDeck(t *testing.T) {
	validator := newDeckValidator()
	deck := [8]uint8{0, 8, 14, 20, 26, 32, 38, 43}
	var out [8]float32
	if code := validator.BuildDeckVector(deck, &out); code != IntakeOK {
		t.Fatalf("BuildDeckVector code = %v, want OK", code)
	}
	if got := magnitude(out); math.Abs(float64(got-1)) > 0.0001 {
		t.Fatalf("vector magnitude = %f, want 1", got)
	}
	if out[0] <= 0 || out[1] <= 0 || out[2] <= 0 || out[3] <= 0 || out[4] <= 0 || out[5] <= 0 || out[6] <= 0 || out[7] <= 0 {
		t.Fatalf("expected primary/secondary contributions in vector: %+v", out)
	}
}

func TestDeckValidatorRejectsZeroVector(t *testing.T) {
	var cards [48]CardDef
	for i := range cards {
		cards[i] = CardDef{PrimaryArchetype: 9, Elixir: 4}
	}
	var out [8]float32
	if code := buildDeckVector([8]uint8{0, 8, 16, 24, 32, 40, 1, 9}, &out, &cards); code != ErrZeroVector {
		t.Fatalf("BuildDeckVector code = %v, want ErrZeroVector", code)
	}
}

func TestParseUsesFallbackSignalsWhenStoreUnavailable(t *testing.T) {
	env := newParseEnv()
	env.signals.unavailable = true
	payload := validPayload(42)
	payload.losses = -3
	payload.wins = 2
	ticket, ack := env.processor.ParseTicket(msgpackPayload(payload), WireFormatMessagePack)
	if ack.Status != QueueStatusQueued || ticket == nil {
		t.Fatalf("ack = %+v ticket=%v, want queued", ack, ticket)
	}
	if ticket.ConsecLosses != -3 || ticket.ConsecWins != 2 || ticket.ChurnRisk != 0.1 || ticket.MonetizationP != 0.1 {
		t.Fatalf("ticket signals = %+v, want fallback streaks and default risks", ticket)
	}
}

func TestParseOverridesClientSignalsWithServerSignals(t *testing.T) {
	env := newParseEnv()
	env.signals.signals = DerivedSignals{ConsecLosses: -4, ConsecWins: 7, ChurnRisk: 0.8, MonetizationP: 0.2}
	payload := validPayload(42)
	payload.losses = -1
	payload.wins = 1
	ticket, ack := env.processor.ParseTicket(msgpackPayload(payload), WireFormatMessagePack)
	if ack.Status != QueueStatusQueued || ticket == nil {
		t.Fatalf("ack = %+v ticket=%v, want queued", ack, ticket)
	}
	if ticket.ConsecLosses != -4 || ticket.ConsecWins != 7 || ticket.ChurnRisk != 0.8 || ticket.MonetizationP != 0.2 {
		t.Fatalf("ticket signals = %+v, want server override", ticket)
	}
}

func TestParseRejectsInvalidDerivedSignals(t *testing.T) {
	tests := []DerivedSignals{
		{ConsecLosses: 1, ConsecWins: 0, ChurnRisk: 0.1, MonetizationP: 0.1},
		{ConsecLosses: 0, ConsecWins: -1, ChurnRisk: 0.1, MonetizationP: 0.1},
		{ConsecLosses: 0, ConsecWins: 0, ChurnRisk: -0.1, MonetizationP: 0.1},
		{ConsecLosses: 0, ConsecWins: 0, ChurnRisk: 0.1, MonetizationP: 1.1},
	}
	for _, signals := range tests {
		env := newParseEnv()
		env.signals.signals = signals
		_, ack := env.processor.ParseTicket(validMsgpackPayload(42), WireFormatMessagePack)
		if ack.Status != QueueStatusRejected {
			t.Fatalf("signals %+v ack = %+v, want rejection", signals, ack)
		}
		if env.sink.writeCount.Load() != 0 {
			t.Fatalf("published invalid signals %+v", signals)
		}
	}
}

func TestParsePopulatesTicketAndPublishesOnce(t *testing.T) {
	env := newParseEnv()
	env.clock.now = 1234
	ticket, ack := env.processor.ParseTicket(validMsgpackPayload(42), WireFormatMessagePack)
	if ack.Status != QueueStatusQueued || ack.ErrorCode != IntakeOK {
		t.Fatalf("ack = %+v, want queued OK", ack)
	}
	if ticket == nil {
		t.Fatal("ParseTicket returned nil ticket")
	}
	if ticket.PlayerID != 42 || ticket.EnqueuedAt != 1234 || ticket.Trophies != 4200 || ticket.PoolTag != PoolMainstream {
		t.Fatalf("ticket = %+v, not populated from server-owned inputs", ticket)
	}
	if ticket.ChurnRisk != 0.25 || ticket.MonetizationP != 0.75 {
		t.Fatalf("ticket risks = %f/%f, want server-derived", ticket.ChurnRisk, ticket.MonetizationP)
	}
	if env.sink.writeCount.Load() != 1 || env.sink.last.Load() != ticket.PlayerID%env.sink.shards {
		t.Fatalf("sink writes=%d shard=%d, want one write to player modulo shard", env.sink.writeCount.Load(), env.sink.last.Load())
	}
	if ack.QueueDepth != env.sink.depth || ack.EstWaitMS != env.estimator.wait {
		t.Fatalf("ack = %+v, want depth/wait from boundaries", ack)
	}
}

func TestParseHandlesRingBufferFullAndReleasesTicket(t *testing.T) {
	env := newParseEnv()
	env.sink.code = ErrRingBufferFull
	_, ack := env.processor.ParseTicket(validMsgpackPayload(42), WireFormatMessagePack)
	if ack.Status != QueueStatusRejected || ack.ErrorCode != ErrRingBufferFull || ack.EstWaitMS != env.estimator.wait {
		t.Fatalf("ack = %+v, want ring full rejection with estimate", ack)
	}
	if env.pool.released.Load() != 1 {
		t.Fatalf("released = %d, want 1", env.pool.released.Load())
	}
}

func TestParseRejectsDuplicateActiveTicket(t *testing.T) {
	env := newParseEnv()
	if ticket, ack := env.processor.ParseTicket(validMsgpackPayload(42), WireFormatMessagePack); ticket == nil || ack.Status != QueueStatusQueued {
		t.Fatalf("first join ticket=%v ack=%+v, want queued", ticket, ack)
	}
	ticket, ack := env.processor.ParseTicket(validMsgpackPayload(42), WireFormatMessagePack)
	if ticket != nil || ack.Status != QueueStatusAlreadyQueued || ack.ErrorCode != IntakeOK {
		t.Fatalf("duplicate ticket=%v ack=%+v, want already queued", ticket, ack)
	}
	if env.sink.writeCount.Load() != 1 {
		t.Fatalf("write count = %d, want no duplicate publication", env.sink.writeCount.Load())
	}
}

func TestDecodersAcceptMessagePackAndJSONAndIgnoreClientRisk(t *testing.T) {
	decoder := payloadDecoder{}
	payload := validPayload(42)
	payloadBytes := msgpackPayloadWithRisk(payload)
	var mp QueueJoinPayload
	if code := decoder.DecodeQueueJoin(payloadBytes, WireFormatMessagePack, &mp); code != IntakeOK {
		t.Fatalf("MessagePack DecodeQueueJoin code = %v, want OK", code)
	}

	jsonPayload := []byte(`{"player_id":42,"session_token":"valid-token","trophies":4200,"tier":3,"card_ids":[0,8,14,20,26,32,38,43],"consec_losses":-1,"consec_wins":2,"churn_risk":1.0,"monetization_p":1.0}`)
	var js QueueJoinPayload
	if code := decoder.DecodeQueueJoin(jsonPayload, WireFormatJSON, &js); code != IntakeOK {
		t.Fatalf("JSON DecodeQueueJoin code = %v, want OK", code)
	}
	if mp != js {
		t.Fatalf("MessagePack payload %+v != JSON payload %+v", mp, js)
	}
}

func TestSuccessfulParseTransfersTicketOwnership(t *testing.T) {
	env := newParseEnv()
	ticket, ack := env.processor.ParseTicket(validMsgpackPayload(42), WireFormatMessagePack)
	if ack.Status != QueueStatusQueued || ticket == nil {
		t.Fatalf("ticket=%v ack=%+v, want queued", ticket, ack)
	}
	if env.pool.released.Load() != 0 {
		t.Fatalf("released = %d, want successful publication to transfer ownership", env.pool.released.Load())
	}
}

func BenchmarkAcquireTicketGOMAXPROCS1(b *testing.B) {
	benchmarkAcquireTicket(b, 1)
}

func BenchmarkAcquireTicketGOMAXPROCSN(b *testing.B) {
	benchmarkAcquireTicket(b, runtime.NumCPU())
}

func BenchmarkResetTicketGOMAXPROCS1(b *testing.B) {
	benchmarkResetTicket(b, 1)
}

func BenchmarkResetTicketGOMAXPROCSN(b *testing.B) {
	benchmarkResetTicket(b, runtime.NumCPU())
}

func BenchmarkReleaseTicketGOMAXPROCS1(b *testing.B) {
	benchmarkReleaseTicket(b, 1)
}

func BenchmarkReleaseTicketGOMAXPROCSN(b *testing.B) {
	benchmarkReleaseTicket(b, runtime.NumCPU())
}

func BenchmarkDecodeQueueJoinMessagePackGOMAXPROCS1(b *testing.B) {
	benchmarkDecodeQueueJoin(b, 1, WireFormatMessagePack)
}

func BenchmarkDecodeQueueJoinMessagePackGOMAXPROCSN(b *testing.B) {
	benchmarkDecodeQueueJoin(b, runtime.NumCPU(), WireFormatMessagePack)
}

func BenchmarkDecodeQueueJoinJSONGOMAXPROCS1(b *testing.B) {
	benchmarkDecodeQueueJoin(b, 1, WireFormatJSON)
}

func BenchmarkDecodeQueueJoinJSONGOMAXPROCSN(b *testing.B) {
	benchmarkDecodeQueueJoin(b, runtime.NumCPU(), WireFormatJSON)
}

func BenchmarkValidateSessionGOMAXPROCS1(b *testing.B) {
	benchmarkValidateSession(b, 1)
}

func BenchmarkValidateSessionGOMAXPROCSN(b *testing.B) {
	benchmarkValidateSession(b, runtime.NumCPU())
}

func BenchmarkLoadSignalsGOMAXPROCS1(b *testing.B) {
	benchmarkLoadSignals(b, 1)
}

func BenchmarkLoadSignalsGOMAXPROCSN(b *testing.B) {
	benchmarkLoadSignals(b, runtime.NumCPU())
}

func BenchmarkBuildDeckVectorGOMAXPROCS1(b *testing.B) {
	benchmarkBuildDeckVector(b, 1)
}

func BenchmarkBuildDeckVectorGOMAXPROCSN(b *testing.B) {
	benchmarkBuildDeckVector(b, runtime.NumCPU())
}

func BenchmarkWriteTicketGOMAXPROCS1(b *testing.B) {
	benchmarkWriteTicket(b, 1)
}

func BenchmarkWriteTicketGOMAXPROCSN(b *testing.B) {
	benchmarkWriteTicket(b, runtime.NumCPU())
}

func BenchmarkShardDepthGOMAXPROCS1(b *testing.B) {
	benchmarkShardDepth(b, 1)
}

func BenchmarkShardDepthGOMAXPROCSN(b *testing.B) {
	benchmarkShardDepth(b, runtime.NumCPU())
}

func BenchmarkEstimateWaitMSGOMAXPROCS1(b *testing.B) {
	benchmarkEstimateWaitMS(b, 1)
}

func BenchmarkEstimateWaitMSGOMAXPROCSN(b *testing.B) {
	benchmarkEstimateWaitMS(b, runtime.NumCPU())
}

func BenchmarkNowUnixNanoGOMAXPROCS1(b *testing.B) {
	benchmarkNowUnixNano(b, 1)
}

func BenchmarkNowUnixNanoGOMAXPROCSN(b *testing.B) {
	benchmarkNowUnixNano(b, runtime.NumCPU())
}

func BenchmarkParseTicketMessagePackGOMAXPROCS1(b *testing.B) {
	benchmarkParseTicket(b, 1, WireFormatMessagePack)
}

func BenchmarkParseTicketMessagePackGOMAXPROCSN(b *testing.B) {
	benchmarkParseTicket(b, runtime.NumCPU(), WireFormatMessagePack)
}

func BenchmarkParseTicketJSONGOMAXPROCS1(b *testing.B) {
	benchmarkParseTicket(b, 1, WireFormatJSON)
}

func BenchmarkParseTicketJSONGOMAXPROCSN(b *testing.B) {
	benchmarkParseTicket(b, runtime.NumCPU(), WireFormatJSON)
}

func benchmarkAcquireTicket(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	pool := newTicketPool()
	for i := 0; i < 8192; i++ {
		pool.ReleaseTicket(&Ticket{})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ticket := pool.AcquireTicket()
		pool.ReleaseTicket(ticket)
	}
}

func benchmarkResetTicket(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	pool := newTicketPool()
	ticket := &Ticket{PlayerID: 1}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.ResetTicket(ticket)
		ticket.PlayerID = 1
	}
}

func benchmarkReleaseTicket(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	pool := newTicketPool()
	for i := 0; i < 8192; i++ {
		pool.ReleaseTicket(&Ticket{})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ticket := pool.AcquireTicket()
		pool.ReleaseTicket(ticket)
	}
}

func benchmarkDecodeQueueJoin(b *testing.B, procs int, format WireFormat) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	decoder := payloadDecoder{}
	raw := validMsgpackPayload(42)
	if format == WireFormatJSON {
		raw = []byte(`{"player_id":42,"session_token":"valid-token","trophies":4200,"tier":3,"card_ids":[0,8,14,20,26,32,38,43],"consec_losses":-1,"consec_wins":2}`)
	}
	var out QueueJoinPayload
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if code := decoder.DecodeQueueJoin(raw, format, &out); code != IntakeOK {
			b.Fatalf("DecodeQueueJoin code = %v", code)
		}
	}
}

func benchmarkValidateSession(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	auth := &authDouble{ok: true}
	payload := validPayload(42)
	var token [64]byte
	copy(token[:], payload.token)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if code := auth.ValidateSession(42, token, uint8(len(payload.token))); code != IntakeOK {
			b.Fatalf("ValidateSession code = %v", code)
		}
	}
}

func benchmarkLoadSignals(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	store := &signalStoreDouble{signals: DerivedSignals{ConsecLosses: -1, ConsecWins: 1, ChurnRisk: 0.1, MonetizationP: 0.1}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, code := store.LoadSignals(42, -1, 1); code != IntakeOK {
			b.Fatalf("LoadSignals code = %v", code)
		}
	}
}

func benchmarkBuildDeckVector(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	validator := newDeckValidator()
	deck := validPayload(42).cards
	var out [8]float32
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if code := validator.BuildDeckVector(deck, &out); code != IntakeOK {
			b.Fatalf("BuildDeckVector code = %v", code)
		}
	}
}

func benchmarkWriteTicket(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	sink := &ringSinkDouble{shards: 16}
	ticket := &Ticket{PlayerID: 42}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if code := sink.WriteTicket(ticket); code != IntakeOK {
			b.Fatalf("WriteTicket code = %v", code)
		}
	}
}

func benchmarkShardDepth(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	sink := &ringSinkDouble{depth: 12, shards: 16}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sink.ShardDepth(42)
	}
}

func benchmarkEstimateWaitMS(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	estimator := waitEstimatorDouble{wait: 1200}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = estimator.EstimateWaitMS(4200)
	}
}

func benchmarkNowUnixNano(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	clock := &clockDouble{now: 1234}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = clock.NowUnixNano()
	}
}

func benchmarkParseTicket(b *testing.B, procs int, format WireFormat) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	pool := &steadyPool{}
	active := &activeTicketSet{}
	sink := &ringSinkDouble{depth: 13, shards: 16}
	processor := newIntakeProcessor(
		pool,
		payloadDecoder{},
		&authDouble{ok: true},
		&signalStoreDouble{signals: DerivedSignals{ConsecLosses: -1, ConsecWins: 2, ChurnRisk: 0.25, MonetizationP: 0.75}},
		newDeckValidator(),
		sink,
		waitEstimatorDouble{wait: 1200},
		&clockDouble{now: 1000},
		active,
	)
	raw := validMsgpackPayload(42)
	if format == WireFormatJSON {
		raw = []byte(`{"player_id":42,"session_token":"valid-token","trophies":4200,"tier":3,"card_ids":[0,8,14,20,26,32,38,43],"consec_losses":-1,"consec_wins":2}`)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		active.clear(42)
		ticket, ack := processor.ParseTicket(raw, format)
		if ticket == nil || ack.Status != QueueStatusQueued {
			b.Fatalf("ParseTicket ticket=%v ack=%+v", ticket, ack)
		}
		pool.ReleaseTicket(ticket)
	}
}

func assertOffset(t *testing.T, field string, got uintptr, want uintptr) {
	t.Helper()
	if got != want {
		t.Fatalf("%s offset = %d, want %d", field, got, want)
	}
}

func assertZeroTicket(t *testing.T, ticket *Ticket) {
	t.Helper()
	if *ticket != (Ticket{}) {
		t.Fatalf("ticket = %+v, want zero value", ticket)
	}
}

func fillTicket(ticket *Ticket) {
	ticket.PlayerID = 1
	ticket.EnqueuedAt = 2
	ticket.DeckVector = [8]float32{1, 2, 3, 4, 5, 6, 7, 8}
	ticket.Trophies = 3
	ticket.ChurnRisk = 0.4
	ticket.MonetizationP = 0.5
	ticket.ConsecLosses = -1
	ticket.ConsecWins = 1
	ticket.PoolTag = PoolMonetize
}

func magnitude(vector [8]float32) float32 {
	var sum float32
	for _, value := range vector {
		sum += value * value
	}
	return float32(math.Sqrt(float64(sum)))
}

type parseEnv struct {
	pool      *poolDouble
	auth      *authDouble
	signals   *signalStoreDouble
	sink      *ringSinkDouble
	estimator waitEstimatorDouble
	clock     *clockDouble
	active    *activeTicketSet
	processor *intakeProcessor
}

func newParseEnv() *parseEnv {
	env := &parseEnv{
		pool:      &poolDouble{inner: newTicketPool()},
		auth:      &authDouble{ok: true},
		signals:   &signalStoreDouble{signals: DerivedSignals{ConsecLosses: -1, ConsecWins: 2, ChurnRisk: 0.25, MonetizationP: 0.75}},
		sink:      &ringSinkDouble{depth: 13, shards: 16},
		estimator: waitEstimatorDouble{wait: 1200},
		clock:     &clockDouble{now: 1000},
		active:    &activeTicketSet{},
	}
	env.processor = newIntakeProcessor(env.pool, payloadDecoder{}, env.auth, env.signals, newDeckValidator(), env.sink, env.estimator, env.clock, env.active)
	return env
}

type poolDouble struct {
	inner    *ticketPool
	acquired atomic.Uint64
	released atomic.Uint64
}

type steadyPool struct {
	ticket Ticket
}

func (p *steadyPool) AcquireTicket() *Ticket {
	p.ticket = Ticket{}
	return &p.ticket
}

func (p *steadyPool) ResetTicket(ticket *Ticket) {
	if ticket != nil {
		*ticket = Ticket{}
	}
}

func (p *steadyPool) ReleaseTicket(ticket *Ticket) {
	if ticket != nil {
		*ticket = Ticket{}
	}
}

func (p *poolDouble) AcquireTicket() *Ticket {
	p.acquired.Add(1)
	return p.inner.AcquireTicket()
}

func (p *poolDouble) ResetTicket(ticket *Ticket) {
	p.inner.ResetTicket(ticket)
}

func (p *poolDouble) ReleaseTicket(ticket *Ticket) {
	if ticket != nil {
		p.released.Add(1)
	}
	p.inner.ReleaseTicket(ticket)
}

type authDouble struct {
	ok bool
}

func (a *authDouble) ValidateSession(_ uint64, token [64]byte, tokenLen uint8) IntakeErrorCode {
	if !a.ok || tokenLen == 0 || !bytes.Equal(token[:tokenLen], []byte("valid-token")) {
		return ErrUnauthorized
	}
	return IntakeOK
}

type signalStoreDouble struct {
	signals     DerivedSignals
	unavailable bool
}

func (s *signalStoreDouble) LoadSignals(_ uint64, fallbackLosses int8, fallbackWins int8) (DerivedSignals, IntakeErrorCode) {
	if s.unavailable {
		return DerivedSignals{ConsecLosses: fallbackLosses, ConsecWins: fallbackWins, ChurnRisk: 0.1, MonetizationP: 0.1}, IntakeOK
	}
	return s.signals, IntakeOK
}

type ringSinkDouble struct {
	code       IntakeErrorCode
	depth      uint16
	shards     uint64
	writeCount atomic.Uint64
	last       atomic.Uint64
}

func (s *ringSinkDouble) WriteTicket(ticket *Ticket) IntakeErrorCode {
	s.writeCount.Add(1)
	if s.shards != 0 {
		s.last.Store(ticket.PlayerID % s.shards)
	}
	if s.code != IntakeOK {
		return s.code
	}
	return IntakeOK
}

func (s *ringSinkDouble) ShardDepth(_ uint64) uint16 {
	return s.depth
}

type waitEstimatorDouble struct {
	wait uint32
}

func (e waitEstimatorDouble) EstimateWaitMS(_ int32) uint32 {
	return e.wait
}

type clockDouble struct {
	now  int64
	step int64
}

func (c *clockDouble) NowUnixNano() int64 {
	if c.step == 0 {
		return c.now
	}
	now := c.now
	c.now += c.step
	return now
}

type rawPayload struct {
	playerID uint64
	token    string
	trophies int32
	tier     uint8
	cards    [8]uint8
	losses   int8
	wins     int8
}

func validPayload(playerID uint64) rawPayload {
	return rawPayload{
		playerID: playerID,
		token:    "valid-token",
		trophies: 4200,
		tier:     3,
		cards:    [8]uint8{0, 8, 14, 20, 26, 32, 38, 43},
		losses:   -1,
		wins:     2,
	}
}

func validMsgpackPayload(playerID uint64) []byte {
	return msgpackPayload(validPayload(playerID))
}

func msgpackPayload(payload rawPayload) []byte {
	out := make([]byte, 0, 128)
	out = append(out, 0x87)
	out = appendMPString(out, "player_id")
	out = appendMPInt(out, int64(payload.playerID))
	out = appendMPString(out, "session_token")
	out = appendMPString(out, payload.token)
	out = appendMPString(out, "trophies")
	out = appendMPInt(out, int64(payload.trophies))
	out = appendMPString(out, "tier")
	out = appendMPInt(out, int64(payload.tier))
	out = appendMPString(out, "card_ids")
	out = append(out, 0x98)
	for _, card := range payload.cards {
		out = appendMPInt(out, int64(card))
	}
	out = appendMPString(out, "consec_losses")
	out = appendMPInt(out, int64(payload.losses))
	out = appendMPString(out, "consec_wins")
	out = appendMPInt(out, int64(payload.wins))
	return out
}

func msgpackPayloadWithRisk(payload rawPayload) []byte {
	out := make([]byte, 0, 160)
	out = append(out, 0x89)
	out = append(out, msgpackPayload(payload)[1:]...)
	out = appendMPString(out, "churn_risk")
	out = append(out, 0xca, 0x3f, 0x80, 0x00, 0x00)
	out = appendMPString(out, "monetization_p")
	out = append(out, 0xca, 0x3f, 0x80, 0x00, 0x00)
	return out
}

func appendMPString(out []byte, value string) []byte {
	if len(value) < 32 {
		out = append(out, 0xa0|byte(len(value)))
	} else {
		out = append(out, 0xd9, byte(len(value)))
	}
	return append(out, value...)
}

func appendMPInt(out []byte, value int64) []byte {
	switch {
	case value >= 0 && value <= 127:
		return append(out, byte(value))
	case value >= -32 && value < 0:
		return append(out, byte(int8(value)))
	case value >= math.MinInt8 && value <= math.MaxInt8:
		return append(out, 0xd0, byte(int8(value)))
	case value >= math.MinInt16 && value <= math.MaxInt16:
		return append(out, 0xd1, byte(value>>8), byte(value))
	default:
		return append(out, 0xd2, byte(value>>24), byte(value>>16), byte(value>>8), byte(value))
	}
}
