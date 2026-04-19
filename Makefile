.PHONY: build run test lint up down migrate fmt

BINARIES := server worker mcp

build:
	go build ./cmd/...

run:
	go run ./cmd/server

test:
	go test ./...

lint:
	go vet ./...

fmt:
	gofmt -w .

up:
	docker compose up --build -d

down:
	docker compose down

migrate:
	docker compose run --rm server migrate -path migrations -database $${DB_DSN} up
