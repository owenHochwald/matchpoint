# MatchPoint

MatchPoint is a single-node matchmaking service for competitive games. It shows
players joining a live queue, moving through a ring buffer, being searched in
Redis, scored by EOMM logic, assigned by Lua, and streamed back to a React
telemetry UI.

The project is built to answer one question clearly: can one Go node keep a
real-time matchmaking loop fast, observable, and safe under load?

```text
player joins
  -> /queue WebSocket
  -> sharded ring buffer
  -> 200ms matchcore tick
  -> Redis candidate queues
  -> EOMM scorer
  -> Redis Lua assignment
  -> /telemetry WebSocket
  -> React simulator UI
```

## What You Can Run

The embedded UI is now focused on the live matchmaking machine:

- **Burst load** sends a fixed number of players into the real `/queue` path.
- **Live refill** keeps replacing matched players with new player joins.
- **Live Player Path** visualizes the service architecture end to end.
- **Service Health** shows tick budget, Redis latency, match yield, skips, and overruns.
- **Match Engine Pulse** shows drain, search, empty-search, and assignment counters.

The older deterministic macro simulator still exists as `cmd/matchpoint-sim`
and `POST /simulate`, but it is not the primary UI path.

## Quick Start

Requirements:

- Go 1.22+
- Docker Compose
- Redis 7, provided by `docker-compose.yml` on local port `6380`

Run the live service:

```sh
make dev
```

Open:

```text
http://localhost:8080/
```

Stop Redis when done:

```sh
make redis-down
```

## Make Commands

| Command | Purpose |
| --- | --- |
| `make redis-up` | Start local Redis on `localhost:6380` |
| `make server` | Run the Go service and embedded UI |
| `make dev` | Start Redis, then run the service |
| `make smoke` | Check `/healthz` and `/simulate` against a running service |
| `make sim` | Run the CLI macro simulator |
| `make frontend-build` | Build the embedded React UI |
| `make frontend-dev` | Run Vite for frontend-only work |
| `make test` | Run Go tests |
| `make vet` | Run `go vet` |
| `make check` | Run frontend build, tests, and vet |
| `make redis-down` | Stop local Redis |

Useful overrides:

```sh
make server ADDR=:18080 REDIS_ADDR=localhost:6380
make sim PLAYERS=50000 ROUNDS=8 SEED=42
```

## Service Endpoints

| Endpoint | Purpose |
| --- | --- |
| `GET /` | Embedded React matchmaking simulator |
| `GET /healthz` | Health check |
| `WS /queue` | Player join intake |
| `WS /telemetry` | 10Hz live telemetry stream |
| `POST /simulate` | Deterministic macro simulation API |

Queue join shape:

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

## What The Metrics Mean

- **Core ticks**: 200ms match loop pulses since service start.
- **Tick time**: latest loop duration. Lower is better; budget is 200ms.
- **Redis latency**: latest Redis boundary latency.
- **Queue depth**: players waiting in visible intake lanes.
- **Total drained**: player joins moved from ring buffer into Redis ownership.
- **Total searches**: Redis candidate lookups attempted.
- **Total matches**: committed Lua match assignments.
- **Match yield / EOMM fit**: total matches divided by total candidate searches.
- **Overruns / skips**: scheduler pressure signals. These should stay at zero.

## Project Map

```text
cmd/matchpoint/        service: Redis, matchcore, queue WS, telemetry UI
cmd/matchpoint-sim/    deterministic macro simulation CLI
internal/ringbuffer/   sharded intake handoff
internal/redisqueue/   Redis keys, score codec, queues, Lua assignment
internal/matchcore/    200ms tick coordinator
internal/eomm/         engagement routing and candidate scoring
internal/telemetry/    telemetry ring, WebSocket frames, embedded web UI
internal/simulation/   macro player-state simulation
contracts/             stable module contracts
reports/               checker and benchmark reports
docs/                  system notes and agent workflow
```

## Verification

```sh
make frontend-build
make test
make vet
```

The default Go test suite uses deterministic fakes and does not require Redis.
