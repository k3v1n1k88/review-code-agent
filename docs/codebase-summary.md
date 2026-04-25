# Codebase Summary

## Project Overview

**Review Code Agent** is a Go monorepo implementing an AI-powered code review system. The application processes code reviews asynchronously, integrating with Claude and OpenAI models through a distributed architecture.

- **Module:** `github.com/vng/review-code-agent`
- **Go Version:** 1.24.7
- **Architecture:** Clean Architecture + Microservices
- **Status:** Phase 01 (Scaffold complete)

## Directory Structure

```
review-code-agent/
├── cmd/
│   ├── server/          # HTTP API service (port 8080)
│   ├── worker/          # Async job processor (RabbitMQ consumer)
│   └── mcp/             # Model Context Protocol server
├── internal/
│   ├── config/          # Configuration loading (Viper)
│   ├── domain/          # Business entities & interfaces
│   ├── usecase/         # Application business rules
│   ├── repository/      # Data persistence abstractions
│   ├── delivery/        # HTTP handlers (Echo framework)
│   └── infrastructure/  # External service integrations
├── pkg/
│   ├── logger/          # Structured logging (slog)
│   ├── errors/          # Custom error types
│   └── validator/       # Input validation (ozzo-validation)
├── migrations/          # PostgreSQL schema files
├── deploy/              # Dockerfiles (distroless)
├── web/                 # Frontend service (to be populated)
├── Makefile             # Build targets (build, test, lint, up, down)
├── docker-compose.yml   # Local dev environment (postgres, rabbitmq, redis)
├── go.mod              # Go module dependencies
├── .env.example        # Environment variable template
└── docs/               # Project documentation
    ├── system-architecture.md
    ├── code-standards.md
    └── codebase-summary.md (this file)
```

## Key Components

### HTTP Server (`cmd/server`)
- **Framework:** Echo v4.15.1
- **Port:** 8080
- **Handlers:** Registered in `internal/delivery/`
- **Status:** Scaffold complete, handlers TBD (Phase 02)

### Worker (`cmd/worker`)
- **Queue:** RabbitMQ via AMQP (v1.11.0)
- **Purpose:** Asynchronous job processing (code reviews, analysis)
- **Status:** Scaffold complete, job handlers TBD (Phase 02)

### MCP Server (`cmd/mcp`)
- **Protocol:** Model Context Protocol
- **Purpose:** AI client integration point
- **Status:** Scaffold complete, protocol handlers TBD (Phase 02)

### Configuration (`internal/config`)
- **Provider:** Viper v1.21.0
- **Files:**
  - `config.go` — Configuration struct definition
  - `load.go` — Loader function, environment-based initialization
- **Usage:** `cfg, err := config.Load()` in `main.go` files

### Utilities (`pkg/`)
- **logger/slog.go** — Structured logging wrapper around Go stdlib
- **errors/errors.go** — Custom error types for domain-specific handling
- **validator/validator.go** — Input validation utilities via ozzo-validation

## Dependencies (go.mod)

### Direct
| Package | Version | Purpose |
|---------|---------|---------|
| go-ozzo/ozzo-validation | v4.3.0 | Input validation |
| labstack/echo | v4.15.1 | HTTP framework |
| rabbitmq/amqp091-go | v1.11.0 | Message queue client |
| spf13/viper | v1.21.0 | Config management |

### Indirect (35+ transitive)
Go stdlib + supporting libs for HTTP, crypto, YAML parsing, etc.

## Infrastructure

### Docker Compose Services
```yaml
postgres:     PostgreSQL 16 with pgvector extension (port 5432)
rabbitmq:     RabbitMQ 3.13 with management UI (port 5672, 15672)
redis:        Redis 7 alpine (port 6379)
server:       HTTP API (port 8080)
worker:       Job processor
web:          Frontend (port TBD)
```

### Dockerfiles
- `deploy/Dockerfile.server` — Multi-stage, distroless server binary
- `deploy/Dockerfile.worker` — Multi-stage, distroless worker binary
- Health checks configured for service orchestration

## Build & Development

### Makefile Targets
```bash
make build      # go build → bin/server, bin/worker, bin/mcp
make run        # go run ./cmd/server (dev mode)
make test       # go test ./...
make lint       # golangci-lint run ./...
make vet        # go vet ./...
make fmt        # gofmt -w .
make up         # docker compose up --build -d
make down       # docker compose down
make migrate    # Run database migrations (TBD)
```

### Development Workflow
1. `make up` — Start Docker services (postgres, rabbitmq, redis)
2. `make build` — Compile all binaries
3. `make run` — Start HTTP server (connects to docker services)
4. Make code changes
5. `make test lint fmt` — Quality checks
6. `make down` — Stop services

## Configuration (Environment)

All config via environment variables (`.env.example`):

```
HTTP_ADDR=:8080
HTTP_READ_TIMEOUT=30s
HTTP_WRITE_TIMEOUT=30s
DB_DSN=postgres://cra:cra@postgres:5432/cra?sslmode=disable
DB_MAX_OPEN_CONNS=25
AMQP_URL=amqp://guest:guest@rabbitmq:5672/
REDIS_ADDR=redis:6379
ANTHROPIC_API_KEY=<required>
OPENAI_API_KEY=<optional>
AI_DEFAULT_MODEL=claude-opus-4-7
JWT_SECRET=<change-me>
API_KEY_PEPPER=<change-me>
SLACK_BOT_TOKEN=<optional>
CODEGRAPH_URL=http://codegraph:9000
CODEGRAPH_TOKEN=<optional>
```

## Code Standards

See `code-standards.md` for detailed conventions:

- **Naming:** CamelCase (functions), snake_case (files), UPPER_SNAKE_CASE (constants)
- **Packages:** One responsibility per package, organized by domain
- **Error Handling:** Explicit error checking, wrapped with context
- **Concurrency:** `context.Context` for all I/O, goroutines managed via usecase layer
- **Testing:** Table-driven tests, test helpers, subtests
- **Logging:** Structured logs via `pkg/logger` using Go's `log/slog`
- **Quality:** Pre-commit checks (fmt, vet, lint, test)

## System Architecture

See `system-architecture.md` for detailed design:

- **Pattern:** Clean Architecture with Clear Layers
- **Dependency Flow:** HTTP → Usecase → Domain → Repository → Database
- **External Integrations:** Anthropic, OpenAI, RabbitMQ, Redis, CodeGraph
- **Data Storage:** PostgreSQL 16 with pgvector (embeddings support)
- **Task Distribution:** RabbitMQ for async job processing

## Phase Roadmap

### Phase 01 ✓ COMPLETE
- [x] Go monorepo scaffold (cmd/{server,worker,mcp})
- [x] Docker Compose with postgres, rabbitmq, redis
- [x] Multi-stage distroless Dockerfiles
- [x] Makefile with build/test/lint/up/down
- [x] Configuration loading via Viper
- [x] Utility packages (logger, errors, validator)

### Phase 02 (TBD)
- [ ] Domain models & database schema
- [ ] Repository pattern implementations
- [ ] HTTP API endpoints (REST)
- [ ] Async job handlers (worker)

### Phase 03 (TBD)
- [ ] Authentication & authorization (JWT)
- [ ] API versioning & documentation (OpenAPI/Swagger)
- [ ] Integration with Claude/OpenAI APIs

### Phase 04+ (TBD)
- [ ] Advanced features (tracing, distributed caching)
- [ ] Monitoring & observability
- [ ] Performance optimization

## Testing Strategy

- **Unit Tests:** Isolated business logic via clean interfaces
- **Integration Tests:** Database + external services (via containers)
- **Test Command:** `make test` (runs all tests in all packages)
- **Coverage:** Aim for >70% (enforced in CI/CD, Phase 02+)

## Security Notes

- Secrets managed via environment variables (not in code)
- Distroless Docker images reduce attack surface
- Input validation via `pkg/validator`
- SQL injection prevention via prepared statements (Phase 02+)
- JWT authentication planned (Phase 03+)

## Known Limitations (Phase 01)

- No database migrations yet (will be in Phase 02)
- Handlers not yet implemented (endpoints TBD Phase 02)
- MCP protocol handlers TBD
- No authentication/authorization yet
- No tracing or distributed logging
- Frontend service structure prepared but empty

## Next Steps

1. **Implement domain models** (Phase 02): Define entities in `internal/domain/`
2. **Create database schema** (Phase 02): SQL files in `migrations/`
3. **Implement repositories** (Phase 02): Data access layer
4. **Build HTTP endpoints** (Phase 02): Handlers in `internal/delivery/`
5. **Add authentication** (Phase 03): JWT validation, RBAC

## Getting Started (for Developers)

```bash
# Clone repo
git clone <repo>
cd review-code-agent

# Start services
make up

# Build binaries
make build

# Run server
make run

# Run tests
make test

# Check code quality
make lint fmt vet

# Stop services
make down
```

## Documentation Files

- **system-architecture.md** — High-level architecture, components, deployment
- **code-standards.md** — Go conventions, naming, structure, quality rules
- **codebase-summary.md** — This file; project overview and directory guide
