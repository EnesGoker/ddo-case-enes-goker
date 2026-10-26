# GNU Make 3.81 uyumlu (macOS). Karmaşık işler scripts/*.sh içinde.
GO      ?= go
COMPOSE ?= docker compose

.PHONY: help env up down logs ps test test-integration test-simulation lint build verify reset-demo simulate load chaos explain coverage

help:
	@echo "make up | down | logs | ps | test | test-integration | test-simulation | lint | build | verify | reset-demo | simulate | load | chaos | explain | coverage"

env:
	@test -f .env || (cp .env.example .env && echo ".env, .env.example'dan oluşturuldu")

up: env
	$(COMPOSE) up --build -d --wait

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f --tail=100

ps:
	$(COMPOSE) ps

test:
	cd backend && $(GO) test ./...

test-integration:
	./scripts/test-integration.sh

test-simulation:
	GP_TEST_TAGS=integration,simulation GP_TEST_TIMEOUT=20m ./scripts/test-integration.sh -run '^(TestSimulator|TestVerifyStopsBeforeBusinessRequestsDuringLive)'

lint:
	cd backend && $(GO) vet ./... && $(GO) vet -tags integration ./... && $(GO) vet -tags integration,simulation ./...
	cd frontend && npm run lint

build:
	cd backend && $(GO) build ./...
	cd frontend && npm run build

verify:
	./scripts/verify.sh

reset-demo:
	./scripts/reset-demo.sh

simulate: env
	$(COMPOSE) --profile sim run --rm --name guvenpolice-simulator --use-aliases simulator

coverage:
	cd backend && $(GO) test -count=1 -coverprofile=coverage.out ./...
	./scripts/check-coverage.sh backend/coverage.out

load: env
	./scripts/load.sh

chaos: env
	./scripts/chaos.sh

explain: env
	./scripts/explain.sh
