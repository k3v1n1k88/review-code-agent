# Code Review Agent Backend — Go Research Report

**Date:** 2026-04-18  
**Scope:** Go web frameworks, MCP server implementation, RabbitMQ integration, webhook handling, rate limiting, input validation, secrets detection  
**Status:** Complete

---

## 1. Go Web Framework Selection

### Comparison Matrix

| Aspect | Gin | Echo | Fiber |
|--------|-----|------|-------|
| **GitHub Stars** | 75,000+ | High | Growing |
| **HTTP Backend** | net/http | net/http | fasthttp |
| **Error Handling** | Panic-style | Returns errors | Returns errors |
| **Context Pattern** | Custom | context.Context | context.Context |
| **Middleware Ecosystem** | Extensive | Most comprehensive | Strong |
| **Performance (no DB)** | Very high | Very high | Highest |
| **Real-world Performance** | Database becomes bottleneck |
| **Maturity** | Production-proven | Production-proven | Production-proven |
| **Team Familiarity** | Common | Growing | Common (JS devs) |

### Recommendation: **Echo**

**Rationale:**
- Best middleware ecosystem (built-in HTTP/2, TLS, security middleware)
- Uses standard context.Context (Go idiom)
- Error propagation via return values (idiomatic Go)
- Excellent documentation for REST APIs
- For a code review agent, middleware richness matters more than marginal performance gains
- Database I/O will dominate, rendering framework performance differences negligible

**Trade-off:** Gin has larger community; Echo has better design for REST APIs.

### Key Libraries for Echo

```go
import (
    "github.com/labstack/echo/v4"
    "github.com/labstack/echo/v4/middleware"
)
```

---

## 2. Go Project Structure (Clean Architecture)

### Recommended Layout

```
code-review-backend/
├── cmd/
│   └── server/
│       └── main.go              # Entry point
├── internal/
│   ├── domain/                  # Business logic (interfaces, entities)
│   │   ├── models.go
│   │   ├── errors.go
│   │   └── review.go
│   ├── usecase/                 # Application logic
│   │   ├── code_review_usecase.go
│   │   └── webhook_usecase.go
│   ├── repository/              # Data access layer
│   │   ├── interfaces.go
│   │   ├── postgres/
│   │   └── memory/              # For testing
│   ├── delivery/                # HTTP handlers
│   │   └── http/
│   │       ├── handler.go
│   │       ├── middleware.go
│   │       └── routes.go
│   ├── infrastructure/
│   │   ├── rabbitmq/            # RabbitMQ client
│   │   ├── github/              # GitHub API client
│   │   ├── gitlab/              # GitLab API client
│   │   └── mcp/                 # MCP server setup
│   └── config/
│       └── config.go
├── api/
│   └── openapi.yaml
├── pkg/                         # Shareable utilities (if multiple services)
│   ├── logger/
│   ├── errors/
│   └── validator/
├── scripts/
├── go.mod
├── go.sum
└── Makefile
```

### Key Principles

1. **Dependency Inversion:** High-level modules depend on abstractions (interfaces), not concrete implementations
2. **Repository Pattern:** Data access abstracted behind interfaces; swappable implementations (Postgres, memory, mocks)
3. **Single Responsibility:** Each layer has one reason to change
4. **Testability:** Inject dependencies; mock external services easily

### Example: Repository Interface

```go
// internal/domain/repository.go
type ReviewRepository interface {
    SaveReview(ctx context.Context, review *Review) error
    GetReview(ctx context.Context, id string) (*Review, error)
    ListByPR(ctx context.Context, prID string) ([]*Review, error)
}

// internal/repository/postgres/review.go
type PostgresReviewRepository struct {
    db *sql.DB
}

func (r *PostgresReviewRepository) SaveReview(ctx context.Context, review *Review) error {
    // Postgres-specific logic
}

// internal/repository/memory/review.go (for tests)
type MemoryReviewRepository struct {
    mu      sync.RWMutex
    reviews map[string]*Review
}

func (r *MemoryReviewRepository) SaveReview(ctx context.Context, review *Review) error {
    // In-memory logic
}
```

---

## 3. MCP Server Implementation in Go

### Available Libraries

#### **Official Go SDK** (Recommended)
- **GitHub:** `github.com/modelcontextprotocol/go-sdk`
- **Maintained by:** Model Context Protocol organization + Google
- **Status:** Official, production-ready
- **Transports:** StdioTransport (stdin/stdout), CommandTransport

#### **mcp-go** (Community)
- **GitHub:** `github.com/mark3labs/mcp-go`
- **Published:** April 8, 2026
- **Imports:** 1,880+ packages
- **Status:** High-level, easier API

#### **go-mcp** (Community)
- **Status:** Client + server implementations
- **Transports:** SSE, STDIO

### Recommended Approach: Official Go SDK

**Setup Pattern:**

```go
import "github.com/modelcontextprotocol/go-sdk/mcp"

// Define tool input/output with JSON schema
type ReviewCodeInput struct {
    Code        string `json:"code"`
    Language    string `json:"language"`
    FilePath    string `json:"filePath"`
}

type ReviewCodeOutput struct {
    Issues      []Issue `json:"issues"`
    Summary     string  `json:"summary"`
}

// Create server
server := mcp.NewServer("code-review-agent")

// Register tool
server.AddTool(
    "review_code",
    "Review code for issues, patterns, security, and best practices",
    func(ctx context.Context, req ReviewCodeInput) (*ReviewCodeOutput, error) {
        // Delegate to usecase/service
        return usecase.ReviewCode(ctx, req)
    },
)

// Run over transport
transport := mcp.StdioTransport()
server.Run(context.Background(), transport)
```

### Key Concepts

1. **Tools:** RPCs exposed to MCP client (Claude)
2. **Resources:** Read-only data exposed to MCP client
3. **Transport:** Communication channel (stdio, HTTP, WebSocket)

### SSE/WebSocket Support

- Official SDK supports custom transports via `jsonrpc` package
- For SSE: Implement custom `jsonrpc.Transport`
- For WebSocket: Wrap connection in `jsonrpc.Transport` adapter

### Integration with Echo

```go
// Option 1: MCP server runs as separate process (simplest)
// Option 2: MCP server in goroutine, expose via Echo endpoint
// Option 3: MCP tools exposed via REST + Echo, then wrapped by external MCP client

// Recommended: Option 1 (separation of concerns)
```

---

## 4. RabbitMQ Integration

### Library: amqp091-go

**Status:** Official RabbitMQ client, maintained by RabbitMQ core team  
**Version:** v1.10.0+ (May 8, 2024+)  
**Import:** `github.com/rabbitmq/amqp091-go`

### Producer Pattern

```go
import amqp "github.com/rabbitmq/amqp091-go"

type CodeReviewProducer struct {
    conn *amqp.Connection
    ch   *amqp.Channel
}

func (p *CodeReviewProducer) PublishReviewJob(ctx context.Context, job *ReviewJob) error {
    // Enable publisher confirms for reliability
    if err := p.ch.Confirm(false); err != nil {
        return fmt.Errorf("confirm mode: %w", err)
    }
    
    body, _ := json.Marshal(job)
    
    err := p.ch.PublishWithContext(ctx,
        "",                           // exchange
        "code-review-jobs",           // routing key
        false,                        // mandatory
        false,                        // immediate
        amqp.Publishing{
            ContentType:  "application/json",
            DeliveryMode: amqp.Persistent,
            Body:         body,
        },
    )
    
    return err
}
```

### Consumer Pattern

```go
type CodeReviewConsumer struct {
    conn *amqp.Connection
    ch   *amqp.Channel
}

func (c *CodeReviewConsumer) ConsumeReviewJobs(ctx context.Context, prefetch int) error {
    // Set QoS to limit unacknowledged messages
    if err := c.ch.Qos(prefetch, 0, false); err != nil {
        return fmt.Errorf("qos: %w", err)
    }
    
    deliveries, err := c.ch.Consume(
        "code-review-jobs",  // queue
        "",                  // consumer tag
        false,               // autoAck
        false,               // exclusive
        false,               // noLocal
        false,               // noWait
        nil,                 // args
    )
    if err != nil {
        return fmt.Errorf("consume: %w", err)
    }
    
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case delivery := <-deliveries:
            var job ReviewJob
            if err := json.Unmarshal(delivery.Body, &job); err != nil {
                delivery.Nack(false, true) // requeue
                continue
            }
            
            if err := processReview(ctx, job); err != nil {
                // Dead letter queue via x-dead-letter-exchange
                delivery.Nack(false, true) // requeue once, then DLQ
                continue
            }
            
            delivery.Ack(false)
        }
    }
}
```

### Queue Setup (Idempotent)

```go
// Regular queue
q, err := ch.QueueDeclare(
    "code-review-jobs",  // name
    true,                // durable
    false,               // auto-delete
    false,               // exclusive
    false,               // noWait
    nil,                 // args
)

// Dead letter queue
dlq, err := ch.QueueDeclare(
    "code-review-jobs-dlq",
    true, false, false, false,
    nil,
)

// Bind main queue to exchange with DLQ argument
err = ch.QueueBind(
    "code-review-jobs",
    "code-review-jobs",
    "",  // exchange (default)
    false,
    amqp.Table{
        "x-dead-letter-exchange": "",
        "x-dead-letter-routing-key": "code-review-jobs-dlq",
    },
)
```

### Error Handling Strategy

1. **Connection Errors:** Implement exponential backoff reconnection at application level
2. **Message Processing Errors:**
   - Recoverable: `Nack(false, true)` to requeue
   - Persistent: `Nack(false, false)` + manual DLQ handling
3. **Channel Closure:** Monitor `NotifyClose()` channel for graceful degradation

### Important: No Auto-Reconnect

- **Library does not auto-reconnect.** Design application-level reconnection logic.
- Topology declaration (queues, exchanges) must happen after reconnect.
- Wrap in supervisor (e.g., using hashicorp/go-multierror or custom retry loop).

---

## 5. GitHub/GitLab Webhook Handling

### GitHub Webhook Setup

#### Signature Verification (Critical)

**Header:** `X-Hub-Signature-256`  
**Format:** `sha256=<hex_digest>`  
**Algorithm:** HMAC-SHA256 using webhook secret + payload body

```go
import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "io"
)

func VerifyGitHubSignature(secret string, signature string, payload []byte) bool {
    h := hmac.New(sha256.New, []byte(secret))
    h.Write(payload)
    expected := "sha256=" + hex.EncodeToString(h.Sum(nil))
    
    // Use constant-time comparison (timing-safe)
    return hmac.Equal([]byte(expected), []byte(signature))
}

// In Echo handler
func handleGitHubWebhook(c echo.Context) error {
    signature := c.Request().Header.Get("X-Hub-Signature-256")
    
    body, _ := io.ReadAll(c.Request().Body)
    
    if !VerifyGitHubSignature(githubSecret, signature, body) {
        return echo.NewHTTPError(http.StatusUnauthorized, "invalid signature")
    }
    
    var event map[string]interface{}
    json.Unmarshal(body, &event)
    
    action := event["action"].(string)
    if action == "opened" || action == "synchronize" {
        // Queue code review job
    }
    
    return c.NoContent(http.StatusOK)
}
```

#### Payload Parsing for PR Events

```go
type GitHubPREvent struct {
    Action string `json:"action"` // opened, synchronize, closed, etc.
    Number int    `json:"number"` // PR number
    PullRequest struct {
        ID    int    `json:"id"`
        Title string `json:"title"`
        Body  string `json:"body"`
        Head struct {
            SHA  string `json:"sha"`
            Ref  string `json:"ref"`
            Repo struct {
                CloneURL string `json:"clone_url"`
            } `json:"repo"`
        } `json:"head"`
        Base struct {
            SHA string `json:"sha"`
        } `json:"base"`
    } `json:"pull_request"`
    Repository struct {
        FullName string `json:"full_name"`
        URL      string `json:"url"`
    } `json:"repository"`
}
```

### GitLab Webhook Setup

#### Signature Verification

**Header:** `X-Gitlab-Token`  
**Algorithm:** Plain string comparison (NOT HMAC)  
**⚠️ Less secure than GitHub** — use GitLab's IP allowlist for additional protection

```go
func VerifyGitLabSignature(secret string, token string) bool {
    // GitLab uses plain string comparison, not timing-safe
    // This is a known weakness; rely on IP allowlist in production
    return secret == token
}

func handleGitLabWebhook(c echo.Context) error {
    token := c.Request().Header.Get("X-Gitlab-Token")
    eventType := c.Request().Header.Get("X-Gitlab-Event")
    
    if !VerifyGitLabSignature(gitlabSecret, token) {
        return echo.NewHTTPError(http.StatusUnauthorized)
    }
    
    if eventType == "Merge Request Hook" {
        // Process MR event
    }
    
    return c.NoContent(http.StatusOK)
}
```

#### MR Event Payload

```go
type GitLabMREvent struct {
    ObjectKind string `json:"object_kind"` // "merge_request"
    Action     string `json:"action"`      // opened, update, merge, close, etc.
    MergeRequest struct {
        ID     int    `json:"id"`
        Iid    int    `json:"iid"`         // Project-specific MR number
        Title  string `json:"title"`
        Body   string `json:"description"`
        State  string `json:"state"`       // opened, merged, closed
        SourceBranch string `json:"source_branch"`
        TargetBranch string `json:"target_branch"`
        Source struct {
            CloneURL string `json:"clone_url"`
        } `json:"source"`
        Target struct {
            CloneURL string `json:"clone_url"`
        } `json:"target"`
        DiffURL string `json:"web_url"` // Use GitLab API to fetch diff
    } `json:"object_attributes"`
    Project struct {
        ID       int    `json:"id"`
        PathWithNamespace string `json:"path_with_namespace"`
    } `json:"project"`
}
```

### Webhook Handling Workflow

```
1. Receive webhook (GitHub/GitLab)
2. Verify signature (timing-safe)
3. Parse event type (PR opened/synchronized or MR opened/updated)
4. Extract diff/patch via API (webhook payload has limited diff data)
5. Queue async code review job in RabbitMQ
6. Return 200 OK immediately (webhook timeout is ~30s)
7. Process review job asynchronously
```

### Getting Diffs

**GitHub API:**
```
GET /repos/{owner}/{repo}/pulls/{pull_number}/files
```

**GitLab API:**
```
GET /projects/{id}/merge_requests/{mr_iid}/changes
```

---

## 6. Rate Limiting & Guardrails

### Rate Limiting: Token Bucket (Recommended)

**Library:** `golang.org/x/time/rate` (standard library)

**Why Token Bucket:**
- Handles bursty traffic naturally (burst capacity)
- Used by AWS, Stripe, GitHub APIs
- Token refill at fixed rate
- Simple, predictable

```go
import "golang.org/x/time/rate"

type APILimiter struct {
    limiter *rate.Limiter
}

// Allow 100 requests/sec with burst of 50
func NewAPILimiter() *APILimiter {
    return &APILimiter{
        limiter: rate.NewLimiter(100, 50),
    }
}

// Middleware for Echo
func (al *APILimiter) Middleware() echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            if !al.limiter.Allow() {
                return echo.NewHTTPError(
                    http.StatusTooManyRequests,
                    "rate limit exceeded",
                )
            }
            return next(c)
        }
    }
}

// Per-user rate limiting (requires Redis or in-memory map)
type PerUserLimiter struct {
    limiters sync.Map // map[userID]*rate.Limiter
}

func (pul *PerUserLimiter) GetLimiter(userID string) *rate.Limiter {
    val, _ := pul.limiters.LoadOrStore(
        userID,
        rate.NewLimiter(10, 20), // 10 req/sec per user, burst 20
    )
    return val.(*rate.Limiter)
}
```

### Input Validation

**Library:** `github.com/go-ozzo/ozzo-validation/v4`

**Why:**
- No error-prone struct tags
- Readable, composable validation rules
- Good error messages
- JSON marshaling support

```go
import validation "github.com/go-ozzo/ozzo-validation/v4"

type CodeReviewRequest struct {
    Code        string `json:"code"`
    Language    string `json:"language"`
    FilePath    string `json:"file_path"`
}

func (r CodeReviewRequest) Validate() error {
    return validation.ValidateStruct(&r,
        validation.Field(
            &r.Code,
            validation.Required,
            validation.Length(1, 100000),
        ),
        validation.Field(
            &r.Language,
            validation.Required,
            validation.In("go", "python", "typescript", "rust", "java"),
        ),
        validation.Field(
            &r.FilePath,
            validation.Required,
            validation.Length(1, 500),
        ),
    )
}

// In handler
func handleReviewRequest(c echo.Context) error {
    var req CodeReviewRequest
    if err := c.BindJSON(&req); err != nil {
        return err
    }
    
    if err := req.Validate(); err != nil {
        return echo.NewHTTPError(http.StatusBadRequest, err)
    }
    
    // Process request
    return nil
}
```

### Secrets Detection in Code Diffs

#### Approach 1: Gitleaks (Pre-commit, local scanning)
- **Speed:** Fast regex matching
- **Accuracy:** Configurable patterns, composite rules (v8.28.0+)
- **Use Case:** Block commits with exposed secrets

#### Approach 2: TruffleHog (CI/CD verification)
- **Strength:** Credential verification (tests if secret still works)
- **Coverage:** 800+ secret types
- **Use Case:** Verify no active credentials leaked

#### Approach 3: Custom filtering (in-service guardrail)

```go
package secrets

import (
    "regexp"
    "strings"
)

var (
    // Regex patterns for common secret formats
    awsKeyPattern    = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)
    githubTokenPattern = regexp.MustCompile(`ghp_[A-Za-z0-9_]{36}`)
    apiKeyPattern    = regexp.MustCompile(`api[_-]?key[=:]\s*["\']?[A-Za-z0-9_-]{20,}["\']?`)
    dbPasswordPattern = regexp.MustCompile(`password\s*[=:]\s*["\']([^"\']+)["\']`)
)

func ContainsSuspiciousSecrets(code string) (bool, []string) {
    var found []string
    
    patterns := map[string]*regexp.Regexp{
        "aws_key":       awsKeyPattern,
        "github_token":  githubTokenPattern,
        "api_key":       apiKeyPattern,
        "db_password":   dbPasswordPattern,
    }
    
    for name, pattern := range patterns {
        if pattern.MatchString(code) {
            found = append(found, name)
        }
    }
    
    return len(found) > 0, found
}

// In webhook handler (before queuing review job)
if hasSuspicious, secrets := secrets.ContainsSuspiciousSecrets(diffContent) {
    // Option A: Block entirely
    return echo.NewHTTPError(http.StatusBadRequest,
        fmt.Sprintf("suspicious patterns detected: %v", secrets))
    
    // Option B: Flag in review, don't send to Claude
    // flag review as "contains sensitive patterns"
}
```

**Recommendation:** Use Gitleaks in CI/CD + custom guardrail in service (Option B: flag without blocking, let human decide).

---

## 7. Middleware Patterns in Echo

### Request Logging

```go
import "github.com/labstack/echo-contrib/echologus"
import "log/slog"

logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
e.Use(echologus.EchoMiddleware(logger))
```

### Context Management & Cancellation

```go
// Pass context through request lifecycle
func contextMiddleware() echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            ctx := c.Request().Context()
            
            // Add request ID for tracing
            requestID := uuid.NewString()
            ctx = context.WithValue(ctx, "requestID", requestID)
            
            // Set timeout (5 seconds for webhooks)
            ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
            defer cancel()
            
            c.SetRequest(c.Request().WithContext(ctx))
            return next(c)
        }
    }
}

// In handler
func handleReview(c echo.Context) error {
    ctx := c.Request().Context()
    requestID := ctx.Value("requestID").(string)
    
    // Pass context to services
    result, err := svc.ReviewCode(ctx, code)
    return nil
}
```

### Auth Middleware

```go
func authMiddleware(secret string) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            token := c.Request().Header.Get("Authorization")
            
            if token == "" {
                return echo.NewHTTPError(http.StatusUnauthorized, "missing token")
            }
            
            // Verify JWT / API key
            if !verifyToken(token, secret) {
                return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
            }
            
            return next(c)
        }
    }
}

e.Use(authMiddleware(os.Getenv("API_SECRET")))
```

### Error Recovery Middleware

```go
e.Use(middleware.Recover()) // Built-in
```

---

## 8. Summary: Recommended Stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| **Web Framework** | Echo | Best middleware ecosystem, idiomatic Go, excellent for REST APIs |
| **Project Structure** | Clean Architecture + Repository Pattern | Testable, maintainable, separates concerns |
| **MCP Server** | Official `modelcontextprotocol/go-sdk` | Maintained by MCP org + Google, production-ready |
| **Message Queue** | RabbitMQ + amqp091-go | Official client, durable delivery, DLQ support |
| **Rate Limiting** | golang.org/x/time/rate | Token bucket, standard library, production-proven |
| **Input Validation** | ozzo-validation/v4 | Readable, composable, good errors |
| **Webhook Signatures** | crypto/hmac (GitHub), plain string (GitLab) | Native to stdlib, timing-safe comparison required |
| **Secrets Detection** | Gitleaks (CI/CD) + custom guardrails (service) | Best coverage + in-service defense |
| **Logging** | log/slog (stdlib) | Structured logging, type-safe, built-in (1.21+) |
| **Testing Repos** | In-memory implementations | No external dependencies during tests |

---

## 9. Key Implementation Patterns

### Graceful Shutdown

```go
func main() {
    e := echo.New()
    
    // Start server in goroutine
    go func() {
        if err := e.Start(":8080"); err != nil && err != http.ErrServerClosed {
            e.Logger.Fatal(err)
        }
    }()
    
    // Wait for interrupt
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    
    // Graceful shutdown with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    if err := e.Shutdown(ctx); err != nil {
        e.Logger.Fatal(err)
    }
}
```

### Dependency Injection

```go
type Handler struct {
    reviewUsecase ReviewUsecase
    logger        *slog.Logger
}

func NewHandler(usecase ReviewUsecase, logger *slog.Logger) *Handler {
    return &Handler{
        reviewUsecase: usecase,
        logger:        logger,
    }
}

func (h *Handler) RegisterRoutes(e *echo.Echo) {
    e.POST("/webhook/github", h.handleGitHubWebhook)
    e.POST("/webhook/gitlab", h.handleGitLabWebhook)
}
```

### Interface-Based Testing

```go
type MockReviewUsecase struct {
    mock.Mock
}

func (m *MockReviewUsecase) ReviewCode(ctx context.Context, req *CodeReviewRequest) (*CodeReviewResult, error) {
    args := m.Called(ctx, req)
    return args.Get(0).(*CodeReviewResult), args.Error(1)
}

func TestHandleReviewRequest(t *testing.T) {
    mockUsecase := new(MockReviewUsecase)
    mockUsecase.On("ReviewCode", mock.Anything, mock.Anything).
        Return(&CodeReviewResult{}, nil)
    
    handler := NewHandler(mockUsecase, testLogger)
    // Test handler
}
```

---

## 10. References

### Web Frameworks
- [Gin vs Echo vs Fiber - DEV Community](https://dev.to/matthiasbruns/go-web-frameworks-in-production-gin-vs-echo-vs-fiber-performance-comparison-1gkp)
- [2026 Minimalist's Guide - Medium](https://medium.com/@samayun_pathan/choosing-a-go-web-framework-in-2026-a-minimalists-guide-to-gin-fiber-chi-echo-and-beego-c79b31b8474d)
- [Encore Framework Comparison](https://encore.dev/articles/best-go-backend-frameworks)

### Go Architecture
- [Go Project Structure 2026 - Reintech](https://reintech.io/blog/go-project-structure-2026-clean-architecture-best-practices)
- [Clean Architecture in Go - GitHub](https://github.com/bxcodec/go-clean-arch)
- [Go Hexagonal Architecture - DEV](https://dev.to/kittipat1413/structuring-a-go-project-with-clean-architecture-a-practical-example-3b3f)

### MCP Protocol
- [Official Go SDK - GitHub](https://github.com/modelcontextprotocol/go-sdk)
- [mcp-go Library - GitHub](https://github.com/mark3labs/mcp-go)
- [MCP Server in Go - Medium](https://medium.com/@smbaker/i-created-my-first-model-context-protocol-mcp-server-in-go-and-it-was-really-easy-c356558a4768)

### RabbitMQ
- [amqp091-go Documentation - Go Packages](https://pkg.go.dev/github.com/rabbitmq/amqp091-go)
- [GitHub Repository - RabbitMQ](https://github.com/rabbitmq/amqp091-go)
- [Message Queue Consumers - OneUptime](https://oneuptime.com/blog/post/2026-02-01-go-message-queue-consumers/view)

### Webhooks
- [GitHub Webhook Signature Validation](https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries)
- [GitHub Webhooks Complete Guide - MagicBell](https://www.magicbell.com/blog/github-webhooks-guide)
- [Webhook Authentication Learnings - Release](https://release.com/blog/webhook-authentication-learnings)

### Rate Limiting
- [Token Bucket in Go - OneUptime](https://oneuptime.com/blog/post/2026-01-25-token-bucket-rate-limiting-go/view)
- [golang.org/x/time/rate - Go Packages](https://pkg.go.dev/golang.org/x/time/rate)
- [Rate Limiting Deep Dive - DEV Community](https://dev.to/dylan_dumont_266378d98367/rate-limiting-deep-dive-token-bucket-vs-leaky-bucket-vs-sliding-window-47b7)

### Validation & Secrets
- [ozzo-validation Documentation](https://go-ozzo.github.io/ozzo-validation/)
- [Gitleaks vs TruffleHog - AppSecSanta](https://appsecsanta.com/sast-tools/gitleaks-vs-trufflehog)
- [Secret Scanning Tools 2026 - AppSecSanta](https://appsecsanta.com/sast-tools/secret-scanning-tools)

### Context & Middleware
- [Go Context Package Best Practices - Dasroot](https://dasroot.net/posts/2026/03/mastering-go-context-request-scoping-cancellation/)
- [Context Cancellation - OneUptime](https://oneuptime.com/blog/post/2026-01-23-go-context-cancellation/view)
- [Go Error Handling Guide - Middleware.io](https://middleware.io/blog/go-error-handling/)

---

## Unresolved Questions

1. **MCP Streaming:** Will the code review agent stream results back via MCP (long-running reviews) or return immediately? Affects transport choice.
2. **RabbitMQ Persistence:** Should review jobs survive RabbitMQ restarts? (Requires durable queues + persistent delivery mode)
3. **GitHub/GitLab API Rate Limits:** Should the service batch API calls or queue separately? Affects webhook queuing strategy.
4. **Distributed Rate Limiting:** Is in-memory token bucket sufficient, or do we need Redis-backed rate limiting for multi-instance deployments?
5. **Webhook IP Allowlisting:** Will we use platform IP allowlists (GitHub/GitLab) in addition to signature verification for defense in depth?
