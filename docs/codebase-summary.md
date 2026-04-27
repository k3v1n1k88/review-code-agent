# Codebase Summary — review-code-agent

**Last Updated:** 2026-04-27 | **Status:** Phase 01 Complete

---

## Project Overview

**review-code-agent** is a Go monorepo implementing an AI-powered code review system with agent-based analysis, RabbitMQ job queue processing, and an MCP (Model Context Protocol) interface for integration with Claude and other AI platforms.

**Module:** `github.com/vng/review-code-agent`  
**Language:** Go 1.25.5  
**Architecture:** Clean Architecture with three independent binaries sharing core packages.

---

## Directory Structure

```
review-code-agent/
├── cmd/                           # Binary entry points
│   ├── server/main.go            # HTTP API server (Echo framework)
│   ├── worker/main.go            # RabbitMQ consumer for async jobs
│   └── mcp/main.go               # MCP server for Claude integration
├── internal/                       # Private application code
│   ├── config/                   # Configuration management
│   │   ├── config.go             # Config struct definition
│   │   └── load.go               # Viper-based config loader
│   ├── domain/                   # Domain models (Phase 04+)
│   ├── usecase/                  # Business logic (Phase 04/05+)
│   ├── repository/               # Data access layer (Phase 02+)
│   ├── delivery/                 # HTTP/MCP handlers (Phase 04/07/08)
│   └── infrastructure/           # External service integrations (Phase 05+)
├── pkg/                           # Reusable public packages
│   ├── logger/slog.go            # JSON structured logging wrapper
│   ├── errors/errors.go          # Domain error types
│   └── validator/validator.go    # Input validation wrapper (ozzo-validation)
├── migrations/                    # SQL migration files (Phase 02+)
├── web/                           # Frontend application (Phase 03+)
├── deploy/                        # Deployment configuration
│   ├── Dockerfile.server         # Multi-stage build for HTTP server
│   ├── Dockerfile.worker         # Multi-stage build for worker
│   └── Dockerfile.mcp            # Multi-stage build for MCP server
├── docker-compose.yml            # Full dev stack orchestration
├── go.mod / go.sum               # Go module dependencies
├── Makefile                       # Build targets and utilities
├── .env.example                  # Environment variable template
└── .dockerignore / .gitignore    # Build and VCS ignore rules
```

---

## Core Packages

### `internal/config`

Centralized configuration management using Viper for environment-based overrides.

**Struct Fields:**
- **HTTP:** `Addr`, `ReadTimeout`, `WriteTimeout`
- **DB:** `DSN`, `MaxOpenConns`
- **AMQP:** `URL`, `Prefetch` (RabbitMQ connection pool)
- **AI:** `AnthropicKey`, `OpenAIKey`, `DefaultModel`
- **Auth:** `JWTSecret`, `APIKeyPepper`
- **Slack:** `BotToken`
- **CodeGraph:** `URL`, `Token`

**Key Function:** `Load() *Config` — reads from `.env` file + environment variables.

### `pkg/logger`

JSON structured logging wrapper around Go's `log/slog` package.

**Exports:**
- `Default() *slog.Logger` — default logger instance
- Works with standard slog methods: `Info()`, `Error()`, `Warn()`, `Debug()`
- Output format: JSON with structured fields for parsing

### `pkg/errors`

Domain-specific error types for consistent error handling across the system.

**Error Types:**
- `NotFoundError` — resource not found (HTTP 404)
- `ValidationError` — input validation failure (HTTP 400)
- `UnauthorizedError` — auth/permission failure (HTTP 401)
- `ConflictError` — resource conflict (HTTP 409)
- Generic error wrapping with message and underlying cause

### `pkg/validator`

Wrapper around `ozzo-validation/v4` for input validation with convenience exports.

**Pattern:** Compose validators for request DTO validation with clear error messages.

---

## Binaries

### `cmd/server`

**Purpose:** HTTP API server exposing REST endpoints for code review submission, status polling, and webhook events.

**Framework:** Echo v4 (lightweight, zero-copy router)

**Key Middleware:**
- `Recover()` — panic recovery
- `RequestID()` — request tracing

**Endpoints:**
- `GET /healthz` — service health check (returns `{"status":"ok"}`)

**Future Endpoints (Phase 04+):**
- Code review submission APIs
- Review status and result retrieval
- Webhook management

**Startup Flow:**
1. Load config via `config.Load()`
2. Initialize logger
3. Create Echo server with middleware
4. Register routes and handlers
5. Start HTTP listener (non-blocking)
6. Block on OS signal (SIGINT/SIGTERM)
7. Graceful shutdown with 10s timeout

### `cmd/worker`

**Purpose:** Background job processor consuming RabbitMQ messages for async code review execution.

**Stub Status:** Preloads config, initializes logger, ready for Phase 09 queue integration.

**Future Implementation:**
- RabbitMQ consumer connection
- Job deserialization
- Domain usecase invocation
- Result persistence

### `cmd/mcp`

**Purpose:** MCP (Model Context Protocol) server for Claude and other AI platform integration via stdin/stdout transport.

**Framework:** `mark3labs/mcp-go` (Go SDK)

**Server Configuration:**
- Name: `"review-code-agent"`
- Version: `"0.1.0"`

**Stub Status:** Server initialization in place, ready for Phase 08 tools/resources.

---

## Dependencies

**Core Framework:**
- `github.com/labstack/echo/v4` — HTTP server
- `github.com/spf13/viper` — Configuration management
- `github.com/rabbitmq/amqp091-go` — RabbitMQ client

**Validation & Utilities:**
- `github.com/go-ozzo/ozzo-validation/v4` — Input validation
- `github.com/mark3labs/mcp-go` — MCP SDK for Claude integration
- `golang.org/x/time/rate` — Rate limiting (imported for Phase 12)

**Build & Runtime:**
- Go 1.25.5+ required
- Multi-stage Docker builds (Alpine for build, distroless for runtime)
- Image size: <50MB per binary (optimized with `-s -w` flags)

---

## Infrastructure & Deployment

### Docker Compose Stack

**Services:**
1. **postgres:pgvector:pg16** — Vector database with pgvector extension
   - Credentials: `cra:cra` / database `cra`
   - Healthcheck: `pg_isready` (5s interval, 10 retries)
   - Volume: `pgdata` (persistent storage)

2. **rabbitmq:3.13-management** — Message broker
   - Ports: `5672` (AMQP), `15672` (Management UI)
   - Healthcheck: `rabbitmq-diagnostics -q ping` (10s interval)

3. **redis:7-alpine** — Caching and rate limiting store
   - Port: `6379`
   - Healthcheck: `redis-cli ping` (5s interval)

4. **server** — HTTP API service
   - Build: `deploy/Dockerfile.server`
   - Port: `8080`
   - Depends on: postgres (healthy), rabbitmq (healthy)
   - Healthcheck: HTTP GET `/healthz` (10s interval)

5. **worker** — Async job processor
   - Build: `deploy/Dockerfile.worker`
   - Depends on: rabbitmq (healthy)

6. **web** — Frontend application (Phase 03+)
   - Build: `./web` (separate application)
   - Port: `3000`
   - Depends on: server (healthy)

### Environment Configuration

**Key Variables** (from `.env.example`):
- `HTTP_ADDR=:8080` — Server listen address
- `DB_DSN` — PostgreSQL connection string
- `AMQP_URL` — RabbitMQ connection string
- `REDIS_URL` — Redis connection string
- `ANTHROPIC_API_KEY` — Anthropic API credentials
- `OPENAI_API_KEY` — OpenAI API credentials
- `JWT_SECRET` — JWT signing key (must change in production)
- `API_KEY_PEPPER` — API key hashing salt
- `SLACK_BOT_TOKEN` — Slack integration token
- `CODEGRAPH_URL` / `CODEGRAPH_TOKEN` — CodeGraph service credentials

---

## Build & Local Development

### Makefile Targets

```bash
make build          # Compile all three binaries
make run            # Start server (requires env setup)
make test           # Run full test suite
make lint           # Run linters and go vet
make up             # Start docker-compose stack
make down           # Stop docker-compose stack
make migrate        # Apply database migrations
make fmt            # Format code
```

### Quick Start

```bash
# Copy and configure env
cp .env.example .env

# Start dev stack
docker compose up -d

# Wait for healthchecks, then test
curl localhost:8080/healthz  # {"status":"ok"}

# Stop
docker compose down
```

---

## Code Standards & Patterns

**Architecture:**
- Clean Architecture: cmd → delivery → usecase → repository → domain
- Dependency injection via config and constructor parameters
- No global state except logger and config singletons

**Error Handling:**
- Domain errors exported from `pkg/errors`
- HTTP status code mapping at delivery layer
- Logging at infrastructure layer (repository, service boundaries)

**Logging:**
- JSON structured logging via `pkg/logger`
- Log at decision points: entry/exit, errors, state changes
- Include relevant context (user ID, request ID, resource ID)

**Testing:**
- Unit tests co-located with source files (`*_test.go`)
- Integration tests in `testdata/` or separate `internal/test` package
- Fixtures in `testdata/` directory
- Mock external services (DB, API, queue)

**Security:**
- No secrets in code; all via `.env` (gitignored)
- Distroless runtime images (non-root user)
- JWT for API authentication (Phase 05+)
- Rate limiting via Redis (Phase 12)

---

## Phase Timeline

| Phase | Status | Focus | Dependencies |
|-------|--------|-------|--------------|
| **01** | ✓ DONE | Go scaffold, Docker setup | None |
| **02** | *Planned* | Database models, migrations | Phase 01 |
| **03** | *Planned* | Dashboard scaffold (Next.js) | Phase 01 |
| **04** | *Planned* | Core domain & API endpoints | Phase 01, 02 |
| **05** | *Planned* | Review engine logic | Phase 04 |
| **06** | *Planned* | Dashboard features | Phase 03, 04 |
| **07** | *Planned* | Webhook delivery system | Phase 04 |
| **08** | *Planned* | MCP tools & resources | Phase 04 |
| **09** | *Planned* | RabbitMQ queue integration | Phase 04, 07 |
| **10** | *Planned* | Feedback loop & scoring | Phase 05 |
| **11** | *Planned* | Slack integration | Phase 04 |
| **12** | *Planned* | Usage tracking & rate limits | Phase 04 |
| **13** | *Planned* | Integration tests | Phase 05+ |
| **14** | *Planned* | Security & performance | Phase 05+ |

---

## Key Integration Points

**PostgreSQL:**
- Schema defined in `migrations/` (Phase 02)
- Accessed via repository pattern in `internal/repository/` (Phase 02)
- pgvector extension for embedding-based search (Phase 05+)

**RabbitMQ:**
- Job queue for async review processing (Phase 09)
- Consumer implemented in `cmd/worker` (Phase 09)
- Publishers in `internal/delivery` (Phase 04+)

**Redis:**
- Session/token caching (Phase 05)
- Rate limit counters (Phase 12)
- Temporary file store (Phase 07+)

**Claude/AI APIs:**
- Anthropic and OpenAI key management via config (Phase 08)
- MCP server for tool invocation (Phase 08)
- Direct API calls from usecase layer (Phase 05+)

**Slack:**
- Bot token configured via `.env` (Phase 11)
- Integration handler in `internal/delivery` (Phase 11)

---

## Maintenance & Monitoring

**Health Checks:**
- All services include Docker Compose health checks
- Server `GET /healthz` endpoint for container orchestrators
- Application-level health: DB connectivity, cache connectivity (Phase 04+)

**Logging:**
- All services emit JSON logs to stdout (Compose captures)
- Structured fields: timestamp, level, message, context
- Parse with `jq` or log aggregation tool (ELK, Datadog, etc.)

**Version Tracking:**
- Go version pinned in `go.mod`
- Image versions pinned in `docker-compose.yml`
- Module version locks via `go.sum`

---

## Known Limitations & TODOs

- **Schema Migrations:** No migration system yet (Phase 02 will introduce `golang-migrate`)
- **Test Coverage:** Zero tests at Phase 01 (will ramp in Phase 04+)
- **Error Logging:** Not all error paths log yet (delivery layer only logs on success, Phase 04)
- **Rate Limiting:** Imported but not integrated (Phase 12)
- **Database:** Connection pooling configured but unused (Phase 02)

---

## External Resources

- **Go Clean Architecture:** See research in `plans/260418-code-review-agent/` and CLAUDE.md
- **Echo Framework:** https://echo.labstack.com
- **Viper Configuration:** https://github.com/spf13/viper
- **pgvector:** https://github.com/pgvector/pgvector
- **RabbitMQ Go Client:** https://github.com/rabbitmq/amqp091-go
- **MCP Protocol:** https://modelcontextprotocol.io

---

## Quick Reference

| Concern | Implementation | Location |
|---------|---|----------|
| Config loading | Viper + env override | `internal/config/` |
| Logging | slog JSON wrapper | `pkg/logger/` |
| Error handling | Domain error types | `pkg/errors/` |
| HTTP server | Echo framework | `cmd/server/` |
| Job queue | RabbitMQ stub | `cmd/worker/` |
| MCP integration | MCP server stub | `cmd/mcp/` |
| Docker images | Multi-stage distroless | `deploy/` |
| Local dev | Docker Compose full stack | `docker-compose.yml` |
| Environment | Template with example values | `.env.example` |

