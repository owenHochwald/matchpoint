# FEATURES.md — Project MatchPoint Systems Specification

> **Document Status:** Authoritative. All agent contracts, implementation
> decisions, and benchmark baselines derive from this document. Amendments
> require explicit versioning.
>
> **Version:** 1.0.0\
> **Scope:** Single-node Go matchmaking core + macro-simulation platform

---

## Table of Contents

1. [System Topology](#1-system-topology)
2. [Data Model & Core Types](#2-data-model--core-types)
3. [Module A — Ingestion Engine](#3-module-a--ingestion-engine)
4. [Module B — Match Core Loop (200ms Tick)](#4-module-b--match-core-loop-200ms-tick)
5. [Module C — Engagement-Optimized Matchmaking (EOMM)](#5-module-c--engagement-optimized-matchmaking-eomm)
6. [Module D — Deck Vector Archetype Engine](#6-module-d--deck-vector-archetype-engine)
7. [Module E — Macro-Simulation Harness](#7-module-e--macro-simulation-harness)
8. [Module F — Telemetry & Visualizer Bridge](#8-module-f--telemetry--visualizer-bridge)
9. [Storage Layer Contract](#9-storage-layer-contract)
10. [Memory & Allocation Contracts](#10-memory--allocation-contracts)
11. [Concurrency Model](#11-concurrency-model)
12. [Error Handling & Observability](#12-error-handling--observability)
13. [Testing Philosophy & Coverage Floors](#13-testing-philosophy--coverage-floors)
14. [Delivery Sequence & Dependency Graph](#14-delivery-sequence--dependency-graph)

---

## 1. System Topology

```
                      ┌────────────────────────────────────────────────────┐
                      │                  SINGLE BARE-METAL NODE            │
                      │                                                    │
WebSocket Clients     │  ┌──────────────┐     Lock-Free     ┌───────────┐ │
──────────────────►   │  │  Ingestion   │ ──► Ring Buffer ──►│  Match    │ │
(player connections)  │  │   Engine     │                   │  Core     │ │
                      │  └──────────────┘                   │  Loop     │ │
                      │         │                           └─────┬─────┘ │
                      │    Ticket Pool                            │        │
                      │    (sync.Pool)                     200ms tick      │
                      │                                           │        │
                      │                          ┌────────────────┼──────┐ │
                      │                          │                │      │ │
                      │                    ┌─────▼────┐   ┌──────▼───┐  │ │
                      │                    │  EOMM    │   │  Vector  │  │ │
                      │                    │  Engine  │   │  Arch.   │  │ │
                      │                    └─────┬────┘   └──────┬───┘  │ │
                      │                          └───────┬────────┘      │ │
                      │                                  │               │ │
                      │                          ┌───────▼──────┐        │ │
                      │                          │ Redis ZSET   │        │ │
                      │                          │ Priority     │        │ │
                      │                          │ Queues + Lua │        │ │
                      │                          └──────────────┘        │ │
                      │                                                   │ │
                      │  ┌─────────────────────────────────────────────┐  │ │
                      │  │  Macro-Simulation Harness (100k goroutines) │  │ │
                      │  └───────────────────┬─────────────────────────┘  │ │
                      │                      │ async telemetry             │ │
                      │             ┌────────▼──────────┐                 │ │
                      │             │ Telemetry Ring     │ ──► WS ──► UI  │ │
                      │             │ Buffer + Emitter   │                 │ │
                      │             └───────────────────┘                 │ │
                      └────────────────────────────────────────────────────┘
```

**Design invariant:** No module may communicate with another by sharing mutable
state directly. All cross-module data flow passes through either a typed
channel, a ring buffer slot, or a function call over a contracted interface. No
global variables.

---

## 2. Data Model & Core Types

### 2.1 `Ticket` — Primary Unit of Queue Entry

```
Ticket (cache-line aligned, 64 bytes target)
├── PlayerID       uint64        // immutable identity
├── Trophies       int32         // current rank score
├── ConsecLosses   int8          // negative counter; magnitude = streak depth
├── ConsecWins     int8          // positive counter
├── ChurnRisk      float32       // [0.0, 1.0]; ML-derived or heuristic
├── MonetizationP  float32       // [0.0, 1.0]; spend propensity score
├── DeckVector     [8]float32    // normalized archetype signature (see §6)
├── EnqueuedAt     int64         // Unix nanoseconds; used for tolerance aging
└── PoolTag        uint8         // 0=mainstream, 1=loser's pool, 2=retention, 3=monetize
```

**Invariants:**

- `DeckVector` must be unit-normalized at ingestion time. Any vector with
  magnitude < 1e-6 is rejected with error `ErrZeroVector`.
- `ConsecLosses` is always ≤ 0. `ConsecWins` is always ≥ 0.
- `EnqueuedAt` is set by the Ingestion Engine and is never mutated after pool
  assignment.
- `PoolTag` is set atomically by EOMM and must be read with `atomic.LoadUint32`
  cast.

**Allocation strategy:** Tickets are exclusively sourced from a `sync.Pool`. The
pool's `New` function allocates a zeroed `Ticket`. Callers reset fields
explicitly before returning to pool. Hot path target: `0 B/op` after warm-up.

### 2.2 `MatchResult`

```
MatchResult
├── MatchID        uint64
├── PlayerA        uint64
├── PlayerB        uint64
├── PredictedWinP  float32   // P(A wins); derived from EOMM fitness function
├── PoolSource     uint8     // which pool(s) the pair was drawn from
└── AssignedAt     int64     // Unix nanoseconds
```

### 2.3 `PlayerState` — Simulation Only

```
PlayerState
├── Ticket         Ticket      // embedded (not pointer; avoids indirection on hot sim loop)
├── TiltFactor     float32     // [0.0, 1.0]; influences deck mutation probability
├── SessionWins    uint16
├── SessionLosses  uint16
└── LastMutatedAt  int64
```

---

## 3. Module A — Ingestion Engine

### 3.1 Purpose

Accept raw WebSocket connections from game clients (or simulation driver), parse
incoming JSON/binary frames into `Ticket` structs, and write those tickets into
the appropriate priority queue without blocking the connection goroutine.

### 3.2 WebSocket Pool Architecture

- Uses `github.com/coder/websocket` (zero-alloc read paths) or
  `nhooyr.io/websocket` for connection management. **Rationale:** stdlib
  `gorilla/websocket` allocates per-message; avoid.
- Each accepted connection gets exactly one goroutine (the "connection
  goroutine"). This goroutine owns the connection for its full lifetime and is
  the only writer/reader on that socket.
- Connection goroutines do **not** perform any matchmaking logic. They parse,
  validate, pool-source a `Ticket`, and write it to the lock-free ring buffer.
  Then they block on the next read.

### 3.3 Frame Parsing Contract

```
Input:  raw []byte from WebSocket frame (JSON or MessagePack; negotiated at handshake)
Output: *Ticket sourced from sync.Pool, fully populated

Validation rules (any failure → drop frame, increment DropCounter metric):
  - PlayerID must be non-zero
  - Trophies must be in [-10_000, 100_000]
  - ChurnRisk and MonetizationP must be in [0.0, 1.0]
  - DeckVector magnitude must be > 1e-6 (pre-normalization check)
  - Frame must parse within 50µs; timeout → drop + counter
```

### 3.4 Ingestion → Queue Handoff

The connection goroutine writes to a **per-CPU shard ring buffer** to minimize
cross-CPU cache contention. The ring buffer shard index is derived from
`PlayerID % NumShards`. This ensures tickets from the same player always enter
the same shard and prevents duplicate-ticket edge cases during reconnection
within a single tick window.

**Ring buffer write path allocation target:** `0 B/op`.

### 3.5 Backpressure

If all ring buffer slots for a shard are occupied (full queue):

1. Attempt a single spin-wait of 10µs.
2. If still full: drop ticket, increment `IngestionBackpressureCounter`, return
   ticket to pool.
3. **Never** block the connection goroutine for more than 10µs on queue write.

---

## 4. Module B — Match Core Loop (200ms Tick)

### 4.1 Master Ticker

A single `time.Ticker` fires every 200ms. The tick handler is the exclusive
writer to Redis and the exclusive reader of ring buffer drain output. It must
complete within 200ms to avoid tick drift.

**Tick budget allocation:**

```
Ring buffer drain:           ~5ms
ZSET query + scoring:       ~40ms
EOMM fitness evaluation:    ~80ms
Redis Lua match assignment: ~20ms
Telemetry emission:          ~5ms
Headroom / OS jitter:       ~50ms
────────────────────────────
Total:                      200ms
```

If a tick overruns 200ms, the **next tick is skipped** (not queued). An overrun
counter is incremented. Three consecutive overruns trigger a `WARN` log to
stderr.

### 4.2 Queue Segment Architecture

Players are bucketed into **trophy-range segments** stored as Redis ZSETs.
Segment boundaries:

| Segment  | Trophy Range   | ZSET Key       |
| -------- | -------------- | -------------- |
| 0        | 0 – 1,000      | `mq:seg:0`     |
| 1        | 1,001 – 3,000  | `mq:seg:1`     |
| 2        | 3,001 – 6,000  | `mq:seg:2`     |
| 3        | 6,001 – 10,000 | `mq:seg:3`     |
| 4        | 10,001+        | `mq:seg:4`     |
| losers   | cross-segment  | `mq:losers`    |
| retain   | cross-segment  | `mq:retention` |
| monetize | cross-segment  | `mq:monetize`  |

Score in ZSET = `Trophies * 1e6 + EnqueuedAt_microseconds_truncated`. This
encodes both rank and arrival time in a single 64-bit float without precision
loss for the expected trophy range.

### 4.3 Exponential Tolerance Expansion

To prevent starvation of players who cannot find a same-rank opponent within a
reasonable window:

```
Tolerance(t) = BaseTolerance × e^(k × t)
```

Where:

- `t` = seconds since `EnqueuedAt` (computed during tick, not cached in Ticket)
- `BaseTolerance` = 50 trophies (configurable at startup via env var
  `MP_BASE_TOLERANCE`)
- `k` = 0.15 (configurable via `MP_TOLERANCE_K`; controls expansion
  aggressiveness)

**Edge cases:**

- When `k × t > 10.0`, clamp `Tolerance` to `MaxTolerance = 2000` to prevent
  unlimited matching.
- When `t = 0` (player just enqueued in this tick), `Tolerance = BaseTolerance`
  exactly.
- Overflow guard: compute
  `math.Min(BaseTolerance * math.Exp(k*t), MaxTolerance)` — never store
  intermediate unbounded `float64`.

**Practical meaning:** A player waiting 30 seconds at `k=0.15` gets tolerance
`50 × e^(0.15×30) = 50 × e^4.5 ≈ 50 × 90.0 = 4,500` → clamped to 2,000 trophies.
Effectively they will match anyone in adjacent segments within ~30 seconds.

### 4.4 Match Candidate Selection

For each unmatched ticket dequeued from a segment ZSET:

1. Compute current `Tolerance(t)` for this ticket.
2. Execute Redis
   `ZRANGEBYSCORE mq:seg:<N> [score - tolerance, score + tolerance] LIMIT 0 8`
   to retrieve up to 8 candidates.
3. Score each candidate pair using the EOMM fitness function (§5.3).
4. Select the highest-scoring valid pair.
5. Execute atomic Lua assignment (§9.3) to commit the match.
6. Both tickets are removed from ZSET atomically. No double-booking is possible.

---

## 5. Module C — Engagement-Optimized Matchmaking (EOMM)

### 5.1 Pool Routing

On every tick, before segment matching, the EOMM engine routes tickets into
specialized pools:

| Condition                                  | Action                      |
| ------------------------------------------ | --------------------------- |
| `ConsecLosses <= -2`                       | Move to `mq:losers` ZSET    |
| `ChurnRisk > 0.75 AND ConsecLosses <= -1`  | Move to `mq:retention` ZSET |
| `MonetizationP > 0.80 AND ConsecWins >= 2` | Move to `mq:monetize` ZSET  |
| Otherwise                                  | Remain in segment ZSET      |

**Routing is mutually exclusive** and evaluated in the order above. A ticket
matching multiple conditions is routed to the first matching pool only.
`PoolTag` is updated atomically.

**Pool evacuation:**

- Loser's pool: player wins a match → immediately re-enqueued into segment ZSET,
  `ConsecLosses` reset to 0.
- Retention pool: player wins a match → re-enqueued into segment ZSET.
- Monetization pool: match completes regardless of outcome → re-enqueued into
  segment ZSET.

### 5.2 The Loser's Pool

Players in `mq:losers` are matched **only** against other players in
`mq:losers`. The same tolerance expansion formula applies with an independent
`BaseTolerance` of 200 trophies to allow wider cross-skill matching (losing
players tend to cluster at lower skill expressions temporarily).

**Starvation guard:** If a loser's pool player has been waiting > 10 ticks (2
seconds) without a match and the pool has fewer than 2 other players, they are
evacuated back to segment ZSET regardless of streak status.

### 5.3 EOMM Fitness Function

```
MatchScore = w1×|ΔTrophies| + w2×VectorDistance + w3×RetentionWeight
```

Where:

- `w1 = 0.4` — trophy proximity penalty (lower is better; normalized to [0,1]
  over MaxTolerance)
- `w2 = 0.3` — deck vector distance (cosine distance: `1.0 - CosineSimilarity`;
  see §6)
- `w3 = 0.3` — retention/monetization weight (see below)
- All three weights sum to 1.0. They are configurable at startup but must sum to
  1.0 or system panics with `ErrWeightMismatch`.

**RetentionWeight** per match pair `(A, B)`:

| Scenario                                    | Value for pair                                                      |
| ------------------------------------------- | ------------------------------------------------------------------- |
| Neither player is high churn risk           | 0.0 (no modifier)                                                   |
| Player A is high churn (`ChurnRisk > 0.75`) | Add 0.5 if B's trophies < A's by ≥ 100 (easier target)              |
| Player A is in monetize pool                | Subtract 0.4 if B is a structural counter (cosine similarity < 0.2) |

**Target win probabilities** (not enforced mechanically; EOMM selects opponents
to make these probable based on trophy/vector distributions):

- Retention match: `P(win) ≈ 0.7` — achieved by selecting opponent with 80–120
  fewer trophies.
- Monetization trigger: `P(win) ≈ 0.3` — achieved by selecting opponent with
  structural counter archetype and 50–100 more trophies.

### 5.4 Adaptive Spike Detection

The EOMM engine tracks a rolling 10-tick window of each player's win rate. If a
player's `RollingWinRate` drops below 0.3 AND `ChurnRisk` crosses 0.75 within
the same tick:

- `PoolTag` is updated to `2` (retention).
- A `ChurnAlertEvent` is emitted to the telemetry ring buffer.

---

## 6. Module D — Deck Vector Archetype Engine

### 6.1 Archetype Dimensions

Each player's deck is represented as an 8-dimensional unit vector:

```
Index  Dimension       Description
0      Tank            High-HP frontline units
1      AreaDamage      Splash/AoE damage dealers
2      FastCycle       Low-elixir cycling cards
3      Control         Stall, freeze, or displacement effects
4      Range           Long-range or air targeting
5      Spell           Direct damage spells
6      Spawner         Cards that produce multiple units
7      Assassin        High single-target burst damage
```

**Normalization:** After assigning raw scores to each dimension (based on card
attributes), the vector is L2-normalized: `v_normalized = v / ||v||`. If
`||v|| < 1e-6`, ingestion rejects the deck with `ErrZeroVector`.

### 6.2 Cosine Similarity

```
CosineSimilarity(A, B) = dot(A.DeckVector, B.DeckVector)
                       = Σ (A[i] × B[i]) for i in [0,7]
```

Since vectors are pre-normalized at ingestion, no division is required at match
time. The dot product is the full similarity. This is the primary optimization:
normalization cost is paid once at ingestion; match-time similarity is 8
multiplications + 7 additions = 15 FLOPs.

**Result range:** `[-1.0, 1.0]`. In practice, all deck dimension scores are
non-negative, so the range is `[0.0, 1.0]` in production. Document the general
case for simulation correctness.

**Cosine distance** (used in fitness function):
`CosineDistance = 1.0 - CosineSimilarity`.

### 6.3 Counter Archetype Detection

A pair is a "structural counter" if `CosineSimilarity < 0.2`. This threshold is
configurable via `MP_COUNTER_THRESHOLD`. Counter pairs are selected for
monetization trigger matches.

A pair is "stylistically similar" if `CosineSimilarity > 0.75`. This is used as
a secondary preference signal in segment matching to create engaging mirrors.

### 6.4 SIMD Consideration

For the simulation harness evaluating 100k players, the dot product loop over
`[8]float32` is a candidate for manual SIMD via Go assembly or `unsafe`
intrinsics. The planning subagent should document this as an optimization escape
hatch, but the baseline implementation must use pure Go for correctness
verification. Review benchmarks will flag if the dot product path becomes a
top-5 CPU frame.

---

## 7. Module E — Macro-Simulation Harness

### 7.1 Purpose

Drive up to 100,000 concurrent simulated players through the full matchmaking
stack, validating system behavior under load, measuring end-to-end latency
distributions, and collecting EOMM accuracy telemetry.

### 7.2 Player State Machine

Each simulated player is an independent goroutine running a state machine:

```
States: QUEUED → WAITING → MATCHED → PLAYING → POST_MATCH → [QUEUED | QUIT]

Transitions:
  QUEUED      → WAITING:    Ticket written to ingestion ring buffer
  WAITING     → MATCHED:    MatchResult received on player's result channel
  MATCHED     → PLAYING:    Simulated match duration: Gaussian(μ=180s, σ=30s)
  PLAYING     → POST_MATCH: Outcome resolved (win/loss via PredictedWinP + noise)
  POST_MATCH  → QUEUED:     If session losses < MaxSessionLosses (configurable)
  POST_MATCH  → QUIT:       If ChurnRisk > rand.Float32() [probabilistic churn]
```

### 7.3 Behavioral Mutation

After each `POST_MATCH` transition:

1. **Tilt update:** `TiltFactor += 0.15` on loss, `TiltFactor -= 0.10` on
   win`. Clamp`[0.0, 1.0]`.
2. **Deck mutation:** With probability `TiltFactor * 0.3`, randomly shift one
   DeckVector dimension by ±0.1, then re-normalize. This models deck-switching
   behavior under frustration.
3. **Churn update:** `ChurnRisk = 0.7*ChurnRisk + 0.3*TiltFactor`. Exponential
   moving average.
4. **Trophy update:** `Trophies += win ? +30 : -20`. Clamp to `[0, 100_000]`.

### 7.4 Goroutine Management

- Players are launched with `errgroup.Group` with a configurable concurrency
  ceiling (`MP_SIM_CONCURRENCY`, default 100_000).
- Each player goroutine has a dedicated result channel of capacity 1. The match
  core loop writes `MatchResult` to this channel non-blockingly; if the channel
  is full (player hasn't consumed previous result), the result is dropped and a
  `SimDropCounter` is incremented.
- Goroutine stacks are pre-sized to 4KB by ensuring player state fits in ~200
  bytes (verified by `unsafe.Sizeof` test assertion).

### 7.5 Simulation Parameters (defaults, all configurable via env)

| Parameter           | Default | Env Var                  |
| ------------------- | ------- | ------------------------ |
| Concurrent players  | 100,000 | `MP_SIM_CONCURRENCY`     |
| Simulation duration | 600s    | `MP_SIM_DURATION`        |
| Match duration μ    | 180s    | `MP_MATCH_DURATION_MEAN` |
| Match duration σ    | 30s     | `MP_MATCH_DURATION_STD`  |
| Max session losses  | 10      | `MP_MAX_SESSION_LOSSES`  |
| Tick rate           | 200ms   | `MP_TICK_RATE`           |

---

## 8. Module F — Telemetry & Visualizer Bridge

### 8.1 Telemetry Event Types

All events are fixed-size structs to enable ring-buffer storage without
allocation:

```
TelemetryEvent
├── EventType    uint8       // enum: QueueDepth, MatchMade, ChurnAlert, AllocSnapshot
├── Timestamp    int64       // Unix nanoseconds
├── Segment      uint8       // which queue segment (0-4, or 255 for system-level)
├── Value1       float32     // primary metric value
└── Value2       float32     // secondary metric value (event-type-specific)
```

Total size: 18 bytes. Ring buffer capacity: 65,536 slots = ~1.1MB. At 200ms tick
rate with ~500 events/tick, this gives ~26 seconds of history.

### 8.2 Ring Buffer Architecture

Telemetry uses an **SPSC (single-producer, single-consumer) lock-free ring
buffer** per metric category. The match core loop is the sole producer; the
WebSocket emitter goroutine is the sole consumer. This SPSC constraint
eliminates the need for CAS loops, enabling pure load/store ordering with
`atomic.LoadUint64` / `atomic.StoreUint64` on the head and tail indices.

**Write path:** If the ring buffer is full, the oldest event is overwritten
(oldest-first eviction). This is preferable to dropping the newest event because
the visualizer prioritizes current state over historical accuracy.

### 8.3 Web Frontend Bridge

A single WebSocket server goroutine reads from all telemetry ring buffers and
fans out updates to connected visualizer clients. Protocol: newline-delimited
JSON. Frame rate: 10Hz (100ms emit interval). Each frame is a JSON object:

```json
{
    "ts": 1718000000000000000,
    "queueDepths": [120, 340, 89, 12, 3],
    "matchesLastTick": 47,
    "eommAccuracy": 0.82,
    "allocBytesHeap": 4194304,
    "churnAlerts": 3
}
```

### 8.4 Visualizer Frontend Contract

The frontend is a single-page HTML/JS application (no build step). It connects
via WebSocket to the Go server at `ws://localhost:8080/telemetry`. It renders:

- Real-time bar chart of queue depths per segment.
- Rolling line chart of match rate (matches/tick).
- Heap allocation gauge (target: flat/stable line).
- EOMM accuracy ratio (actual `P(win) ≈ target P(win)` within ±0.1 tolerance).

---

## 9. Storage Layer Contract

### 9.1 Redis Version Requirement

Redis 7.0+ required for `LMPOP` and enhanced Lua scripting. Connection via
`go-redis/v9`. Single-instance Redis; no cluster mode (consistent with
single-node philosophy).

### 9.2 Key Schema

```
mq:seg:<0-4>      ZSET   Segment priority queues (score = trophy composite)
mq:losers         ZSET   Loser's pool
mq:retention      ZSET   Retention trigger pool
mq:monetize       ZSET   Monetization trigger pool
mq:player:<id>    HASH   Player metadata (ChurnRisk, MonetizationP, ConsecLosses)
mq:match:<id>     HASH   Match record (TTL: 3600s)
mq:locks          SET    Active match locks (for Lua atomic guard)
```

### 9.3 Atomic Match Assignment (Lua Script)

```lua
-- KEYS[1] = source ZSET key
-- KEYS[2] = source ZSET key (for second player, may equal KEYS[1])
-- ARGV[1] = playerA member string
-- ARGV[2] = playerB member string
-- ARGV[3] = matchID string
-- Returns 1 on success, 0 if either player already removed (race guard)

local a = redis.call('ZSCORE', KEYS[1], ARGV[1])
local b = redis.call('ZSCORE', KEYS[2], ARGV[2])
if a == false or b == false then return 0 end
redis.call('ZREM', KEYS[1], ARGV[1])
redis.call('ZREM', KEYS[2], ARGV[2])
redis.call('HSET', 'mq:match:' .. ARGV[3], 'playerA', ARGV[1], 'playerB', ARGV[2])
redis.call('EXPIRE', 'mq:match:' .. ARGV[3], 3600)
return 1
```

This script is loaded via `SCRIPT LOAD` at startup and called via `EVALSHA` to
avoid re-parsing overhead. The SHA is cached in the application at startup.

### 9.4 Redis Connection Pool

- Pool size: `runtime.NumCPU() * 4` connections.
- Command timeout: 5ms. Any command exceeding 5ms increments
  `RedisLatencyCounter` and triggers a WARN log.
- Pipeline batching: the tick handler pipelines all ZRANGEBYSCORE queries for
  one tick and issues them in a single round-trip before scoring.

---

## 10. Memory & Allocation Contracts

### 10.1 Hot-Path Allocation Budget

| Function           | Budget (B/op) | Justification                                            |
| ------------------ | ------------- | -------------------------------------------------------- |
| `ParseTicket`      | 0             | Pool-sourced Ticket; MessagePack into stack buffer       |
| `RingBuffer.Write` | 0             | Pre-allocated slot array; no dynamic memory              |
| `RingBuffer.Read`  | 0             | Returns pointer into slot array                          |
| `ComputeTolerance` | 0             | Pure math; no allocations                                |
| `CosineSimilarity` | 0             | Stack-only; 8 float32 multiplications                    |
| `FitnessScore`     | 0             | Stack-only; calls above two                              |
| `TickHandler`      | < 512         | Redis pipeline allocation; justified by network boundary |
| `EmitTelemetry`    | 0             | Fixed-size struct written to ring buffer slot            |
| `SimPlayerTick`    | 0 (per tick)  | State mutation in-place; no new allocations              |

### 10.2 `sync.Pool` Objects

| Pool                 | Object            | Reset Function                    |
| -------------------- | ----------------- | --------------------------------- |
| `ticketPool`         | `*Ticket`         | Zero all fields except `PlayerID` |
| `matchResultPool`    | `*MatchResult`    | Zero all fields                   |
| `telemetryEventPool` | `*TelemetryEvent` | Zero all fields                   |

Pool reset functions must be called before returning objects and must be
verified by a test that checks all fields are zero after reset (sentinel test:
assign non-zero values, reset, assert zero).

### 10.3 Struct Alignment

All hot-path structs must be verified for optimal field ordering using
`fieldalignment` (golang.org/x/tools/go/analysis/passes/fieldalignment). A CI
check runs `fieldalignment ./...` and fails on any struct that can be compacted.
The `Ticket` struct above is pre-ordered to minimize padding.

---

## 11. Concurrency Model

### 11.1 Goroutine Budget

```
System goroutines:
  1  × Master tick goroutine
  1  × Telemetry emitter goroutine
  1  × WebSocket server (accept loop)
  N  × Connection goroutines (one per active WebSocket; max 10,000 in production)
  P  × Ring buffer drain workers (P = runtime.NumCPU())

Simulation goroutines (simulation mode only):
  100,000 × Player state machines
  1       × Simulation orchestrator
```

Total goroutine count in full simulation: ~100,012 + N_connections. At 4KB stack
floor, this is ~400MB stack space. Acceptable for bare-metal with ≥8GB RAM.

### 11.2 Shared State Inventory

| Shared resource       | Access mechanism                    | Rationale                          |
| --------------------- | ----------------------------------- | ---------------------------------- |
| Ring buffer head/tail | `atomic.Uint64`                     | SPSC; no CAS needed                |
| Pool tag on Ticket    | `atomic.Uint32` (via cast)          | Written by EOMM, read by tick loop |
| Metrics counters      | `atomic.Int64`                      | Write-heavy, read occasionally     |
| Redis ZSET state      | Lua EVAL (Redis-side serialization) | External serialization boundary    |
| `sync.Pool`           | stdlib pool (internal locking)      | Pool design amortizes lock cost    |

**Rule:** If a variable is shared between two or more goroutines, it must appear
in this table with its access mechanism documented. The review agent will audit
this table against the implementation on each module delivery.

### 11.3 Channel Discipline

- All channels have explicit capacity. Zero-capacity (synchronous) channels are
  forbidden on hot paths.
- Channel sends on hot paths must use `select` with a `default` case to remain
  non-blocking.
- No goroutine may hold an undrained channel reference after its owning context
  is cancelled. Context cancellation triggers graceful drain before goroutine
  exit.

---

## 12. Error Handling & Observability

### 12.1 Error Taxonomy

```
ErrZeroVector       — Deck vector with magnitude < 1e-6
ErrWeightMismatch   — EOMM weights do not sum to 1.0
ErrRingBufferFull   — Ring buffer at capacity; ticket dropped
ErrRedisTimeout     — Redis command exceeded 5ms budget
ErrTickOverrun      — Match core tick exceeded 200ms
ErrDualBooking      — Lua script returned 0 (race guard triggered)
ErrPoolExhausted    — sync.Pool returned nil (memory pressure)
```

All errors are typed `const` sentinel errors (`errors.New` at package init). No
dynamic error string formatting on hot paths.

### 12.2 Structured Logging

- All log output via `log/slog` (stdlib, zero-alloc text handler in production).
- Log levels: `DEBUG` (disabled in production), `INFO` (lifecycle events),
  `WARN` (budget overruns, backpressure), `ERROR` (booking failures, data races
  detected post-hoc).
- No `fmt.Sprintf` in log calls on hot paths. Use `slog.Int`, `slog.Float64`,
  etc.

### 12.3 Prometheus Metrics (optional, behind build tag `metrics`)

Expose at `/metrics`:

- `matchpoint_queue_depth{segment}` gauge
- `matchpoint_matches_total` counter
- `matchpoint_tick_duration_seconds` histogram
- `matchpoint_ingestion_backpressure_total` counter
- `matchpoint_redis_latency_seconds` histogram

---

## 13. Testing Philosophy & Coverage Floors

### 13.1 TDD Flow

```
1. Planning subagent identifies files, APIs, decisions, risks, and tests
2. Main agent writes focused tests and implementation
3. Review subagent audits the diff and may make narrow fixes
4. Main agent runs required checks
5. Coverage must be ≥ 85% on all non-generated packages
```

### 13.2 Required Test Types Per Module

| Test Type           | Purpose                               | Tooling                               |
| ------------------- | ------------------------------------- | ------------------------------------- |
| Unit (table-driven) | Behaviour spec coverage               | `testing.T`                           |
| Fuzz                | Parser and fitness function stability | `testing.F` (Go 1.21+)                |
| Benchmark           | Allocation and throughput             | `testing.B` + `benchmem`              |
| Race                | Concurrent access safety              | `go test -race`                       |
| Integration         | Redis round-trip correctness          | `testcontainers-go` (Redis container) |

### 13.3 Fuzz Targets

Required fuzz targets:

- `FuzzParseTicket`: random byte sequences; must never panic, must reject
  invalid input gracefully.
- `FuzzFitnessScore`: random float32 inputs for all weights; must never produce
  NaN or ±Inf.
- `FuzzToleranceFormula`: random `(t, k, BaseTolerance)` triples; must never
  produce negative or NaN output; clamp must always fire before MaxTolerance.

---

## 14. Delivery Sequence & Dependency Graph

```
ticket ──────────────────────────────────────────────────────────┐
  │                                                               │
ringbuffer ──► matchcore ──► eomm ──► vectorarch ──► simulation  │
                  │                                      │        │
              redisqueue                            telemetry ◄──┘
```

**Strict rule:** No module may depend on another module that is not marked
complete in the checklist below. Use narrow interfaces or local fakes for
upstream dependencies during development; replace with real implementations
after review is complete.

### 14.1 Delivery Checklist

This checklist is the source of truth for remaining feature work. Completed
items are checked and struck through. For in-progress state such as planned,
implemented, reviewing, or blocked, use `docs/SESSIONS.md`.

- [x] ~~ticket - Ticket struct, pool, and ingestion contract~~
- [x] ~~ringbuffer - Lock-free ring buffer for WebSocket decoupling~~
- [x] ~~redisqueue - Redis ZSET priority queue + Lua atomic scripts~~
- [x] ~~matchcore - 200ms tick loop + exponential tolerance expansion~~
- [x] ~~eomm - Loser's pool, retention matches, monetization triggers~~
- [x] ~~vectorarch - 8-dim archetype vector + cosine similarity engine~~
- [x] ~~simulation - 100k goroutine player state machine harness~~
- [ ] telemetry - Async ring-buffer telemetry + web frontend bridge

**Estimated module complexity:**

| Module     | Estimated LOC | Estimated TDD cycles |
| ---------- | ------------- | -------------------- |
| ticket     | 150           | 1                    |
| ringbuffer | 300           | 2                    |
| redisqueue | 400           | 2–3                  |
| matchcore  | 600           | 3–4                  |
| eomm       | 500           | 3                    |
| vectorarch | 200           | 1–2                  |
| simulation | 800           | 3                    |
| telemetry  | 350           | 2                    |
