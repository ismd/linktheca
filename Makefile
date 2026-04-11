.PHONY: help dev-db dev-db-down dev-db-logs run test test-unit test-integration lint tidy build clean

help:
	@echo "Available targets:"
	@echo "  dev-db          - start Postgres in Docker"
	@echo "  dev-db-down     - stop Postgres"
	@echo "  dev-db-logs     - follow Postgres logs"
	@echo "  run             - run backend locally"
	@echo "  test            - run all Go tests with race detector"
	@echo "  test-unit       - run only unit tests (skip integration)"
	@echo "  test-integration - run only integration tests"
	@echo "  lint            - go vet"
	@echo "  tidy            - go mod tidy"
	@echo "  build           - build the backend binary"
	@echo "  clean           - remove build artifacts"

dev-db:
	docker compose -f compose.dev.yaml up -d

dev-db-down:
	docker compose -f compose.dev.yaml down

dev-db-logs:
	docker compose -f compose.dev.yaml logs -f postgres

run:
	go run ./cmd/linktheca

test:
	go test ./... -race -count=1

test-unit:
	go test ./... -race -count=1 -short

test-integration:
	go test ./... -race -count=1 -run Integration

lint:
	go vet ./...

tidy:
	go mod tidy

build:
	mkdir -p bin
	go build -o bin/linktheca ./cmd/linktheca

clean:
	rm -rf bin tmp
