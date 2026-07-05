# MatchPoint

MatchPoint is a single-node matchmaking infrastructure service for competitive
games: queue intake, Redis-backed match assignment, deck-aware scoring,
engagement routing, simulation, and live telemetry in one Go backend.

Think of the queueing layer behind games like Clash Royale or Fortnite, but
compressed into a bare-metal-friendly prototype that can be read, benchmarked,
and run locally. Players enter with trophies, streak state, churn/spend signals,
and an 8-slot deck archetype vector. MatchPoint moves those tickets through
bounded in-memory handoff, Redis sorted-set queues, Lua guarded assignment, and
a 200ms match-core tick that is instrumented end to end.

```text
client tickets
     |
     v
WebSocket intake -> ring-buffer shards -> 200ms match core
                                      |       |
                                      |       +-> EOMM scoring
                                      |       +-> deck vector similarity
                                      v
                           Redis ZSET queues + Lua assignment
                                      |
                                      v
                       telemetry ring -> WebSocket -> ops UI
```

## So What?

Most matchmaking demos stop at "pair two players with similar rating." That is
not where production systems get hard. MatchPoint is about the rest of the
problem: queue pressure, bounded tick budgets, duplicate assignment, deck
composition, retention pools, churn alerts, allocator pressure, and the question
every infra team eventually asks: can we prove the loop is behaving?

This repo is the proof harness. It gives you a runnable matchmaking service, a
macro simulation path for large player populations, and benchmark/report data
that shows which hot paths stay allocation-free.

## What The Service Supports

| Layer | What is implemented |
| --- | --- |
| Ticket intake | Player identity, trophies, streaks, churn risk, monetization probability, pool tags, and normalized 8-dimensional deck vectors |
| Queue handoff | Sharded ring buffers with duplicate-publisher protection, bounded backpressure, snapshots, drain operations, and `0 B/op` hot paths |
| Redis queues | Trophy segments, pool keys, score encoding, candidate fetches, movement between pools, and Lua-backed dual-booking protection |
| Match core | A deterministic 200ms tick with drain, enqueue, candidate query, scoring, assignment, skip-on-overrun behavior, and metrics emission |
| Deck intelligence | Archetype vectors for beatdown, control, cycle, swarm, spell pressure, air pressure, building pressure, and champion pressure style matching |
| EOMM routing | Mainstream, loser, retention, and monetization paths with starvation evacuation, churn spike detection, and retention-oriented scoring |
| Simulation | Player sessions, match outcomes, tilt, churn, trophy movement, deck mutation, session exits, and convergence gates |
| Telemetry | Fixed-size async event ring, 10Hz WebSocket frames, embedded React/Tailwind visualizer, queue depth, matches/tick, heap, churn alerts, frontend simulation controls |

## Benchmarks And Telemetry

The project keeps the fast paths honest with allocation benchmarks, race runs,
contract checks, and module-level checker reports.

| Area | Current signal |
| --- | --- |
| Ticket parsing | `BenchmarkParseTicketMessagePackGOMAXPROCS1`: `84.58 ns/op`, `0 B/op`, `0 allocs/op` |
| Ring buffer write | `BenchmarkWriteTicketAcceptedGOMAXPROCS1`: `1291 ns/op`, `0 B/op`, `0 allocs/op` |
| Redis queue assignment fake | `BenchmarkAssignMatchFakeGOMAXPROCS1`: `386.4 ns/op`, `71 B/op`, `5 allocs/op` |
| Match core loop benchmark | `BenchmarkMatchLoop`: `205.8 ns/op`, `24 B/op`, `1 alloc/op` |
| EOMM scoring | `BenchmarkScoreCandidateGOMAXPROCS1`: `24.57 ns/op`, `0 B/op`, `0 allocs/op` |
| Telemetry | `65,536` event ring slots, `/telemetry` WebSocket, 10Hz visualizer frames |

The detailed audit trail lives in `reports/`. The headline is that the core
queueing, scoring, and routing paths were built around explicit allocator
budgets instead of retrofitted after the fact.

## Run It

Install the local binaries:

```sh
go install ./cmd/matchpoint ./cmd/matchpoint-sim
```

Start Redis:

```sh
docker compose up -d redis
```

Run the service against the Compose Redis port:

```sh
matchpoint -addr :8080 -redis localhost:6380
```

Or run it directly through Go:

```sh
go run ./cmd/matchpoint -addr :8080 -redis localhost:6380
```

Open the telemetry deck:

```text
http://localhost:8080/
```

The deck includes live telemetry charts, queue injection controls, and a
frontend simulation launcher. The simulation launcher calls:

```text
POST http://localhost:8080/simulate
```

with a JSON body like:

```json
{
  "players": 10000,
  "rounds": 3,
  "seed": 42
}
```

Queue intake is exposed at `ws://localhost:8080/queue`. Send frames shaped like:

```json
{
  "playerId": 1001,
  "trophies": 2400,
  "deckVector": [1, 0, 0, 0, 0, 0, 0, 0],
  "churnRisk": 0.1,
  "monetizationP": 0.2,
  "poolTag": 0,
  "consecLosses": 0,
  "consecWins": 0
}
```

## How The Live Service Communicates

The frontend talks to the Go service over three local endpoints:

| Endpoint | Direction | Purpose |
| --- | --- | --- |
| `GET /` | Browser -> service | Serves the embedded React telemetry deck |
| `WS /telemetry` | Service -> browser | Streams 10Hz telemetry frames: queue depth, core loop ticks, Redis latency, EOMM fit, heap, and match counters |
| `WS /queue` | Browser -> service | Sends JSON queue tickets into the ring buffer |
| `POST /simulate` | Browser -> service | Runs deterministic macro simulation and returns season-level results |

The live matchmaking path is:

```text
browser /queue
  -> Go WebSocket intake
  -> sharded ring buffer
  -> 200ms matchcore tick
  -> Redis sorted-set candidate queues
  -> EOMM scorer
  -> Redis Lua assignment
  -> telemetry sink
  -> browser /telemetry
```

The service logs structured lifecycle and traffic events with `slog`: startup,
Redis connection, Lua script loading, ring/matchcore/EOMM readiness, queue
WebSocket open/close accepted/rejected counts, simulation requests, simulation
completion, and shutdown.

## Key Commands

| Command | Purpose |
| --- | --- |
| `go mod download` | Install Go module dependencies |
| `go install ./cmd/matchpoint ./cmd/matchpoint-sim` | Install the service and simulation binaries locally |
| `docker compose up -d redis` | Start Redis 7 on local port `6380` |
| `go run ./cmd/matchpoint -addr :8080 -redis localhost:6380` | Run the matchmaking service and embedded telemetry UI |
| `go run ./cmd/matchpoint-sim -players 100000 -rounds 16` | Run the macro simulation with 100k simulated players |
| `curl -X POST http://localhost:8080/simulate -H 'Content-Type: application/json' -d '{"players":10000,"rounds":3,"seed":42}'` | Run the same deterministic simulation through the service API |
| `go test ./...` | Run the default fake-backed test suite |
| `MP_REDIS_INTEGRATION=1 go test ./internal/redisqueue` | Run live Redis integration tests |
| `go test ./internal/eomm -bench=. -benchmem -run='^$'` | Benchmark the EOMM scoring path |
| `go test ./internal/matchcore -bench=BenchmarkMatchLoop -benchmem -run='^$'` | Benchmark the match loop |

## Project Map

```text
cmd/matchpoint/        live service: Redis, match core, queue WS, telemetry UI
cmd/matchpoint-sim/    macro simulation runner
internal/ticket/       queue payload validation and ticket construction
internal/ringbuffer/   sharded ingestion handoff buffers
internal/redisqueue/   Redis keys, score codec, queue ops, Lua assignment
internal/matchcore/    200ms tick coordinator
internal/eomm/         engagement routing and candidate scoring
internal/vectorarch/   deck vector normalization and similarity
internal/simulation/   deterministic player-state simulation
internal/telemetry/    async metrics ring, WS frames, embedded web UI
contracts/             module contracts and stable type surfaces
reports/               checker reports, benchmark audits, race/static results
docs/                  system specs, workflow notes, session state
```

## Requirements

- Go 1.22+
- Redis 7+ for the live service and Redis integration tests
- Docker Compose if you want the included local Redis

The default test suite uses deterministic fakes and does not require Redis.

## Agent Note

If you are an agent or automation process, do not use this README as your work
ledger. Read `docs/AGENTS.md` for workflow, `docs/SESSIONS.md` for current
handoff state, and `docs/FEATURES.md` for the authoritative system checklist.
