// Package ticket implements the MatchPoint ticket intake contract.
package ticket

import (
	"bytes"
	"math"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// TicketPoolTag identifies the queue family selected for a ticket.
type TicketPoolTag uint8

const (
	// PoolMainstream is the default segment queue selected at ingestion.
	PoolMainstream TicketPoolTag = 0
	// PoolLosers is assigned later by EOMM for losing-streak protection.
	PoolLosers TicketPoolTag = 1
	// PoolRetention is assigned later by EOMM for churn-risk protection.
	PoolRetention TicketPoolTag = 2
	// PoolMonetize is assigned later by EOMM for monetization trigger matches.
	PoolMonetize TicketPoolTag = 3
)

// IntakeErrorCode is the stable observable error taxonomy for queue intake.
type IntakeErrorCode uint8

const (
	// IntakeOK means no intake error occurred.
	IntakeOK IntakeErrorCode = 0
	// ErrUnauthorized means the session token failed server validation.
	ErrUnauthorized IntakeErrorCode = 1
	// ErrInvalidTrophies means client trophies were outside [0, 15000].
	ErrInvalidTrophies IntakeErrorCode = 2
	// ErrTierMismatch means the reported tier did not match trophy range.
	ErrTierMismatch IntakeErrorCode = 3
	// ErrInvalidDeck means card count, duplicates, IDs, elixir, or archetypes failed validation.
	ErrInvalidDeck IntakeErrorCode = 4
	// ErrRingBufferFull means the shard did not accept the ticket after the backpressure wait.
	ErrRingBufferFull IntakeErrorCode = 5
	// ErrZeroVector means deck vector magnitude was below 1e-6 before normalization.
	ErrZeroVector IntakeErrorCode = 6
	// ErrMalformedPayload means the frame could not be decoded as the negotiated wire format.
	ErrMalformedPayload IntakeErrorCode = 7
	// ErrParseTimeout means payload parsing exceeded the 50 microsecond budget.
	ErrParseTimeout IntakeErrorCode = 8
)

// QueueJoinStatus is the client-visible queue join result class.
type QueueJoinStatus uint8

const (
	// QueueStatusQueued means the ticket was accepted by the ring buffer shard.
	QueueStatusQueued QueueJoinStatus = 0
	// QueueStatusAlreadyQueued means this player already has an active queue ticket.
	QueueStatusAlreadyQueued QueueJoinStatus = 1
	// QueueStatusRejected means intake validation or handoff failed.
	QueueStatusRejected QueueJoinStatus = 2
)

// WireFormat identifies the negotiated inbound frame format.
type WireFormat uint8

const (
	// WireFormatMessagePack is the preferred binary queue join frame format.
	WireFormatMessagePack WireFormat = 0
	// WireFormatJSON is the fallback queue join frame format.
	WireFormatJSON WireFormat = 1
)

// Ticket is the primary queue-entry unit.
type Ticket struct {
	// PlayerID is immutable after validation and must be non-zero.
	PlayerID uint64
	// EnqueuedAt is Unix nanoseconds assigned by the server immediately before ring-buffer publication.
	EnqueuedAt int64
	// DeckVector is an L2-normalized 8-dimensional archetype vector; magnitude must be approximately 1.
	DeckVector [8]float32
	// Trophies is server-validated client ladder position in [0, 15000]; MATCH_SPEC governs this bound.
	Trophies int32
	// ChurnRisk is server-derived in [0.0, 1.0] and must never be trusted from the client.
	ChurnRisk float32
	// MonetizationP is server-derived in [0.0, 1.0] and must never be trusted from the client.
	MonetizationP float32
	// ConsecLosses is server-derived and always <= 0; client values are fallback only.
	ConsecLosses int8
	// ConsecWins is server-derived and always >= 0; client values are fallback only.
	ConsecWins int8
	// PoolTag is initialized to PoolMainstream before publication; concurrent mutation is outside this module.
	PoolTag TicketPoolTag
}

// QueueJoinPayload is the decoded client queue-join request before server derivation.
type QueueJoinPayload struct {
	// PlayerID is required and must be non-zero.
	PlayerID uint64
	// SessionToken contains the opaque auth token bytes copied from the wire string.
	SessionToken [64]byte
	// SessionTokenLen is the number of valid bytes in SessionToken and must be non-zero.
	SessionTokenLen uint8
	// Trophies is required and must be in [0, 15000].
	Trophies int32
	// Tier is required and must match the trophy table in MATCH_SPEC section 1.1.
	Tier uint8
	// CardIDs are required card roster indices; each must be in [0, 47] and unique.
	CardIDs [8]uint8
	// ConsecLosses is optional client data and may be used only if server session state is unavailable.
	ConsecLosses int8
	// ConsecWins is optional client data and may be used only if server session state is unavailable.
	ConsecWins int8
}

// QueueJoinAck is the single response frame emitted after a queue-join attempt.
type QueueJoinAck struct {
	// Status is queued, already queued, or rejected.
	Status QueueJoinStatus
	// ErrorCode is IntakeOK on success or the stable intake failure code.
	ErrorCode IntakeErrorCode
	// QueueDepth is the approximate current depth of the player's segment.
	QueueDepth uint16
	// EstWaitMS is the estimated wait time in milliseconds for retry or successful queue feedback.
	EstWaitMS uint32
}

// CardDef is a compile-time roster entry for the fixed v1 48-card table.
type CardDef struct {
	// PrimaryArchetype is the primary dimension index in [0, 7].
	PrimaryArchetype uint8
	// SecondaryArchetype is the optional secondary dimension index in [0, 7].
	SecondaryArchetype uint8
	// HasSecondary reports whether SecondaryArchetype contributes weight 0.4.
	HasSecondary bool
	// Elixir is the fixed card cost used for average deck cost validation.
	Elixir uint8
}

// DerivedSignals contains server-owned behavioral and risk state.
type DerivedSignals struct {
	// ConsecLosses is loaded from server session state and must be <= 0.
	ConsecLosses int8
	// ConsecWins is loaded from server session state and must be >= 0.
	ConsecWins int8
	// ChurnRisk is loaded from analytics state or defaults to 0.1.
	ChurnRisk float32
	// MonetizationP is loaded from analytics state or defaults to 0.1.
	MonetizationP float32
}

// TicketPool owns the acquire/reset/release lifecycle for pooled tickets.
type TicketPool interface {
	// AcquireTicket returns a zeroed or reset Ticket owned exclusively by the caller until release or publication.
	AcquireTicket() *Ticket
	// ResetTicket zeros all Ticket fields before reuse.
	ResetTicket(ticket *Ticket)
	// ReleaseTicket resets ticket and returns it to the pool; nil tickets are ignored.
	ReleaseTicket(ticket *Ticket)
}

// PayloadDecoder decodes one inbound frame using the negotiated wire format.
type PayloadDecoder interface {
	// DecodeQueueJoin decodes raw into out without trusting server-derived fields from the wire.
	DecodeQueueJoin(raw []byte, format WireFormat, out *QueueJoinPayload) IntakeErrorCode
}

// Authenticator validates the opaque session token for the player.
type Authenticator interface {
	// ValidateSession returns ErrUnauthorized unless token is valid for playerID.
	ValidateSession(playerID uint64, sessionToken [64]byte, sessionTokenLen uint8) IntakeErrorCode
}

// SignalStore provides server-derived streak and risk fields for ticket construction.
type SignalStore interface {
	// LoadSignals returns server-owned signals; unavailable stores return defaults and IntakeOK.
	LoadSignals(playerID uint64, fallbackLosses int8, fallbackWins int8) (DerivedSignals, IntakeErrorCode)
}

// DeckValidator validates fixed-roster decks and computes the normalized vector.
type DeckValidator interface {
	// BuildDeckVector validates cardIDs and writes the normalized vector into out.
	BuildDeckVector(cardIDs [8]uint8, out *[8]float32) IntakeErrorCode
}

// RingBufferSink is the ticket module's handoff boundary to the later ringbuffer module.
type RingBufferSink interface {
	// WriteTicket publishes ticket to the shard selected from PlayerID modulo shard count.
	WriteTicket(ticket *Ticket) IntakeErrorCode
	// ShardDepth returns an approximate queue depth for playerID's shard or segment.
	ShardDepth(playerID uint64) uint16
}

// QueueEstimator provides an approximate wait estimate for acknowledgment frames.
type QueueEstimator interface {
	// EstimateWaitMS returns a recent-match-rate estimate for the player's trophy segment.
	EstimateWaitMS(trophies int32) uint32
}

// Clock provides server-side enqueue timestamps.
type Clock interface {
	// NowUnixNano returns the current server timestamp in Unix nanoseconds.
	NowUnixNano() int64
}

// IntakeProcessor composes validation, derivation, pool acquire, and ring-buffer publication.
type IntakeProcessor interface {
	// ParseTicket validates raw input, acquires a Ticket, populates server-derived fields, and publishes it.
	ParseTicket(raw []byte, format WireFormat) (*Ticket, QueueJoinAck)
}

type ticketPool struct {
	pool sync.Pool
}

func newTicketPool() *ticketPool {
	return &ticketPool{}
}

func (p *ticketPool) AcquireTicket() *Ticket {
	ticket, _ := p.pool.Get().(*Ticket)
	if ticket == nil {
		ticket = new(Ticket)
	}
	*ticket = Ticket{}
	return ticket
}

func (p *ticketPool) ResetTicket(ticket *Ticket) {
	if ticket != nil {
		*ticket = Ticket{}
	}
}

func (p *ticketPool) ReleaseTicket(ticket *Ticket) {
	if ticket == nil {
		return
	}
	*ticket = Ticket{}
	p.pool.Put(ticket)
}

type payloadDecoder struct{}

func (payloadDecoder) DecodeQueueJoin(raw []byte, format WireFormat, out *QueueJoinPayload) IntakeErrorCode {
	if out == nil {
		return ErrMalformedPayload
	}

	var payload QueueJoinPayload
	var code IntakeErrorCode
	switch format {
	case WireFormatMessagePack:
		code = decodeMessagePack(raw, &payload)
	case WireFormatJSON:
		code = decodeJSON(raw, &payload)
	default:
		return ErrMalformedPayload
	}
	if code != IntakeOK {
		return code
	}
	*out = payload
	return IntakeOK
}

type deckValidator struct {
	cards [48]CardDef
}

func newDeckValidator() deckValidator {
	return deckValidator{cards: defaultCardTable()}
}

func (v deckValidator) BuildDeckVector(cardIDs [8]uint8, out *[8]float32) IntakeErrorCode {
	return buildDeckVector(cardIDs, out, &v.cards)
}

func buildDeckVector(cardIDs [8]uint8, out *[8]float32, cards *[48]CardDef) IntakeErrorCode {
	if out == nil || cards == nil {
		return ErrInvalidDeck
	}

	var raw [8]float32
	var seen uint64
	var archetypes uint16
	elixirSum := 0

	for _, cardID := range cardIDs {
		if cardID >= 48 {
			return ErrInvalidDeck
		}
		bit := uint64(1) << cardID
		if seen&bit != 0 {
			return ErrInvalidDeck
		}
		seen |= bit

		card := cards[cardID]
		if card.Elixir == 0 {
			return ErrInvalidDeck
		}
		if card.PrimaryArchetype < 8 {
			raw[card.PrimaryArchetype] += 1.0
			archetypes |= uint16(1) << card.PrimaryArchetype
		}
		if card.HasSecondary {
			if card.SecondaryArchetype >= 8 {
				return ErrInvalidDeck
			}
			raw[card.SecondaryArchetype] += 0.4
			archetypes |= uint16(1) << card.SecondaryArchetype
		}
		elixirSum += int(card.Elixir)
	}

	var mag2 float32
	for _, value := range raw {
		mag2 += value * value
	}
	mag := float32(math.Sqrt(float64(mag2)))
	if mag < 1e-6 {
		return ErrZeroVector
	}
	if elixirSum < 20 || elixirSum > 40 || popcount16(archetypes) < 2 {
		return ErrInvalidDeck
	}
	for i := range raw {
		out[i] = raw[i] / mag
	}
	return IntakeOK
}

func popcount16(value uint16) int {
	count := 0
	for value != 0 {
		value &= value - 1
		count++
	}
	return count
}

type activeTicketSet struct {
	slots [4096]atomic.Uint64
}

func (s *activeTicketSet) markOrExists(playerID uint64) bool {
	idx := playerID & uint64(len(s.slots)-1)
	for probe := uint64(0); probe < uint64(len(s.slots)); probe++ {
		slot := &s.slots[(idx+probe)&uint64(len(s.slots)-1)]
		current := slot.Load()
		if current == playerID {
			return true
		}
		if current == 0 && slot.CompareAndSwap(0, playerID) {
			return false
		}
	}
	return true
}

func (s *activeTicketSet) clear(playerID uint64) {
	idx := playerID & uint64(len(s.slots)-1)
	for probe := uint64(0); probe < uint64(len(s.slots)); probe++ {
		slot := &s.slots[(idx+probe)&uint64(len(s.slots)-1)]
		current := slot.Load()
		if current == playerID {
			slot.CompareAndSwap(playerID, 0)
			return
		}
		if current == 0 {
			return
		}
	}
}

type intakeProcessor struct {
	pool           TicketPool
	decoder        PayloadDecoder
	fastDecoder    payloadDecoder
	useFastDecoder bool
	auth           Authenticator
	signals        SignalStore
	deck           DeckValidator
	fastDeck       deckValidator
	useFastDeck    bool
	sink           RingBufferSink
	estimator      QueueEstimator
	clock          Clock
	active         *activeTicketSet
	payloadScratch QueueJoinPayload
	vectorScratch  [8]float32
}

func newIntakeProcessor(pool TicketPool, decoder PayloadDecoder, auth Authenticator, signals SignalStore, deck DeckValidator, sink RingBufferSink, estimator QueueEstimator, clock Clock, active *activeTicketSet) *intakeProcessor {
	p := &intakeProcessor{
		pool:      pool,
		auth:      auth,
		signals:   signals,
		sink:      sink,
		estimator: estimator,
		clock:     clock,
		active:    active,
	}
	if concrete, ok := decoder.(payloadDecoder); ok {
		p.fastDecoder = concrete
		p.useFastDecoder = true
	} else if concrete, ok := decoder.(*payloadDecoder); ok && concrete != nil {
		p.fastDecoder = *concrete
		p.useFastDecoder = true
	} else {
		p.decoder = decoder
	}
	if concrete, ok := deck.(deckValidator); ok {
		p.fastDeck = concrete
		p.useFastDeck = true
	} else if concrete, ok := deck.(*deckValidator); ok && concrete != nil {
		p.fastDeck = *concrete
		p.useFastDeck = true
	} else {
		p.deck = deck
	}
	return p
}

func (p *intakeProcessor) ParseTicket(raw []byte, format WireFormat) (*Ticket, QueueJoinAck) {
	start := p.clock.NowUnixNano()
	payload := &p.payloadScratch
	*payload = QueueJoinPayload{}
	var code IntakeErrorCode
	if p.useFastDecoder {
		switch format {
		case WireFormatMessagePack:
			code = decodeMessagePack(raw, payload)
		case WireFormatJSON:
			code = decodeJSON(raw, payload)
		default:
			code = ErrMalformedPayload
		}
	} else {
		code = p.decoder.DecodeQueueJoin(raw, format, payload)
	}
	if p.clock.NowUnixNano()-start > int64(50*time.Microsecond) {
		return nil, rejected(ErrParseTimeout, 0)
	}
	if code != IntakeOK {
		return nil, rejected(code, 0)
	}
	if payload.PlayerID == 0 {
		return nil, rejected(ErrMalformedPayload, 0)
	}
	if payload.SessionTokenLen == 0 {
		return nil, rejected(ErrUnauthorized, 0)
	}
	if p.auth.ValidateSession(payload.PlayerID, payload.SessionToken, payload.SessionTokenLen) != IntakeOK {
		return nil, rejected(ErrUnauthorized, 0)
	}
	if payload.Trophies < 0 || payload.Trophies > 15000 {
		return nil, rejected(ErrInvalidTrophies, 0)
	}
	if tierForTrophies(payload.Trophies) != payload.Tier {
		return nil, rejected(ErrTierMismatch, 0)
	}

	vector := &p.vectorScratch
	*vector = [8]float32{}
	if p.useFastDeck {
		code = buildDeckVector(payload.CardIDs, vector, &p.fastDeck.cards)
	} else {
		code = p.deck.BuildDeckVector(payload.CardIDs, vector)
	}
	if code != IntakeOK {
		return nil, rejected(code, 0)
	}

	signals, code := p.signals.LoadSignals(payload.PlayerID, payload.ConsecLosses, payload.ConsecWins)
	if code != IntakeOK {
		return nil, rejected(code, 0)
	}
	if signals.ConsecLosses > 0 || signals.ConsecWins < 0 || signals.ChurnRisk < 0 || signals.ChurnRisk > 1 || signals.MonetizationP < 0 || signals.MonetizationP > 1 {
		return nil, rejected(ErrMalformedPayload, 0)
	}

	if p.active.markOrExists(payload.PlayerID) {
		return nil, QueueJoinAck{
			Status:     QueueStatusAlreadyQueued,
			ErrorCode:  IntakeOK,
			QueueDepth: p.sink.ShardDepth(payload.PlayerID),
			EstWaitMS:  p.estimator.EstimateWaitMS(payload.Trophies),
		}
	}

	ticket := p.pool.AcquireTicket()
	ticket.PlayerID = payload.PlayerID
	ticket.EnqueuedAt = p.clock.NowUnixNano()
	ticket.DeckVector = *vector
	ticket.Trophies = payload.Trophies
	ticket.ChurnRisk = signals.ChurnRisk
	ticket.MonetizationP = signals.MonetizationP
	ticket.ConsecLosses = signals.ConsecLosses
	ticket.ConsecWins = signals.ConsecWins
	ticket.PoolTag = PoolMainstream

	if code := p.sink.WriteTicket(ticket); code != IntakeOK {
		p.active.clear(payload.PlayerID)
		p.pool.ReleaseTicket(ticket)
		return nil, QueueJoinAck{
			Status:    QueueStatusRejected,
			ErrorCode: code,
			EstWaitMS: p.estimator.EstimateWaitMS(payload.Trophies),
		}
	}

	return ticket, QueueJoinAck{
		Status:     QueueStatusQueued,
		ErrorCode:  IntakeOK,
		QueueDepth: p.sink.ShardDepth(payload.PlayerID),
		EstWaitMS:  p.estimator.EstimateWaitMS(payload.Trophies),
	}
}

func rejected(code IntakeErrorCode, estWaitMS uint32) QueueJoinAck {
	return QueueJoinAck{Status: QueueStatusRejected, ErrorCode: code, EstWaitMS: estWaitMS}
}

func tierForTrophies(trophies int32) uint8 {
	switch {
	case trophies < 1000:
		return 1
	case trophies < 3000:
		return 2
	case trophies < 6000:
		return 3
	case trophies < 9000:
		return 4
	case trophies < 12000:
		return 5
	default:
		return 6
	}
}

func defaultCardTable() [48]CardDef {
	const (
		tank uint8 = iota
		areaDamage
		fastCycle
		control
		ranged
		spell
		spawner
		assassin
	)

	return [48]CardDef{
		{PrimaryArchetype: tank, Elixir: 5},
		{PrimaryArchetype: tank, Elixir: 6},
		{PrimaryArchetype: tank, SecondaryArchetype: control, HasSecondary: true, Elixir: 4},
		{PrimaryArchetype: tank, SecondaryArchetype: spawner, HasSecondary: true, Elixir: 5},
		{PrimaryArchetype: tank, Elixir: 8},
		{PrimaryArchetype: tank, Elixir: 4},
		{PrimaryArchetype: tank, SecondaryArchetype: control, HasSecondary: true, Elixir: 3},
		{PrimaryArchetype: tank, Elixir: 7},
		{PrimaryArchetype: areaDamage, Elixir: 5},
		{PrimaryArchetype: areaDamage, SecondaryArchetype: fastCycle, HasSecondary: true, Elixir: 2},
		{PrimaryArchetype: areaDamage, Elixir: 4},
		{PrimaryArchetype: areaDamage, SecondaryArchetype: spell, HasSecondary: true, Elixir: 5},
		{PrimaryArchetype: areaDamage, SecondaryArchetype: spell, HasSecondary: true, Elixir: 4},
		{PrimaryArchetype: areaDamage, SecondaryArchetype: ranged, HasSecondary: true, Elixir: 5},
		{PrimaryArchetype: fastCycle, Elixir: 2},
		{PrimaryArchetype: fastCycle, SecondaryArchetype: ranged, HasSecondary: true, Elixir: 3},
		{PrimaryArchetype: fastCycle, SecondaryArchetype: spell, HasSecondary: true, Elixir: 1},
		{PrimaryArchetype: fastCycle, SecondaryArchetype: assassin, HasSecondary: true, Elixir: 2},
		{PrimaryArchetype: fastCycle, SecondaryArchetype: assassin, HasSecondary: true, Elixir: 3},
		{PrimaryArchetype: fastCycle, SecondaryArchetype: spawner, HasSecondary: true, Elixir: 3},
		{PrimaryArchetype: control, Elixir: 3},
		{PrimaryArchetype: control, Elixir: 3},
		{PrimaryArchetype: control, SecondaryArchetype: spell, HasSecondary: true, Elixir: 4},
		{PrimaryArchetype: control, SecondaryArchetype: tank, HasSecondary: true, Elixir: 4},
		{PrimaryArchetype: control, SecondaryArchetype: fastCycle, HasSecondary: true, Elixir: 2},
		{PrimaryArchetype: control, SecondaryArchetype: spawner, HasSecondary: true, Elixir: 3},
		{PrimaryArchetype: ranged, Elixir: 4},
		{PrimaryArchetype: ranged, Elixir: 3},
		{PrimaryArchetype: ranged, SecondaryArchetype: control, HasSecondary: true, Elixir: 4},
		{PrimaryArchetype: ranged, Elixir: 4},
		{PrimaryArchetype: ranged, SecondaryArchetype: spell, HasSecondary: true, Elixir: 3},
		{PrimaryArchetype: ranged, Elixir: 3},
		{PrimaryArchetype: spell, Elixir: 5},
		{PrimaryArchetype: spell, SecondaryArchetype: areaDamage, HasSecondary: true, Elixir: 4},
		{PrimaryArchetype: spell, SecondaryArchetype: control, HasSecondary: true, Elixir: 3},
		{PrimaryArchetype: spell, SecondaryArchetype: control, HasSecondary: true, Elixir: 4},
		{PrimaryArchetype: spell, Elixir: 4},
		{PrimaryArchetype: spell, SecondaryArchetype: areaDamage, HasSecondary: true, Elixir: 4},
		{PrimaryArchetype: spawner, Elixir: 5},
		{PrimaryArchetype: spawner, SecondaryArchetype: tank, HasSecondary: true, Elixir: 5},
		{PrimaryArchetype: spawner, SecondaryArchetype: ranged, HasSecondary: true, Elixir: 6},
		{PrimaryArchetype: spawner, SecondaryArchetype: areaDamage, HasSecondary: true, Elixir: 5},
		{PrimaryArchetype: spawner, SecondaryArchetype: control, HasSecondary: true, Elixir: 4},
		{PrimaryArchetype: assassin, SecondaryArchetype: fastCycle, HasSecondary: true, Elixir: 4},
		{PrimaryArchetype: assassin, SecondaryArchetype: control, HasSecondary: true, Elixir: 5},
		{PrimaryArchetype: assassin, SecondaryArchetype: spell, HasSecondary: true, Elixir: 3},
		{PrimaryArchetype: assassin, Elixir: 4},
		{PrimaryArchetype: assassin, SecondaryArchetype: fastCycle, HasSecondary: true, Elixir: 3},
	}
}

func decodeJSON(raw []byte, out *QueueJoinPayload) IntakeErrorCode {
	i := skipSpace(raw, 0)
	if i >= len(raw) || raw[i] != '{' {
		return ErrMalformedPayload
	}
	i++
	seenCards := false
	for {
		i = skipSpace(raw, i)
		if i >= len(raw) {
			return ErrMalformedPayload
		}
		if raw[i] == '}' {
			if !seenCards {
				return ErrMalformedPayload
			}
			return IntakeOK
		}

		var key []byte
		var ok bool
		key, i, ok = parseJSONString(raw, i)
		if !ok {
			return ErrMalformedPayload
		}
		i = skipSpace(raw, i)
		if i >= len(raw) || raw[i] != ':' {
			return ErrMalformedPayload
		}
		i = skipSpace(raw, i+1)

		switch {
		case bytes.Equal(key, []byte("player_id")):
			value, next, ok := parseJSONInt(raw, i)
			if !ok || value < 0 {
				return ErrMalformedPayload
			}
			out.PlayerID = uint64(value)
			i = next
		case bytes.Equal(key, []byte("session_token")):
			next, ok := copyJSONString(raw, i, &out.SessionToken, &out.SessionTokenLen)
			if !ok {
				return ErrMalformedPayload
			}
			i = next
		case bytes.Equal(key, []byte("trophies")):
			value, next, ok := parseJSONInt(raw, i)
			if !ok {
				return ErrMalformedPayload
			}
			out.Trophies = int32(value)
			i = next
		case bytes.Equal(key, []byte("tier")):
			value, next, ok := parseJSONInt(raw, i)
			if !ok || value < 0 || value > 255 {
				return ErrMalformedPayload
			}
			out.Tier = uint8(value)
			i = next
		case bytes.Equal(key, []byte("card_ids")):
			next, ok := parseJSONCards(raw, i, &out.CardIDs)
			if !ok {
				return ErrMalformedPayload
			}
			seenCards = true
			i = next
		case bytes.Equal(key, []byte("consec_losses")):
			value, next, ok := parseJSONInt(raw, i)
			if !ok || value < math.MinInt8 || value > math.MaxInt8 {
				return ErrMalformedPayload
			}
			out.ConsecLosses = int8(value)
			i = next
		case bytes.Equal(key, []byte("consec_wins")):
			value, next, ok := parseJSONInt(raw, i)
			if !ok || value < math.MinInt8 || value > math.MaxInt8 {
				return ErrMalformedPayload
			}
			out.ConsecWins = int8(value)
			i = next
		default:
			next, ok := skipJSONValue(raw, i)
			if !ok {
				return ErrMalformedPayload
			}
			i = next
		}

		i = skipSpace(raw, i)
		if i >= len(raw) {
			return ErrMalformedPayload
		}
		if raw[i] == ',' {
			i++
			continue
		}
		if raw[i] == '}' {
			if !seenCards {
				return ErrMalformedPayload
			}
			return IntakeOK
		}
		return ErrMalformedPayload
	}
}

func skipSpace(raw []byte, i int) int {
	for i < len(raw) {
		switch raw[i] {
		case ' ', '\n', '\r', '\t':
			i++
		default:
			return i
		}
	}
	return i
}

func parseJSONString(raw []byte, i int) ([]byte, int, bool) {
	if i >= len(raw) || raw[i] != '"' {
		return nil, i, false
	}
	start := i + 1
	i = start
	for i < len(raw) {
		if raw[i] == '\\' {
			return nil, i, false
		}
		if raw[i] == '"' {
			return raw[start:i], i + 1, true
		}
		i++
	}
	return nil, i, false
}

func copyJSONString(raw []byte, i int, dst *[64]byte, dstLen *uint8) (int, bool) {
	value, next, ok := parseJSONString(raw, i)
	if !ok || len(value) > len(dst) {
		return i, false
	}
	copy(dst[:], value)
	*dstLen = uint8(len(value))
	return next, true
}

func parseJSONInt(raw []byte, i int) (int64, int, bool) {
	start := i
	if i < len(raw) && raw[i] == '-' {
		i++
	}
	digits := i
	for i < len(raw) && raw[i] >= '0' && raw[i] <= '9' {
		i++
	}
	if digits == i {
		return 0, start, false
	}
	value, err := strconv.ParseInt(string(raw[start:i]), 10, 64)
	return value, i, err == nil
}

func parseJSONCards(raw []byte, i int, cards *[8]uint8) (int, bool) {
	i = skipSpace(raw, i)
	if i >= len(raw) || raw[i] != '[' {
		return i, false
	}
	i++
	for idx := 0; idx < len(cards); idx++ {
		i = skipSpace(raw, i)
		value, next, ok := parseJSONInt(raw, i)
		if !ok || value < 0 || value > 255 {
			return i, false
		}
		cards[idx] = uint8(value)
		i = skipSpace(raw, next)
		if idx < len(cards)-1 {
			if i >= len(raw) || raw[i] != ',' {
				return i, false
			}
			i++
		}
	}
	i = skipSpace(raw, i)
	if i >= len(raw) || raw[i] != ']' {
		return i, false
	}
	return i + 1, true
}

func skipJSONValue(raw []byte, i int) (int, bool) {
	i = skipSpace(raw, i)
	if i >= len(raw) {
		return i, false
	}
	switch raw[i] {
	case '"':
		_, next, ok := parseJSONString(raw, i)
		return next, ok
	case '[':
		depth := 1
		i++
		for i < len(raw) && depth > 0 {
			switch raw[i] {
			case '"':
				_, next, ok := parseJSONString(raw, i)
				if !ok {
					return i, false
				}
				i = next
				continue
			case '[':
				depth++
			case ']':
				depth--
			}
			i++
		}
		return i, depth == 0
	case '{':
		depth := 1
		i++
		for i < len(raw) && depth > 0 {
			switch raw[i] {
			case '"':
				_, next, ok := parseJSONString(raw, i)
				if !ok {
					return i, false
				}
				i = next
				continue
			case '{':
				depth++
			case '}':
				depth--
			}
			i++
		}
		return i, depth == 0
	default:
		for i < len(raw) && raw[i] != ',' && raw[i] != '}' && raw[i] != ']' {
			i++
		}
		return i, true
	}
}

func decodeMessagePack(raw []byte, out *QueueJoinPayload) IntakeErrorCode {
	if len(raw) == 0 {
		return ErrMalformedPayload
	}
	i := 0
	count, ok := readMapLen(raw, &i)
	if !ok {
		return ErrMalformedPayload
	}
	seenCards := false
	for field := 0; field < count; field++ {
		key, ok := readMPString(raw, &i)
		if !ok {
			return ErrMalformedPayload
		}
		switch {
		case bytes.Equal(key, []byte("player_id")):
			value, ok := readMPInt(raw, &i)
			if !ok || value < 0 {
				return ErrMalformedPayload
			}
			out.PlayerID = uint64(value)
		case bytes.Equal(key, []byte("session_token")):
			if !readMPStringIntoToken(raw, &i, &out.SessionToken, &out.SessionTokenLen) {
				return ErrMalformedPayload
			}
		case bytes.Equal(key, []byte("trophies")):
			value, ok := readMPInt(raw, &i)
			if !ok {
				return ErrMalformedPayload
			}
			out.Trophies = int32(value)
		case bytes.Equal(key, []byte("tier")):
			value, ok := readMPInt(raw, &i)
			if !ok || value < 0 || value > 255 {
				return ErrMalformedPayload
			}
			out.Tier = uint8(value)
		case bytes.Equal(key, []byte("card_ids")):
			if !readMPCards(raw, &i, &out.CardIDs) {
				return ErrMalformedPayload
			}
			seenCards = true
		case bytes.Equal(key, []byte("consec_losses")):
			value, ok := readMPInt(raw, &i)
			if !ok || value < math.MinInt8 || value > math.MaxInt8 {
				return ErrMalformedPayload
			}
			out.ConsecLosses = int8(value)
		case bytes.Equal(key, []byte("consec_wins")):
			value, ok := readMPInt(raw, &i)
			if !ok || value < math.MinInt8 || value > math.MaxInt8 {
				return ErrMalformedPayload
			}
			out.ConsecWins = int8(value)
		default:
			if !skipMPValue(raw, &i) {
				return ErrMalformedPayload
			}
		}
	}
	if i != len(raw) || !seenCards {
		return ErrMalformedPayload
	}
	return IntakeOK
}

func readMapLen(raw []byte, i *int) (int, bool) {
	if *i >= len(raw) {
		return 0, false
	}
	b := raw[*i]
	*i += 1
	if b >= 0x80 && b <= 0x8f {
		return int(b & 0x0f), true
	}
	if b == 0xde && *i+2 <= len(raw) {
		n := int(raw[*i])<<8 | int(raw[*i+1])
		*i += 2
		return n, true
	}
	return 0, false
}

func readArrayLen(raw []byte, i *int) (int, bool) {
	if *i >= len(raw) {
		return 0, false
	}
	b := raw[*i]
	*i += 1
	if b >= 0x90 && b <= 0x9f {
		return int(b & 0x0f), true
	}
	if b == 0xdc && *i+2 <= len(raw) {
		n := int(raw[*i])<<8 | int(raw[*i+1])
		*i += 2
		return n, true
	}
	return 0, false
}

func readMPString(raw []byte, i *int) ([]byte, bool) {
	if *i >= len(raw) {
		return nil, false
	}
	b := raw[*i]
	*i += 1
	var n int
	switch {
	case b >= 0xa0 && b <= 0xbf:
		n = int(b & 0x1f)
	case b == 0xd9 && *i < len(raw):
		n = int(raw[*i])
		*i++
	case b == 0xda && *i+2 <= len(raw):
		n = int(raw[*i])<<8 | int(raw[*i+1])
		*i += 2
	default:
		return nil, false
	}
	if n < 0 || *i+n > len(raw) {
		return nil, false
	}
	value := raw[*i : *i+n]
	*i += n
	return value, true
}

func readMPStringIntoToken(raw []byte, i *int, dst *[64]byte, dstLen *uint8) bool {
	value, ok := readMPString(raw, i)
	if !ok || len(value) > len(dst) {
		return false
	}
	copy(dst[:], value)
	*dstLen = uint8(len(value))
	return true
}

func readMPInt(raw []byte, i *int) (int64, bool) {
	if *i >= len(raw) {
		return 0, false
	}
	b := raw[*i]
	*i += 1
	switch {
	case b <= 0x7f:
		return int64(b), true
	case b >= 0xe0:
		return int64(int8(b)), true
	case b == 0xcc && *i < len(raw):
		value := raw[*i]
		*i++
		return int64(value), true
	case b == 0xcd && *i+2 <= len(raw):
		value := uint16(raw[*i])<<8 | uint16(raw[*i+1])
		*i += 2
		return int64(value), true
	case b == 0xce && *i+4 <= len(raw):
		value := uint32(raw[*i])<<24 | uint32(raw[*i+1])<<16 | uint32(raw[*i+2])<<8 | uint32(raw[*i+3])
		*i += 4
		return int64(value), true
	case b == 0xd0 && *i < len(raw):
		value := int8(raw[*i])
		*i++
		return int64(value), true
	case b == 0xd1 && *i+2 <= len(raw):
		value := int16(uint16(raw[*i])<<8 | uint16(raw[*i+1]))
		*i += 2
		return int64(value), true
	case b == 0xd2 && *i+4 <= len(raw):
		value := int32(uint32(raw[*i])<<24 | uint32(raw[*i+1])<<16 | uint32(raw[*i+2])<<8 | uint32(raw[*i+3]))
		*i += 4
		return int64(value), true
	}
	return 0, false
}

func readMPCards(raw []byte, i *int, cards *[8]uint8) bool {
	n, ok := readArrayLen(raw, i)
	if !ok || n != len(cards) {
		return false
	}
	for idx := range cards {
		value, ok := readMPInt(raw, i)
		if !ok || value < 0 || value > 255 {
			return false
		}
		cards[idx] = uint8(value)
	}
	return true
}

func skipMPValue(raw []byte, i *int) bool {
	if *i >= len(raw) {
		return false
	}
	b := raw[*i]
	switch {
	case b <= 0x7f || b >= 0xe0:
		*i++
		return true
	case b >= 0xa0 && b <= 0xbf:
		_, ok := readMPString(raw, i)
		return ok
	case b >= 0x90 && b <= 0x9f:
		n, ok := readArrayLen(raw, i)
		if !ok {
			return false
		}
		for idx := 0; idx < n; idx++ {
			if !skipMPValue(raw, i) {
				return false
			}
		}
		return true
	case b >= 0x80 && b <= 0x8f:
		n, ok := readMapLen(raw, i)
		if !ok {
			return false
		}
		for idx := 0; idx < n*2; idx++ {
			if !skipMPValue(raw, i) {
				return false
			}
		}
		return true
	case b == 0xc2 || b == 0xc3 || b == 0xc0:
		*i++
		return true
	case b == 0xcc || b == 0xd0:
		*i += 2
	case b == 0xcd || b == 0xd1:
		*i += 3
	case b == 0xce || b == 0xd2 || b == 0xca:
		*i += 5
	case b == 0xcf || b == 0xd3 || b == 0xcb:
		*i += 9
	case b == 0xd9:
		_, ok := readMPString(raw, i)
		return ok
	case b == 0xda:
		_, ok := readMPString(raw, i)
		return ok
	default:
		return false
	}
	return *i <= len(raw)
}
