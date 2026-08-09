.PHONY: help dev-db dev-db-down dev-db-logs dev-run run test test-unit test-integration smoke-radar lint tidy build server-build clean

help:
	@echo "Available targets:"
	@echo "  dev-db           - start Postgres in Docker"
	@echo "  dev-db-down      - stop Postgres"
	@echo "  dev-db-logs      - follow Postgres logs"
	@echo "  dev-run          - run backend with dev database"
	@echo "  run              - run backend locally"
	@echo "  test             - run all Go tests with race detector"
	@echo "  test-unit        - run only unit tests (skip integration)"
	@echo "  test-integration - run only integration tests"
	@echo "  smoke-radar      - smoke tests with real TEI (slow)"
	@echo "  lint             - go vet"
	@echo "  tidy             - go mod tidy"
	@echo "  build            - build the backend binary"
	@echo "  clean            - remove build artifacts"

dev-db:
	docker compose -f compose.dev.yaml up -d

dev-db-down:
	docker compose -f compose.dev.yaml down

dev-db-logs:
	docker compose -f compose.dev.yaml logs -f postgres

dev-run:
	LINKTHECA_DB_DSN="postgres://linktheca:linktheca@localhost:5432/linktheca?sslmode=disable" \
	LINKTHECA_JWT_SECRET="dev-only-secret-that-is-at-least-32-bytes-long" \
	LINKTHECA_TEI_URL="http://localhost:8081" \
	LINKTHECA_RADAR_ENABLED=true \
	make run

run:
	go run ./cmd/linktheca-server

test:
	go test ./... -race -count=1

test-unit:
	go test ./... -race -count=1 -short

test-integration:
	go test ./... -race -count=1 -run Integration

smoke-radar:
	go test -tags=smoke -timeout=10m -count=1 ./internal/radar/... ./internal/core/embeddings/...

lint:
	go vet ./...

tidy:
	go mod tidy

build: server-build

server-build:
	mkdir -p bin
	go build -o bin/linktheca-server ./cmd/linktheca-server

clean:
	rm -rf bin tmp
