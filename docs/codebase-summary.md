# Code Review Agent SaaS - Codebase Summary

**Project:** Code Review Agent (review-code-agent)  
**Language:** Go 1.25  
**Last Updated:** 2026-04-27

## Overview

A Go-based microservices SaaS platform for code review automation powered by AI models (Claude, OpenAI). Phase 01 establishes the foundational monorepo structure, infrastructure, and core configuration.

## Architecture

Multi-service architecture with containerized deployment:
- **Server:** HTTP API (Echo framework, port 8080)
- **Worker:** Async task processor (AMQP consumer)
- **MCP:** Model Context Protocol server
- **Web:** Frontend application (port 3000)

## Directory Structure

```
.
├── cmd/                          # Executable entry points
│   ├── server/main.go           # HTTP API server
│   ├── worker/main.go           # Async worker
│   └── mcp/main.go              # MCP server
│
├── internal/                      # Private application code
│   ├── config/                  # Configuration loading (Viper)
│   ├── domain/                  # Business logic (domain objects)
│   ├── usecase/                 # Use cases (application logic)
│   ├── repository/              # Data access layer
│   ├── delivery/                # HTTP handlers, gRPC
│   └── infrastructure/          # External service integrations
│
├── pkg/                          # Shared, reusable libraries
│   ├── logger/                  # Structured logging (slog)
│   ├── errors/                  # Error handling utilities
│   └── validator/               # Input validation (ozzo-validation)
│
├── deploy/                       # Dockerfiles for services
│   ├── Dockerfile.server        # Multi-stage, distroless
│   ├── Dockerfile.worker
│   └── Dockerfile.mcp
│
├── migrations/                   # Database migrations
├── web/                          # Frontend application
├── docker-compose.yml           # Local dev environment
├── go.mod / go.sum             # Go dependencies
├── Makefile                     # Build automation
└── .env.example                # Configuration template
```

## Infrastructure Stack

### Services (docker-compose.yml)
- **PostgreSQL 16** (pgvector) - Primary datastore, vector embeddings
- **RabbitMQ 3.13** - Message queue (AMQP)
- **Redis 7** - Caching layer
- **Server** - HTTP API container
- **Worker** - Async processor container
- **MCP** - Model Context Protocol container
- **Web** - Frontend SPA container

### Healthchecks
All infrastructure services include healthchecks. API servers depend on healthy infrastructure before starting.

## Configuration

**Environment Variables** (`.env.example`):
- HTTP server settings (addr, timeouts)
- PostgreSQL connection (DSN, connection pool)
- RabbitMQ settings (AMQP URL, prefetch)
- Redis URL
- AI model credentials (Anthropic, OpenAI)
- JWT/API key secrets
- Service URLs (Slack bot token, CodeGraph)

**Loading:** `internal/config/load.go` uses Viper to read from `.env` at startup.

## Dependencies

**Core Frameworks:**
- `echo/v4` - HTTP web framework
- `viper` - Configuration management
- `ozzo-validation/v4` - Input validation
- `slog` - Structured logging (stdlib)

**Database & Async:**
- (Phase 02+) PostgreSQL driver, pgvector client
- (Phase 02+) AMQP client, Redis client

## Build & Deployment

**Local Development:**
```bash
make build      # Compile all binaries → ./bin/
make run        # Run server locally
make test       # Run tests
make up         # Start docker-compose stack
make down       # Stop stack
make migrate    # Run DB migrations
```

**Production Deployment:**
Multi-stage Dockerfiles produce distroless images (gcr.io/distroless/static):
- Minimal attack surface, no shell or package manager
- ~5 MB container images (stripped binaries)
- Non-root user (nonroot:nonroot)

## Next Phases

- **Phase 02:** Database schema, repository layer
- **Phase 03+:** Domain models, use cases, API endpoints
- **Phase 04+:** Worker tasks, MCP integration
- **Phase 05+:** Frontend scaffolding
- **Phase 06+:** Tests, validation, security hardening
