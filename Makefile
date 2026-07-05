SHELL := /bin/sh

ADDR ?= :8080
REDIS_ADDR ?= localhost:6380
REDIS_PASSWORD ?=
PLAYERS ?= 100000
ROUNDS ?= 16
SEED ?= 42
SIM_API_PLAYERS ?= 10000
SIM_API_ROUNDS ?= 3
SIM_API_SEED ?= 42
SERVICE_URL ?= http://localhost$(ADDR)

.PHONY: help
help:
	@printf '%s\n' 'MatchPoint commands:'
	@printf '  %-18s %s\n' 'make redis-up' 'Start local Redis on localhost:6380'
	@printf '  %-18s %s\n' 'make redis-down' 'Stop local Redis'
	@printf '  %-18s %s\n' 'make server' 'Run service and embedded UI'
	@printf '  %-18s %s\n' 'make dev' 'Start Redis, then run service'
	@printf '  %-18s %s\n' 'make sim' 'Run CLI simulation'
	@printf '  %-18s %s\n' 'make smoke' 'Check healthz and simulation API against a running service'
	@printf '  %-18s %s\n' 'make frontend-build' 'Build embedded React telemetry UI'
	@printf '  %-18s %s\n' 'make frontend-dev' 'Run Vite dev server for UI work'
	@printf '  %-18s %s\n' 'make test' 'Run Go tests'
	@printf '  %-18s %s\n' 'make vet' 'Run go vet'
	@printf '  %-18s %s\n' 'make check' 'Run frontend build, Go tests, and vet'

.PHONY: redis-up
redis-up:
	docker compose up -d redis

.PHONY: redis-down
redis-down:
	docker compose down

.PHONY: server
server:
	go run ./cmd/matchpoint -addr $(ADDR) -redis $(REDIS_ADDR) -redis-password "$(REDIS_PASSWORD)"

.PHONY: dev
dev: redis-up server

.PHONY: sim
sim:
	go run ./cmd/matchpoint-sim -players $(PLAYERS) -rounds $(ROUNDS) -seed $(SEED)

.PHONY: smoke
smoke:
	curl -fsS $(SERVICE_URL)/healthz
	curl -fsS -X POST $(SERVICE_URL)/simulate \
		-H 'Content-Type: application/json' \
		-d '{"players":$(SIM_API_PLAYERS),"rounds":$(SIM_API_ROUNDS),"seed":$(SIM_API_SEED)}'

.PHONY: frontend-build
frontend-build:
	npm --prefix internal/telemetry/web run build

.PHONY: frontend-dev
frontend-dev:
	npm --prefix internal/telemetry/web run dev

.PHONY: test
test:
	go test ./... -count=1

.PHONY: vet
vet:
	go vet ./...

.PHONY: check
check: frontend-build test vet
