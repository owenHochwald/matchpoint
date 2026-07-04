# MATCH_SPEC.md — MatchPoint Domain Specification

> Companion to FEATURES.md. Defines the game-domain ground truth that the
> matchmaking system is built around: trophy tiers, deck archetypes,
> simulation population parameters, the player intake API contract, and
> the success metrics the system is evaluated against.
>
> Inspired by Clash Royale's ladder and deck system. Card names and tier
> labels are original to MatchPoint — no licensed names are used.

---

## 1. Trophy Ladder

### 1.1 Tier Definitions

The trophy ladder runs from 0 to 10,000+. Players start at 0 on first
account creation. Trophy gain/loss is asymmetric by design to create
a meaningful skill ceiling.

| Tier | Name          | Trophy Range  | Win Δ | Loss Δ | Notes                          |
|------|---------------|---------------|-------|--------|--------------------------------|
| 1    | Iron          | 0 – 999       | +30   | -0     | Floor protected; no loss below 0 |
| 2    | Bronze        | 1,000 – 2,999 | +30   | -20    | Largest population segment     |
| 3    | Silver        | 3,000 – 5,999 | +28   | -22    |                                |
| 4    | Gold          | 6,000 – 8,999 | +26   | -24    |                                |
| 5    | Diamond       | 9,000 – 11,999| +24   | -26    |                                |
| 6    | Champion      | 12,000+       | +20   | -30    | Skill compression; hardest to climb |

**Win/Loss deltas** are the base amounts. EOMM does not alter actual trophy
changes — it only selects opponents. Trophy math is applied post-match
based on outcome alone.

**Tier floor protection:** A player cannot drop below the bottom of their
current tier floor once they have reached it. E.g., a player who reaches
3,000 trophies cannot fall below 3,000. This is a retention mechanic and
must be respected by the simulation's trophy update logic.

```
Trophy floor per tier:
  Iron:     0       (no protection; free fall to 0)
  Bronze:   1,000
  Silver:   3,000
  Gold:     6,000
  Diamond:  9,000
  Champion: 12,000
```

### 1.2 Realistic Population Distribution

The simulated population must approximate a real playerbase, which is
heavily right-skewed. Most players are in Bronze; very few reach Champion.

| Tier      | % of 100k population | Count   | Mean trophies (seed) |
|-----------|----------------------|---------|----------------------|
| Iron      | 15%                  | 15,000  | 500                  |
| Bronze    | 45%                  | 45,000  | 1,800                |
| Silver    | 25%                  | 25,000  | 4,200                |
| Gold      | 10%                  | 10,000  | 7,200                |
| Diamond   | 4%                   | 4,000   | 10,200               |
| Champion  | 1%                   | 1,000   | 12,800               |

Seed trophies within each tier are drawn from a **truncated normal
distribution** centered on the mean with σ = 400, clipped to the tier
range. Use `rand.NormFloat64() * 400 + mean`, then clamp to tier bounds.

This distribution ensures all queue segments are meaningfully populated
at simulation start, making EOMM pool routing testable from tick 1.

---

## 2. Deck & Archetype Model

### 2.1 Deck Composition

Each player carries a deck of **8 cards** selected from a pool of 48
named cards. Cards have fixed archetype memberships. A player's deck
vector is derived from the cards they hold.

### 2.2 The 48-Card Roster

Cards are organized by primary archetype. Each card belongs to exactly
one primary archetype but may contribute fractionally to a secondary one
(reflected in vector construction in §2.4).

**Tank (8 cards)**
```
Giant Brute, Stone Golem, Iron Knight, Siege Turtle,
Rock Colossus, Armor Bear, Shieldwall, Fortress Drake
```

**AreaDamage (6 cards)**
```
Bomb Lobber, Flame Barrel, Cluster Shot, Arc Mage,
Blast Witch, Mortar Shell
```

**FastCycle (6 cards)**
```
Quick Goblin, Dart Scout, Pebble Throw, Rush Imp,
Flash Knife, Skeleton Pack
```

**Control (6 cards)**
```
Freeze Totem, Slow Mire, Gravity Well, Push Cannon,
Stun Lantern, Web Snare
```

**Range (6 cards)**
```
Sniper Elf, Tower Archer, Volley Crossbow, Eagle Eye,
Long Bolt, Rifleman
```

**Spell (6 cards)**
```
Lightning Strike, Fireball, Ice Shard, Poison Cloud,
Void Burst, Chain Shock
```

**Spawner (5 cards)**
```
Goblin Hut, Skeleton Forge, Bat Cave, Minion Nest,
Spider Den
```

**Assassin (5 cards)**
```
Shadow Blade, Mirror Dancer, Glass Cannon, Venom Strike,
Phantom Rogue
```

### 2.3 Deck Validity Rules

A valid deck must satisfy all of the following:
- Exactly 8 cards selected
- No duplicate cards
- Average elixir cost in [2.5, 5.0] (each card has a fixed elixir cost
  defined in §2.5; this validates the deck isn't all heavy or all cheap)
- At least 2 distinct archetypes represented

Decks failing any rule are rejected at ingestion with `ErrInvalidDeck`.

### 2.4 Vector Construction

Given a player's 8-card deck, the 8-dimensional archetype vector is
constructed as follows:

1. For each card, look up its archetype weights from the card table (§2.5).
   Primary archetype gets weight 1.0. Secondary archetype (if any) gets 0.4.

2. Sum the weights across all 8 cards for each dimension:
   ```
   raw[dim] = Σ weight(card, dim) for each card in deck
   ```

3. L2-normalize the result:
   ```
   magnitude = sqrt(Σ raw[dim]^2)
   vector[dim] = raw[dim] / magnitude
   ```

4. If magnitude < 1e-6, reject with `ErrZeroVector` (impossible with a
   valid deck, but guard is required).

**Example:** A deck with 3 Tank cards, 2 Spell cards, 2 FastCycle cards,
1 Control card produces:
```
raw = [3.0, 0.0, 2.0, 1.0, 0.0, 2.0, 0.0, 0.0]
magnitude ≈ sqrt(9 + 4 + 1 + 4) = sqrt(18) ≈ 4.24
vector ≈ [0.707, 0.0, 0.471, 0.236, 0.0, 0.471, 0.0, 0.0]
```

### 2.5 Card Table (Elixir Costs & Secondary Archetypes)

| Card              | Archetype   | Elixir | Secondary Archetype |
|-------------------|-------------|--------|---------------------|
| Giant Brute       | Tank        | 5      | —                   |
| Stone Golem       | Tank        | 6      | —                   |
| Iron Knight       | Tank        | 4      | Control             |
| Siege Turtle      | Tank        | 5      | Spawner             |
| Rock Colossus     | Tank        | 8      | —                   |
| Armor Bear        | Tank        | 4      | —                   |
| Shieldwall        | Tank        | 3      | Control             |
| Fortress Drake    | Tank        | 7      | —                   |
| Bomb Lobber       | AreaDamage  | 5      | —                   |
| Flame Barrel      | AreaDamage  | 2      | FastCycle           |
| Cluster Shot      | AreaDamage  | 4      | —                   |
| Arc Mage          | AreaDamage  | 5      | Spell               |
| Blast Witch       | AreaDamage  | 4      | Spell               |
| Mortar Shell      | AreaDamage  | 5      | Range               |
| Quick Goblin      | FastCycle   | 2      | —                   |
| Dart Scout        | FastCycle   | 3      | Range               |
| Pebble Throw      | FastCycle   | 1      | Spell               |
| Rush Imp          | FastCycle   | 2      | Assassin            |
| Flash Knife       | FastCycle   | 3      | Assassin            |
| Skeleton Pack     | FastCycle   | 3      | Spawner             |
| Freeze Totem      | Control     | 3      | —                   |
| Slow Mire         | Control     | 3      | —                   |
| Gravity Well      | Control     | 4      | Spell               |
| Push Cannon       | Control     | 4      | Tank                |
| Stun Lantern      | Control     | 2      | FastCycle           |
| Web Snare         | Control     | 3      | Spawner             |
| Sniper Elf        | Range       | 4      | —                   |
| Tower Archer      | Range       | 3      | —                   |
| Volley Crossbow   | Range       | 4      | Control             |
| Eagle Eye         | Range       | 4      | —                   |
| Long Bolt         | Range       | 3      | Spell               |
| Rifleman          | Range       | 3      | —                   |
| Lightning Strike  | Spell       | 5      | —                   |
| Fireball          | Spell       | 4      | AreaDamage          |
| Ice Shard         | Spell       | 3      | Control             |
| Poison Cloud      | Spell       | 4      | Control             |
| Void Burst        | Spell       | 4      | —                   |
| Chain Shock       | Spell       | 4      | AreaDamage          |
| Goblin Hut        | Spawner     | 5      | —                   |
| Skeleton Forge    | Spawner     | 5      | Tank                |
| Bat Cave          | Spawner     | 6      | Range               |
| Minion Nest       | Spawner     | 5      | AreaDamage          |
| Spider Den        | Spawner     | 4      | Control             |
| Shadow Blade      | Assassin    | 4      | FastCycle           |
| Mirror Dancer     | Assassin    | 5      | Control             |
| Glass Cannon      | Assassin    | 3      | Spell               |
| Venom Strike      | Assassin    | 4      | —                   |
| Phantom Rogue     | Assassin    | 3      | FastCycle           |

### 2.6 Simulated Deck Distribution

The 100k simulated players should carry decks drawn from these archetype
profiles with the following population weights. This ensures the cosine
similarity engine sees realistic diversity rather than uniform random noise.

| Archetype Profile   | Dominant Dims        | % of Population |
|---------------------|----------------------|-----------------|
| Tank-Spell          | Tank, Spell          | 18%             |
| FastCycle-Assassin  | FastCycle, Assassin  | 16%             |
| Control-Range       | Control, Range       | 14%             |
| Spawner-Tank        | Spawner, Tank        | 12%             |
| Spell-AreaDamage    | Spell, AreaDamage    | 12%             |
| AreaDamage-Control  | AreaDamage, Control  | 10%             |
| FastCycle-Spell     | FastCycle, Spell     | 10%             |
| Hybrid (3+ dims)    | No dominant          | 8%              |

For each profile, simulated decks are generated by randomly selecting
cards weighted toward the profile's dominant archetypes (3:1 weight
ratio for dominant vs. other archetypes), then applying §2.3 validity
rules. Retry up to 10 times on validity failure; panic if all fail
(indicates a bug in card selection logic, not expected in normal operation).

---

## 3. Player Intake Interface

This is the formal contract for what the matchmaking system receives when
a player joins the queue. It is the boundary between the game client (or
simulation driver) and the Ingestion Engine.

### 3.1 Transport

- Protocol: WebSocket, binary frames preferred (MessagePack), JSON fallback
- Negotiation: client sends `Content-Type: application/msgpack` in the
  upgrade handshake; server falls back to JSON if absent
- Frame direction: client → server (player joining queue)
- One frame per queue join event; player sends a new frame on re-queue
  after a match completes

### 3.2 QueueJoinPayload (wire format)

This is what the client sends. Fields the server computes itself are
marked [SERVER-DERIVED] and must never be trusted from the client.

```
QueueJoinPayload {
  // Identity
  player_id        uint64    REQUIRED  Globally unique player identifier
  session_token    string    REQUIRED  Opaque auth token; validated server-side

  // Current ladder state (client-reported, server-validated against DB)
  trophies         int32     REQUIRED  Must be in [0, 15000]; server cross-checks
  tier             uint8     REQUIRED  Must match trophies range (§1.1); server validates

  // Deck (client-reported)
  card_ids         [8]uint8  REQUIRED  Indices into the 48-card roster (0-47)
                                       Duplicates → ErrInvalidDeck
                                       Out-of-range → ErrInvalidDeck

  // Behavioral signals (client-reported; server may override from DB)
  consec_losses    int8      OPTIONAL  Default 0 if absent; server overwrites from DB
  consec_wins      int8      OPTIONAL  Default 0 if absent; server overwrites from DB

  // NOTE: churn_risk and monetization_p are NEVER sent by client.
  // They are computed server-side from session history and never exposed
  // to the client to prevent manipulation.
}
```

### 3.3 Server Derivation Steps

On receiving a valid `QueueJoinPayload`, the Ingestion Engine performs
these steps before constructing a `Ticket`:

```
1. Validate session_token → reject with ErrUnauthorized if invalid
2. Validate trophies range [0, 15000] → reject with ErrInvalidTrophies
3. Validate tier matches trophies per §1.1 table → reject with ErrTierMismatch
4. Validate card_ids: length==8, no duplicates, all in [0,47] → ErrInvalidDeck
5. Validate deck average elixir in [2.5, 5.0] → ErrInvalidDeck
6. Look up consec_losses, consec_wins from server-side session DB
   (overrides client-sent values; client values used only if DB is unavailable)
7. Look up churn_risk, monetization_p from analytics DB
   (default 0.1 and 0.1 respectively if unavailable)
8. Compute DeckVector from card_ids per §2.4
9. Acquire Ticket from pool, populate all fields
10. Set EnqueuedAt = time.Now().UnixNano()
11. Set PoolTag = 0 (mainstream); EOMM will reroute on next tick if needed
12. Write Ticket to ring buffer shard (PlayerID % NumShards)
```

### 3.4 Server Response

After a successful queue join, the server sends a single acknowledgment
frame back to the client:

```
QueueJoinAck {
  status        uint8    0=queued, 1=already_queued, 2=rejected
  error_code    uint8    0=none; see error codes below
  queue_depth   uint16   Approximate current depth of player's segment
  est_wait_ms   uint32   Estimated wait time in milliseconds
                         (based on recent match rate for the segment)
}

Error codes:
  0   None
  1   ErrUnauthorized
  2   ErrInvalidTrophies
  3   ErrTierMismatch
  4   ErrInvalidDeck
  5   ErrRingBufferFull  (retry after est_wait_ms)
```

### 3.5 Match Result Delivery

When the match core assigns a match, the server pushes a `MatchAssignment`
frame to both players' WebSocket connections:

```
MatchAssignment {
  match_id         uint64   Unique match identifier
  opponent_id      uint64   Opponent's player_id
  opponent_tier    uint8    Opponent's tier (for UI display)
  predicted_win_p  float32  P(this player wins); NOT sent to client in production
                            Present in simulation mode only (flag: MP_SIM_MODE=1)
  server_region    string   Always "local" for single-node deployment
}
```

`predicted_win_p` is explicitly withheld from production clients to prevent
players from reverse-engineering the EOMM targeting logic. It is included
in simulation mode for EOMM accuracy validation.

---

## 4. Success Metrics & Acceptance Criteria

These are the quantitative targets the system must hit. The review agent
validates these in the macro-simulation run (Module 7). A simulation run
that does not meet these criteria at the specified population is a system
failure, not a benchmark warning.

### 4.1 Matchmaking Quality Metrics

| Metric                        | Target          | Hard Limit      | Measurement                        |
|-------------------------------|-----------------|-----------------|------------------------------------|
| Median queue wait time        | < 3s            | < 10s           | p50 of (MatchedAt - EnqueuedAt)    |
| p95 queue wait time           | < 12s           | < 30s           | p95 of same                        |
| p99 queue wait time           | < 30s           | < 60s           | p99 of same                        |
| Trophy Δ within match         | < 200 trophies  | < 500 trophies  | |A.Trophies - B.Trophies| per match |
| Starvation rate               | < 0.1%          | < 0.5%          | Players waiting > 60s / total      |
| Match rate per tick           | > 40% of queue  | > 20% of queue  | Matches made / queue depth per tick |

### 4.2 EOMM Accuracy Metrics

| Metric                        | Target          | Measurement                                     |
|-------------------------------|-----------------|-------------------------------------------------|
| Retention match win rate      | 0.65 – 0.75     | Actual win rate for PoolTag=2 matches           |
| Monetization match win rate   | 0.25 – 0.35     | Actual win rate for PoolTag=3 matches           |
| Loser's pool evacuation rate  | > 60% per match | Players leaving losers pool via win / matches   |
| Churn reduction vs. baseline  | > 15% lower     | ChurnRisk Δ for EOMM-targeted vs. random match  |

### 4.3 System Performance Metrics

| Metric                        | Target          | Hard Limit      | Measurement                        |
|-------------------------------|-----------------|-----------------|-----------------------------------|
| Tick execution time           | < 100ms         | < 200ms         | time.Since(tickStart) per tick     |
| Tick overrun rate             | < 0.1%          | < 1%            | Overruns / total ticks             |
| Hot-path heap allocation      | 0 B/op          | 0 B/op          | go test -benchmem                  |
| Heap size (100k sim, steady)  | < 512MB         | < 1GB           | runtime.ReadMemStats               |
| Redis command latency p99     | < 3ms           | < 5ms           | Measured in tick handler           |
| Goroutine count (100k sim)    | < 102,000       | < 105,000       | runtime.NumGoroutine()             |

### 4.4 Simulation Convergence Criteria

The macro-simulation is considered valid (i.e., producing meaningful data)
only when all of the following are true after a 60-second warm-up period:

- All 6 queue segments have at least 50 active players
- The loser's pool has at least 100 active players
- The retention pool has at least 200 active players
- The match rate has been non-zero for at least 30 consecutive ticks
- Heap size has been stable (< 5% growth) for at least 60 consecutive ticks

If convergence is not reached within 120 seconds, the simulation driver
logs `SIM_CONVERGENCE_FAIL` and exits with code 2. This indicates a
population seeding bug, not a matchmaking bug.

---

## 5. Key Invariants for the Orchestrator

The following are the facts the orchestrator must treat as immutable
ground truth when making implementation and contract decisions:

1. **Trophies are the primary ranking signal.** Vector similarity is a
   secondary quality signal. Never let cosine distance dominate trophy
   proximity in the fitness function — the w1=0.4 / w2=0.3 split is fixed.

2. **The loser's pool exists to protect retention, not to punish skill.**
   Players in the loser's pool should win roughly 50% of matches against
   each other (it's a peer pool, not an easier pool). EOMM win rate
   manipulation only applies to the retention and monetization pools.

3. **The server never trusts churn_risk or monetization_p from the client.**
   These are derived server-side only. Any implementation code that reads
   these from the wire payload has a security bug.

4. **Tier floor protection is a trophy update rule, not a matchmaking rule.**
   The matchmaker matches by trophies, not by tier. Floor protection only
   applies when updating trophies after a match result.

5. **The 48-card roster is fixed for v1.** The implementation must not design
   the card system to require runtime card table updates. The table is a
   compile-time constant (`var CardTable = [48]CardDef{...}`).
