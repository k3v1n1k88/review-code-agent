# Code Review Agent Backend — Research & Planning Index

**Created:** 2026-04-18  
**Status:** Research Complete ✅  

---

## 📋 Documents

### [RESEARCH_SUMMARY.md](RESEARCH_SUMMARY.md)
**Quick Reference** — 1-page executive summary  
- Top 10 findings
- Recommended stack
- Code patterns
- Unresolved questions

### [research/researcher-01-backend-report.md](research/researcher-01-backend-report.md)
**Full Technical Report** — 912 lines, comprehensive  
1. Go Web Framework Selection (Gin vs Echo vs Fiber)
2. Project Structure (Clean Architecture + Repository Pattern)
3. MCP Server Implementation (Official Go SDK)
4. RabbitMQ Integration (amqp091-go producer/consumer)
5. GitHub/GitLab Webhook Handling (signature verification, diff parsing)
6. Rate Limiting (token bucket)
7. Input Validation (ozzo-validation)
8. Secrets Detection (Gitleaks + TruffleHog + custom)
9. Middleware Patterns (logging, auth, context, recovery)
10. Recommended Stack (summary table)
11. Key Implementation Patterns (graceful shutdown, DI, testing)
12. References (all sources with markdown hyperlinks)

### [README.md](README.md)
**Project Overview** — planning structure and next phases

---

## 🎯 Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Web Framework | **Echo** | Best middleware ecosystem, idiomatic Go |
| Architecture | **Clean + Repository Pattern** | Testable, maintainable, swappable |
| MCP | **Official Go SDK** | Production-grade, maintained by MCP org |
| Queue | **RabbitMQ + amqp091-go** | Durable, DLQ, official client |
| Rate Limiting | **golang.org/x/time/rate** | Token bucket, stdlib |
| Validation | **ozzo-validation/v4** | Readable, composable |
| Logging | **log/slog** | Structured, type-safe |
| Testing | **In-memory repos** | No external deps |
| Secrets | **Gitleaks + TruffleHog + custom** | Defense in depth |

---

## 🔗 Quick Links

- **Official Go SDK:** https://github.com/modelcontextprotocol/go-sdk
- **Echo Framework:** https://echo.labstack.com
- **RabbitMQ Client:** https://github.com/rabbitmq/amqp091-go
- **Rate Limiter:** https://pkg.go.dev/golang.org/x/time/rate
- **Validation:** https://pkg.go.dev/github.com/go-ozzo/ozzo-validation/v4
- **GitHub Webhooks:** https://docs.github.com/en/webhooks

---

## 📝 Implementation Roadmap

1. **Phase 1:** Project scaffolding (go.mod, directory structure)
2. **Phase 2:** Database schema & repository implementations
3. **Phase 3:** Echo endpoints & webhook handlers
4. **Phase 4:** MCP server integration
5. **Phase 5:** RabbitMQ workers & async processing
6. **Phase 6:** Tests & documentation

---

**Report Status:** Research Complete ✅  
**Next Phase:** Implementation Planning (Awaiting approval)
