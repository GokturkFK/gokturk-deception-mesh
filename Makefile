.PHONY: build test lint fmt vet run-control-api run-sensor-ssh migrate-up migrate-down docker-up docker-down clean

GO ?= go
GOLANGCI_LINT ?= golangci-lint
MIGRATE ?= goose
DB_DSN ?= postgres://gokturk:gokturk@localhost:5432/gokturk?sslmode=disable
COMPOSE_FILE ?= deployments/docker/docker-compose.yml

build:
	$(GO) build ./...

test:
	$(GO) test -race -coverprofile=coverage.txt ./...

lint:
	$(GOLANGCI_LINT) run ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

run-control-api:
	$(GO) run ./cmd/control-api

run-sensor-ssh:
	$(GO) run ./cmd/sensor-ssh

migrate-up:
	$(MIGRATE) -dir migrations postgres "$(DB_DSN)" up

migrate-down:
	$(MIGRATE) -dir migrations postgres "$(DB_DSN)" down

docker-up:
	docker compose -f $(COMPOSE_FILE) up --build

docker-down:
	docker compose -f $(COMPOSE_FILE) down -v

clean:
	rm -rf bin dist coverage.txt coverage.html
