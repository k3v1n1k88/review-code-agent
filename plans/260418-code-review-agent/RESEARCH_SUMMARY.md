# Research Summary — Code Review Agent Backend

**Researcher:** AI Research Agent  
**Date:** 2026-04-18  
**Status:** ✅ Complete  
**Report:** `research/researcher-01-backend-report.md` (912 lines)

---

## Executive Summary

Comprehensive technical research for building a **Go-based code review agent backend** with MCP integration, webhook handling, and async processing. Report covers 10 core topics with code examples, trade-off analysis, and production-ready patterns.

---

## Top Findings

### 1. Framework: Echo > Gin for Code Review Agent
- **Why:** Best-in-class middleware ecosystem (logging, auth, rate limiting, recovery)
- **Trade-off:** Gin has larger community; Echo has better REST API design
- **Decision:** Prioritize middleware richness for guardrails & safety features
- **Database Impact:** Real-world performance differences negligible (I/O-bound)

### 2. Architecture: Clean Architecture + Repository Pattern (Mandatory)
- **Structure:** cmd/ → delivery/ → usecase/ → repository/ → domain/ → infrastructure/
- **Benefit:** Swappable implementations (Postgres/memory/mocks); testable business logic
- **Pattern:** Interfaces in domain/repository; implementations in repository/ subdirs
- **Go Idiom:** Private `internal/` package prevents external imports; clear boundaries

### 3. MCP Server: Official Go SDK (modelcontextprotocol/go-sdk)
- **Maintained by:** MCP org + Google (production-grade)
- **Setup:** 3 steps — NewServer() → AddTool() → Run(transport)
- **Transports:** StdioTransport (default), CommandTransport
- **SSE/WebSocket:** Custom via jsonrpc package (if needed)
- **Integration:** Separate process recommended (separation of concerns)

### 4. Message Queue: RabbitMQ + amqp091-go (Official)
- **Producer:** PublishWithContext + Confirm mode for reliability
- **Consumer:** Qos(prefetch), Consume() loop, manual Ack/Nack
- **Error Handling:** No auto-reconnect (design application-level reconnection)
- **Reliability:** Dead letter queues, persistent delivery mode, publisher confirms
- **Key Pattern:** Queue/exchange declaration idempotent; must rerun post-reconnect

### 5. Webhook Handling
**GitHub:**
- Header: `X-Hub-Signature-256`
- Algorithm: HMAC-SHA256
- ⚠️ Use timing-safe comparison (hmac.Equal), not ==

**GitLab:**
- Header: `X-Gitlab-Token`
- Algorithm: Plain string comparison (less secure)
- Mitigation: Rely on IP allowlist + string comparison

**Workflow:** Verify → Parse → Extract diff via API → Queue async job → Return 200 OK

### 6. Rate Limiting: golang.org/x/time/rate (Token Bucket)
- **Algorithm:** Token bucket (burst capacity + refill rate)
- **Goroutine-safe:** Yes, built-in synchronization
- **Global:** NewLimiter(100, 50) = 100 req/sec, burst 50
- **Per-user:** Sync.Map + lazy init per user ID
- **Best for:** REST APIs, bursty traffic handling

### 7. Input Validation: ozzo-validation/v4
- **Strength:** No struct tags; readable, composable rules
- **Example:** Field(ptr, Required, Length(1, 100), In(...))
- **Errors:** JSON-marshable with meaningful messages
- **Integration:** Validate() method on request structs

### 8. Secrets Detection: Defense-in-Depth
- **CI/CD:** Gitleaks (fast regex, v8.28.0+ composite rules)
- **Verification:** TruffleHog (800+ patterns, credential verification)
- **In-Service:** Custom guardrails (flag suspicious patterns, don't block)
- **Pattern:** Gitleaks pre-commit + TruffleHog in CI + service-level filtering

### 9. Middleware in Echo
- **Logging:** echo-contrib/echologus + log/slog (structured)
- **Context:** Request ID, timeout, cancellation via context.WithTimeout
- **Auth:** Middleware checks JWT/API key before handler
- **Recovery:** Built-in middleware.Recover()
- **Chaining:** Multiple middlewares via e.Use(middleware1, middleware2, ...)

### 10. Recommended Stack (Production-Ready)
| Layer | Technology | Why |
|-------|-----------|-----|
| Web | Echo | Middleware ecosystem, idiomatic Go |
| Architecture | Clean + Repo Pattern | Testability, maintainability |
| MCP | Official Go SDK | Maintained, production-grade |
| Queue | RabbitMQ + amqp091-go | Durable, DLQ, official client |
| Rate Limit | golang.org/x/time/rate | Token bucket, stdlib |
| Validation | ozzo-validation/v4 | Readable, composable |
| Logging | log/slog | Structured, type-safe (Go 1.21+) |
| Testing | In-memory repos | No external dependencies |
| Secrets | Gitleaks + TruffleHog + custom | Defense in depth |

---

## Critical Implementation Patterns (With Code)

### Repository Interface (Testable)
```go
// internal/domain/repository.go
type ReviewRepository interface {
    SaveReview(ctx context.Context, review *Review) error
    GetReview(ctx context.Context, id string) (*Review, error)
}

// internal/repository/postgres/review.go (production)
type PostgresReviewRepository struct { db *sql.DB }

// internal/repository/memory/review.go (tests)
type MemoryReviewRepository struct { reviews map[string]*Review }
```

### MCP Tool Registration
```go
server := mcp.NewServer("code-review-agent")
server.AddTool("review_code", "Review code", func(ctx context.Context, req ReviewCodeInput) (*ReviewCodeOutput, error) {
    return usecase.ReviewCode(ctx, req)
})
server.Run(context.Background(), mcp.StdioTransport())
```

### RabbitMQ Producer (Reliable)
```go
err := ch.PublishWithContext(ctx, "", "code-review-jobs", false, false,
    amqp.Publishing{
        ContentType:  "application/json",
        DeliveryMode: amqp.Persistent,
        Body:         body,
    },
)
```

### GitHub Webhook Signature Verification (Timing-Safe)
```go
h := hmac.New(sha256.New, []byte(secret))
h.Write(payload)
expected := "sha256=" + hex.EncodeToString(h.Sum(nil))
if !hmac.Equal([]byte(expected), []byte(signature)) {
    return echo.NewHTTPError(http.StatusUnauthorized)
}
```

### Rate Limiting Middleware
```go
limiter := rate.NewLimiter(100, 50) // 100 req/sec, burst 50
func middleware(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c echo.Context) error {
        if !limiter.Allow() {
            return echo.NewHTTPError(http.StatusTooManyRequests)
        }
        return next(c)
    }
}
```

### Input Validation
```go
func (r CodeReviewRequest) Validate() error {
    return validation.ValidateStruct(&r,
        validation.Field(&r.Code, validation.Required, validation.Length(1, 100000)),
        validation.Field(&r.Language, validation.Required, validation.In("go", "python", "typescript")),
    )
}
```

---

## Unresolved Questions for Planning

1. **MCP Streaming:** Real-time result streaming vs. batch return?
2. **RabbitMQ Durability:** Do review jobs survive restarts?
3. **GitHub API Strategy:** Batch calls or separate queues per file?
4. **Distributed Rate Limiting:** In-memory vs. Redis for multi-instance?
5. **Defense Layers:** Use IP allowlisting + signature verification?

---

## Deliverables

✅ **Comprehensive Report:** 912 lines, 10 sections, code examples, trade-offs  
✅ **Recommended Stack:** Vetted for production use  
✅ **Code Patterns:** Copy-paste ready for immediate implementation  
✅ **Sources:** All findings linked to authoritative sources (2026 current)  
✅ **Decision Matrix:** Framework & architecture decisions documented

---

## Next Phase: Implementation Planning

This research provides the foundation for:
1. **Phase 1:** Project scaffolding (go.mod, directory structure)
2. **Phase 2:** Database schema & repository layer
3. **Phase 3:** Echo endpoints & webhook handlers
4. **Phase 4:** MCP server integration
5. **Phase 5:** RabbitMQ workers & async jobs
6. **Phase 6:** Tests & CI/CD setup

---

**Report Location:** `/d/vanntl/review-code-agent/plans/260418-code-review-agent/research/researcher-01-backend-report.md`
