// Package contracts defines the Planner-owned public contract for MatchPoint
// modules. This file is intentionally declarative: no implementation logic
// belongs here.
package contracts

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
//
// Field order is part of the contract: the target size is one 64-byte cache
// line on 64-bit Go implementations.
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
