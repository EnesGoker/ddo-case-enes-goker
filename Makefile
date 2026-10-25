# GNU Make 3.81 uyumlu (macOS). Karmaşık işler scripts/*.sh içinde.
GO      ?= go
COMPOSE ?= docker compose

.PHONY: help env up down logs ps test test-integration lint build verify reset-demo simulate

help:
	@echo "make up | down | logs | ps | test | test-integration | lint | build | verify | reset-demo | simulate"

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

lint:
	cd backend && $(GO) vet ./... && $(GO) vet -tags integration ./...
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
