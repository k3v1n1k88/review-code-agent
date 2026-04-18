# Phase 01 — Project Scaffold & Docker Compose

## Context Links
- Parent: [plan.md](plan.md)
- Research: `research/researcher-01-backend-report.md` §2 (project structure), §8 (stack)

## Overview
- **Priority**: P1
- **Status**: pending
- **Effort**: 6h
- **Group**: A (parallel — no deps)
- **Description**: Bootstrap Go monorepo, three binaries (`server`, `worker`, `mcp`), Dockerfiles, docker-compose stack (postgres+pgvector, rabbitmq, **redis**, backend, web), Makefile, env scaffolding.
<!-- Updated: Validation Session 1 - add Redis container for rate limiter -->

## Key Insights
- Clean Architecture (research §2): `cmd/`, `internal/{domain,usecase,repository,delivery,infrastructure,config}`, `pkg/`.
- One binary per concern — server (HTTP), worker (queue consumer), mcp (stdio/SSE). Shared internal packages.
- Do NOT bake secrets; `.env.example` only. Config via Viper.

## Requirements

### Functional
- `go mod init github.com/vng/review-code-agent`
- Three entrypoints compile with `go build ./cmd/...`
- `docker compose up` brings up full dev stack
- Hot reload in dev via `air` (optional, documented)

### Non-functional
- Go 1.23+
- Multi-stage Dockerfile (scratch or distroless final)
- Image < 50MB per binary
- Compose healthchecks for postgres + rabbitmq

## Architecture

```
review-code-agent/
├── cmd/
│   ├── server/main.go         # Echo HTTP
│   ├── worker/main.go         # RabbitMQ consumer
│   └── mcp/main.go            # MCP stdio transport
├── internal/
│   ├── domain/                # (Phase 04)
│   ├── usecase/               # (Phase 04/05)
│   ├── repository/            # (Phase 02)
│   ├── delivery/              # (Phase 04/07/08)
│   ├── infrastructure/        # (Phase 05/09/11)
│   └── config/
│       ├── config.go
│       └── load.go
├── pkg/
│   ├── logger/slog.go
│   ├── errors/errors.go
│   └── validator/validator.go
├── migrations/                # (Phase 02)
├── web/                       # (Phase 03)
├── deploy/
│   ├── Dockerfile.server
│   ├── Dockerfile.worker
│   └── Dockerfile.mcp
├── docker-compose.yml
├── .env.example
├── Makefile
├── go.mod
└── go.sum
```

## Related Code Files

### Create
- `/go.mod`, `/go.sum`
- `/cmd/server/main.go` — minimal "hello Echo"
- `/cmd/worker/main.go` — empty consumer stub
- `/cmd/mcp/main.go` — empty MCP stub
- `/internal/config/config.go` — `Config` struct (DB, AMQP, HTTP, AI keys)
- `/internal/config/load.go` — Viper loader, env override
- `/pkg/logger/slog.go` — JSON slog factory
- `/pkg/errors/errors.go` — domain error types
- `/deploy/Dockerfile.server` — multi-stage build
- `/deploy/Dockerfile.worker`, `/deploy/Dockerfile.mcp`
- `/docker-compose.yml`
- `/.env.example`
- `/Makefile`
- `/.dockerignore`, `/.gitignore`

### Modify
- None (fresh repo)

## Implementation Steps

1. **Init Go module**
   ```
   go mod init github.com/vng/review-code-agent
   go get github.com/labstack/echo/v4 github.com/labstack/echo/v4/middleware
   go get github.com/spf13/viper
   go get github.com/rabbitmq/amqp091-go
   go get github.com/lib/pq
   go get github.com/modelcontextprotocol/go-sdk
   go get github.com/go-ozzo/ozzo-validation/v4
   go get golang.org/x/time/rate
   ```

2. **Write `internal/config/config.go`** — struct only:
   ```go
   type Config struct {
       HTTP struct { Addr string; ReadTimeout, WriteTimeout time.Duration }
       DB   struct { DSN string; MaxOpenConns int }
       AMQP struct { URL string; Prefetch int }
       AI   struct { AnthropicKey, OpenAIKey string; DefaultModel string }
       Auth struct { JWTSecret string; APIKeyPepper string }
       Slack struct { BotToken string }
       CodeGraph struct { URL, Token string }
   }
   ```

3. **Write `cmd/server/main.go`** — 40-line Echo hello:
   ```go
   func main() {
       cfg := config.Load()
       e := echo.New()
       e.Use(middleware.Recover(), middleware.RequestID())
       e.GET("/healthz", func(c echo.Context) error { return c.JSON(200, map[string]string{"status":"ok"}) })
       go func() { e.Start(cfg.HTTP.Addr) }()
       quit := make(chan os.Signal, 1)
       signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
       <-quit
       ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
       defer cancel()
       e.Shutdown(ctx)
   }
   ```

4. **Write Dockerfiles** (multi-stage):
   ```dockerfile
   FROM golang:1.23-alpine AS build
   WORKDIR /src
   COPY go.mod go.sum ./
   RUN go mod download
   COPY . .
   RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app ./cmd/server

   FROM gcr.io/distroless/static
   COPY --from=build /app /app
   USER nonroot:nonroot
   ENTRYPOINT ["/app"]
   ```

5. **Write `docker-compose.yml`**:
   ```yaml
   services:
     postgres:
       image: pgvector/pgvector:pg16
       environment: { POSTGRES_USER: cra, POSTGRES_PASSWORD: cra, POSTGRES_DB: cra }
       healthcheck: { test: [CMD-SHELL, pg_isready -U cra], interval: 5s }
       volumes: [pgdata:/var/lib/postgresql/data]
       ports: ["5432:5432"]
     rabbitmq:
       image: rabbitmq:3.13-management
       ports: ["5672:5672", "15672:15672"]
       healthcheck: { test: rabbitmq-diagnostics -q ping, interval: 10s }
     server:
       build: { context: ., dockerfile: deploy/Dockerfile.server }
       env_file: .env
       depends_on: { postgres: { condition: service_healthy }, rabbitmq: { condition: service_healthy } }
       ports: ["8080:8080"]
     worker:
       build: { context: ., dockerfile: deploy/Dockerfile.worker }
       env_file: .env
       depends_on: { rabbitmq: { condition: service_healthy } }
     web:
       build: { context: ./web }
       env_file: .env
       ports: ["3000:3000"]
       depends_on: [server]
   volumes: { pgdata: {} }
   ```

6. **Makefile** — targets: `build`, `run`, `test`, `lint`, `up`, `down`, `migrate`, `fmt`.

7. **`.env.example`**:
   ```
   HTTP_ADDR=:8080
   DB_DSN=postgres://cra:cra@postgres:5432/cra?sslmode=disable
   AMQP_URL=amqp://guest:guest@rabbitmq:5672/
   ANTHROPIC_API_KEY=
   JWT_SECRET=change-me-32-bytes
   API_KEY_PEPPER=change-me
   SLACK_BOT_TOKEN=
   CODEGRAPH_URL=http://codegraph:9000
   ```

8. **Smoke test**: `docker compose up`, `curl localhost:8080/healthz` returns `ok`.

## Todo List

- [ ] `go mod init` + deps installed
- [ ] `pkg/logger`, `pkg/errors`, `pkg/validator` stubs compile
- [ ] `internal/config/{config,load}.go` loads from env + file
- [ ] Three cmd entrypoints compile (`go build ./cmd/...`)
- [ ] Dockerfiles build < 50MB images
- [ ] docker-compose up passes healthchecks
- [ ] `curl /healthz` returns 200 from container
- [ ] `.env.example` committed (real `.env` gitignored)

## Success Criteria
- `make up && curl -f localhost:8080/healthz` exits 0
- `go vet ./... && go build ./...` clean
- Image inspection: server image < 50MB uncompressed

## Risk Assessment
- **pgvector image tag drift**: pin `pgvector/pgvector:pg16` exact version.
- **Windows dev**: document WSL2 requirement for Docker Compose, add newline to Makefile.
- **MCP SDK availability**: if `modelcontextprotocol/go-sdk` pulled from module cache fails, fallback to `mark3labs/mcp-go`.

## Security Considerations
- `.env` gitignored; only `.env.example` committed.
- Distroless/nonroot user in final image.
- No hardcoded secrets.

## Next Steps
- **Unblocks**: Phase 04 (needs main.go skeleton), Phase 09 (needs worker skeleton), Phase 08 (needs mcp skeleton).
- **Parallel**: Phase 02 and 03 can run simultaneously.
