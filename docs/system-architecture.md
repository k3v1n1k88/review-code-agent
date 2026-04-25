# System Architecture

## Overview

Review Code Agent is a Go monorepo implementing a microservice architecture with Clean Architecture patterns. The system processes code reviews asynchronously, integrating with AI models (Claude, OpenAI) and external services.

**Module:** `github.com/vng/review-code-agent`
**Go Version:** 1.24.7

## Architecture Pattern: Clean Architecture

The codebase follows Clean Architecture principles with strict separation of concerns:

```
cmd/
├── server    # HTTP API service
├── worker    # Async job processor
└── mcp       # Model Context Protocol server

internal/
├── config    # Configuration management
├── domain    # Business logic entities & interfaces
├── usecase   # Application business rules
├── repository # Data persistence abstractions
├── delivery  # HTTP handlers & presentation logic
└── infrastructure # External service clients, DB drivers

pkg/
├── logger    # Structured logging utility
├── errors    # Custom error types
└── validator # Input validation helpers
```

## Core Components

### 1. HTTP Server (`cmd/server`)
- **Framework:** Echo v4.15.1
- **Port:** 8080
- **Timeout:** 30s read/write
- **Responsibility:** REST API endpoints, request validation, response serialization

### 2. Worker (`cmd/worker`)
- **Message Queue:** RabbitMQ (AMQP)
- **Purpose:** Asynchronous job processing (code reviews, analysis tasks)
- **Concurrency:** Job-level parallelism via worker pool

### 3. MCP Server (`cmd/mcp`)
- **Protocol:** Model Context Protocol
- **Purpose:** Integration point for Claude/AI clients to interact with review system

### 4. Configuration (`internal/config`)
- **Provider:** Viper v1.21.0
- **Sources:** Environment variables, .env files
- **Pattern:** Singleton-based config loading on startup

### 5. Data Layer (`internal/repository`)
- **Database:** PostgreSQL 16 with pgvector extension
- **Abstraction:** Repository interfaces for data operations
- **Migration:** SQL schema files in `migrations/`

### 6. External Integrations
- **AI Models:** Anthropic Claude, OpenAI API
- **Message Queue:** RabbitMQ (job distribution)
- **Cache:** Redis (session caching, rate limiting)
- **CodeGraph:** Optional code analysis service
- **Slack:** Notifications (bot token configured)

## Deployment Stack

### Docker Services

| Service | Image | Purpose |
|---------|-------|---------|
| postgres | pgvector/pgvector:pg16 | Vector database for embeddings |
| rabbitmq | rabbitmq:3.13-management | Task queue, async jobs |
| redis | redis:7-alpine | In-memory cache |
| server | custom (distroless) | HTTP API |
| worker | custom (distroless) | Job processor |
| web | custom (distroless) | Frontend service |

All services use health checks for orchestration readiness.

### Multi-Stage Dockerfiles
- Location: `deploy/Dockerfile.server`, `deploy/Dockerfile.worker`, etc.
- Base: Distroless (minimal attack surface)
- Build: Go compiler + dependencies
- Runtime: Single-app binary, no shell

## Critical Configuration

Environment variables (`.env.example`):

```
HTTP_ADDR=:8080
DB_DSN=postgres://cra:cra@postgres:5432/cra?sslmode=disable
AMQP_URL=amqp://guest:guest@rabbitmq:5672/
REDIS_ADDR=redis:6379
ANTHROPIC_API_KEY=<required>
JWT_SECRET=<change-me>
API_KEY_PEPPER=<change-me>
```

## Dependency Injection

Currently uses constructor injection via factory functions. Services instantiate dependencies explicitly, enabling testability and runtime flexibility.

## Data Flow

1. **HTTP Request** → `delivery/handlers` → `usecase/services`
2. **Business Logic** → `domain/entities` (validation via `pkg/validator`)
3. **Data Access** → `repository/interfaces` → PostgreSQL
4. **Async Tasks** → `cmd/worker` (consumes RabbitMQ messages)
5. **External APIs** → Anthropic/OpenAI (configured via `internal/infrastructure`)

## Error Handling

Custom error types in `pkg/errors` with HTTP status mapping. All errors logged via structured logging (`pkg/logger/slog`).

## Observability

- **Logging:** Structured logs via Go stdlib `log/slog`
- **Tracing:** To be implemented (Phase 02+)
- **Metrics:** To be implemented (Phase 02+)

## Next Phases

- Phase 02: API endpoints, models, repository implementations
- Phase 03: Authentication, authorization, JWT validation
- Phase 04: Async job processing, worker implementation
- Phase 05+: Advanced features (tracing, distributed caching, etc.)
