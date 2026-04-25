# Code Standards

## Go Conventions

### Naming Conventions

| Element | Convention | Example |
|---------|-----------|---------|
| Package | snake_case (import path) | `github.com/vng/review-code-agent/internal/domain` |
| Function | CamelCase (exported), camelCase (private) | `GetReview()`, `newValidator()` |
| Variable | camelCase | `reviewID`, `maxRetries` |
| Constants | UPPER_SNAKE_CASE | `MAX_REVIEW_SIZE`, `DEFAULT_TIMEOUT` |
| Interfaces | end with `er` | `Reader`, `Writer`, `Validator` |
| File | snake_case | `config.go`, `validator.go` |

### Directory Structure

```
cmd/{app}/
├── main.go              # Entry point only
internal/
├── config/              # Configuration loading & parsing
├── domain/              # Business entities & interfaces
├── usecase/             # Application business rules
├── repository/          # Data persistence abstractions
├── delivery/            # HTTP handlers, request/response
└── infrastructure/      # External service clients
pkg/
├── logger/              # Logging utilities
├── errors/              # Custom error types
└── validator/           # Input validation
```

### Package Organization

- **One responsibility per package**: No grab-bag utilities
- **Domain-driven**: Organize by business concept, not by layer
- **Clear dependencies**: Avoid circular imports, `internal/` packages are private
- **Public interfaces**: Keep exported symbols minimal, hide implementation details

### Function Signatures

```go
// ✓ Good: Clear input/output, error handling
func (r *ReviewRepository) GetByID(ctx context.Context, id string) (*Review, error)

// ✗ Bad: No context, multiple returns without naming
func (r *ReviewRepository) Get(id string) (*Review, string, error)

// ✓ Good: Dependency injection via constructor
func NewReviewService(repo ReviewRepository, logger Logger) *ReviewService
```

### Error Handling

```go
// ✓ Good: Wrap errors with context
if err != nil {
    return fmt.Errorf("failed to fetch review: %w", err)
}

// ✓ Good: Check error type for decisions
if errors.Is(err, ErrNotFound) {
    // handle not found
}

// ✗ Bad: Silent failures, error erasure
_ = someFunction() // Never ignore errors without doc comment
```

### Comments & Documentation

```go
// ✓ Good: Exported function documented
// GetReview fetches a review by ID from the database.
// Returns ErrNotFound if the review doesn't exist.
func (r *ReviewRepository) GetReview(ctx context.Context, id string) (*Review, error) {}

// ✓ Good: Complex logic documented
// Each review is processed in a separate goroutine to avoid blocking.
// Context cancellation will terminate pending jobs.

// ✗ Bad: Redundant comments
i = i + 1 // increment i
```

### Concurrency

- Use `context.Context` for cancellation & deadlines in all I/O operations
- Prefer channels over shared memory
- Use `sync.WaitGroup` for coordinating goroutines
- Keep goroutine spawning centralized in `usecase/` or `infrastructure/`

### Testing

```go
// ✓ Good: Table-driven tests with subtests
func TestValidateReview(t *testing.T) {
    tests := []struct {
        name    string
        review  *Review
        wantErr bool
    }{
        {
            name:    "valid review",
            review:  &Review{ID: "123", Code: "..."},
            wantErr: false,
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test logic
        })
    }
}

// ✓ Good: Test helpers for setup
func setupTestDB(t *testing.T) *sql.DB {
    // Create test database
}
```

### Dependencies

Approved libraries:

| Purpose | Package | Version |
|---------|---------|---------|
| Validation | `go-ozzo/ozzo-validation` | v4.3.0 |
| HTTP Framework | `labstack/echo` | v4.15.1 |
| Message Queue | `rabbitmq/amqp091-go` | v1.11.0 |
| Config Management | `spf13/viper` | v1.21.0 |

Guidelines:
- Minimal external dependencies
- Only add dependencies after approval via CLAUDE.md review
- Use Go stdlib first (database/sql, net/http, log/slog)

### Logging

```go
// ✓ Good: Structured logging via pkg/logger
logger.Info("review processed",
    slog.String("reviewID", review.ID),
    slog.Int("lines", review.LineCount),
)

// ✓ Good: Include context in errors
logger.Error("database query failed", slog.String("error", err.Error()))

// ✗ Bad: Unstructured strings
log.Println("something happened")
```

## Build & Quality

### Makefile Targets

```bash
make build      # Compile all binaries (server, worker, mcp)
make test       # Run all tests
make lint       # Run golangci-lint
make vet        # Run go vet
make fmt        # Format code with gofmt
make up         # Start docker-compose services
make down       # Stop docker-compose services
```

### Pre-Commit Checklist

- [ ] `make fmt` passes (no unformatted code)
- [ ] `make vet` passes (no suspicious constructs)
- [ ] `make lint` passes (linter passes with golangci-lint)
- [ ] `make test` passes (all tests pass)
- [ ] No hardcoded secrets (check .env usage)
- [ ] Error handling implemented (no silent failures)

### Linting Rules

- golangci-lint configuration in `.golangci.yml`
- Run before every commit: `make lint`
- Fix issues locally, don't commit lint violations

## Configuration

### Environment Variables

All configuration via `.env` (dev) or secrets manager (production).

Required variables (see `.env.example`):
- `HTTP_ADDR` — HTTP server bind address
- `DB_DSN` — PostgreSQL connection string
- `AMQP_URL` — RabbitMQ URL
- `REDIS_ADDR` — Redis connection
- `ANTHROPIC_API_KEY` — Claude API key
- `JWT_SECRET` — JWT signing secret (min 32 bytes)

### Configuration Loading

```go
// ✓ Good: Loaded via internal/config/load.go
cfg, err := config.Load()
if err != nil {
    log.Fatalf("failed to load config: %v", err)
}
```

## File Size Limits

- **Go source files:** Keep under 400 lines (refactor large files)
- **Markdown docs:** Under 800 lines per file (split into index + subtopics)
- **Reasons:** Reduced cognitive load, easier testing & review

## Deployment Standards

### Docker

- **Base image:** distroless (Alpine for dev)
- **Build stage:** Multi-stage, compile in builder image
- **Runtime:** Single binary, no shell
- **Healthchecks:** Define for all services in docker-compose.yml

### Database

- **Migrations:** Use SQL files in `migrations/` directory
- **Schema:** Run migrations on startup or via CI/CD pipeline
- **Backups:** Configure in production deployment docs (Phase 02+)

## Security

- Never commit `.env` files with real secrets
- Validate all user input via `pkg/validator`
- Use parameterized queries (prepared statements)
- Enforce JWT authentication on protected endpoints (Phase 03+)
- Log security events (auth failures, validation errors)

## Documentation

- Update docs when code changes significantly
- Link code files in architecture docs only after they're implemented
- Keep examples in docs current with actual function signatures
