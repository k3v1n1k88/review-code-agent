.PHONY: build run test lint up down migrate fmt tidy

BINDIR := bin

build:
	go build -o $(BINDIR)/server ./cmd/server
	go build -o $(BINDIR)/worker ./cmd/worker
	go build -o $(BINDIR)/mcp    ./cmd/mcp

run:
	go run ./cmd/server

test:
	go test ./...

lint:
	go vet ./...
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed, skipping"

fmt:
	gofmt -w .

tidy:
	go mod tidy

up:
	docker compose up --build -d

down:
	docker compose down

migrate:
	@echo "Migrations will be wired in Phase 02"
