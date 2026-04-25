.PHONY: build run test lint up down migrate fmt vet

BINARY_SERVER=bin/server
BINARY_WORKER=bin/worker
BINARY_MCP=bin/mcp

build:
	go build -o $(BINARY_SERVER) ./cmd/server
	go build -o $(BINARY_WORKER) ./cmd/worker
	go build -o $(BINARY_MCP) ./cmd/mcp

run:
	go run ./cmd/server

test:
	go test ./...

lint:
	golangci-lint run ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

up:
	docker compose up --build -d

down:
	docker compose down

migrate:
	@echo "Run: go run ./cmd/migrate (not yet implemented)"
