.PHONY: build run test lint up down migrate fmt vet

BINARY_DIR := bin

build:
	go build -o $(BINARY_DIR)/server ./cmd/server
	go build -o $(BINARY_DIR)/worker ./cmd/worker
	go build -o $(BINARY_DIR)/mcp    ./cmd/mcp

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
	go run ./cmd/migrate

clean:
	rm -rf $(BINARY_DIR)
