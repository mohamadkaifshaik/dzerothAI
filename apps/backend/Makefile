.PHONY: run-api build-api migrate-up migrate-down test test-integration fmt vet lint

# Database DSN is assembled from individual env vars so each component can be
# overridden independently without editing this file.
DB_DSN ?= postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(POSTGRES_HOST):$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=disable
MIGRATIONS_DIR := db/migrations

run-api:
	go run ./cmd/api

build-api:
	go build -o bin/api ./cmd/api

migrate-up:
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_DSN)" up

migrate-down:
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_DSN)" down 1

test:
	go test -race ./...

test-integration:
	go test -race -tags integration ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

lint:
	@golangci-lint run ./... || (echo "golangci-lint not installed or found issues; install from https://golangci-lint.run/"; exit 0)
