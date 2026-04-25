---
title: "Code Review Agent SaaS"
description: "Internal SaaS for automated code review via webhook, REST API, and MCP"
status: in-progress
priority: P1
effort: 120h
completed: 6h (Phase 01)
branch: main
tags: [backend, frontend, api, auth, feature, infra]
created: 2026-04-18
updated: 2026-04-25
---

# Code Review Agent SaaS — Master Plan

Internal SaaS for automated PR code review. Ingests via GitHub/GitLab webhooks, REST API, and MCP streaming. Produces structured findings (inline comments + summary), learns from human feedback via pgvector RAG, and surfaces metrics via Next.js admin dashboard.

## Tech Stack

| Layer | Choice |
|-------|--------|
| Backend | Go 1.23 + Echo v4 (clean architecture) |
| Frontend | Next.js 15 (App Router) admin dashboard |
| Database | PostgreSQL 16 + pgvector 0.7 |
| Queue | RabbitMQ 3.13 (amqp091-go) |
| Auth | Username/password (bcrypt) + API keys (SHA256 hashed) |
| Notifications | Slack (incoming webhook + Web API) |
| AI | Claude API (Opus/Sonnet/Haiku) via configurable provider interface |
| Deploy | Docker + Docker Compose |

## Research Context

- Backend research: `research/researcher-01-backend-report.md`
- AI/feedback research: `research/researcher-02-ai-feedback-report.md`

## Dependency Matrix (Parallelization)

```
Phase 00 (Scaffold Group Meta)  [skipped — described inline in each phase]

Group A  [parallel, no cross-deps]
├─ Phase 01: Project scaffold + Docker Compose
├─ Phase 02: Database schema + migrations (pgvector)
└─ Phase 03: Next.js admin scaffold

Group B  [needs A — parallel within]
├─ Phase 04: Go backend core (Echo, auth, guardrails) — needs 01, 02
├─ Phase 05: Review engine (diff/context/AI) — needs 01, 02
└─ Phase 06: Dashboard features (team/model/metrics) — needs 03, 02

Group C  [needs B — parallel within]
├─ Phase 07: Webhook handler (GitHub/GitLab) — needs 04, 05
├─ Phase 08: REST API + MCP streaming — needs 04, 05
└─ Phase 09: RabbitMQ job queue — needs 04, 05

Group D  [needs C — parallel within]
├─ Phase 10: Feedback loop (UI + pgvector RAG) — needs 05, 06, 09
├─ Phase 11: Slack notifications — needs 07, 09
└─ Phase 12: AI usage tracking + pricing dashboard — needs 05, 06

Group E  [final]
├─ Phase 13: Integration tests + full-stack Compose — needs all
└─ Phase 14: Security hardening + performance — needs all
```

## Phase Index

| # | File | Status | Effort |
|---|------|--------|--------|
| 01 | [phase-01-scaffold.md](phase-01-scaffold.md) | done | 6h |
| 02 | [phase-02-database.md](phase-02-database.md) | pending | 8h |
| 03 | [phase-03-dashboard-scaffold.md](phase-03-dashboard-scaffold.md) | pending | 6h |
| 04 | [phase-04-backend-core.md](phase-04-backend-core.md) | pending | 10h |
| 05 | [phase-05-review-engine.md](phase-05-review-engine.md) | pending | 14h |
| 06 | [phase-06-dashboard-features.md](phase-06-dashboard-features.md) | pending | 10h |
| 07 | [phase-07-webhook.md](phase-07-webhook.md) | pending | 8h |
| 08 | [phase-08-api-mcp.md](phase-08-api-mcp.md) | pending | 10h |
| 09 | [phase-09-queue.md](phase-09-queue.md) | pending | 8h |
| 10 | [phase-10-feedback-loop.md](phase-10-feedback-loop.md) | pending | 10h |
| 11 | [phase-11-slack.md](phase-11-slack.md) | pending | 6h |
| 12 | [phase-12-usage-tracking.md](phase-12-usage-tracking.md) | pending | 8h |
| 13 | [phase-13-integration-tests.md](phase-13-integration-tests.md) | pending | 8h |
| 14 | [phase-14-security-perf.md](phase-14-security-perf.md) | pending | 8h |

Total: 120h

## File Ownership Map (No Overlap)

| Phase | Owned Paths |
|-------|-------------|
| 01 | `/go.mod`, `/Makefile`, `/docker-compose.yml`, `/Dockerfile.*`, `/.env.example`, `/cmd/server/main.go`, `/cmd/worker/main.go`, `/cmd/mcp/main.go`, `/internal/config/*` |
| 02 | `/migrations/*.sql`, `/scripts/migrate.sh`, `/internal/repository/postgres/schema.go` |
| 03 | `/web/**` (package.json, next.config.ts, app/layout, app/page, app/login, components/ui/*) |
| 04 | `/internal/domain/*`, `/internal/delivery/http/middleware/*`, `/internal/delivery/http/routes.go`, `/internal/infrastructure/auth/*`, `/internal/guardrail/*` |
| 05 | `/internal/usecase/review/*`, `/internal/infrastructure/ai/*`, `/internal/infrastructure/codegraph/*`, `/internal/infrastructure/git/*` |
| 06 | `/web/app/teams/**`, `/web/app/models/**`, `/web/app/reviews/**`, `/web/app/metrics/**`, `/web/lib/api/*` |
| 07 | `/internal/delivery/http/webhook/*`, `/internal/usecase/webhook/*` |
| 08 | `/internal/delivery/http/api/*`, `/internal/delivery/mcp/*`, `/internal/delivery/http/stream/*` |
| 09 | `/internal/infrastructure/queue/*`, `/internal/usecase/worker/*` |
| 10 | `/internal/infrastructure/embedding/*`, `/internal/usecase/feedback/*`, `/internal/repository/postgres/feedback.go`, `/web/app/feedback/**` |
| 11 | `/internal/infrastructure/slack/*`, `/internal/usecase/notify/*` |
| 12 | `/internal/usecase/usage/*`, `/internal/repository/postgres/usage.go`, `/web/app/usage/**` |
| 13 | `/test/integration/*`, `/docker-compose.test.yml` |
| 14 | `/internal/security/*`, `/deploy/*`, hardening patches (documented as targeted edits) |

## Guiding Principles

- **YAGNI**: no premature multi-region, no multi-tenant sharding, no GraphQL. Single Postgres, single RabbitMQ.
- **KISS**: clean architecture, interface-driven. One binary per concern (server/worker/mcp).
- **DRY**: shared `pkg/` for logger, errors, validator; shared Next.js `components/ui`.
- **<200 lines/file**: split handlers by domain; split repos by entity.

## Unresolved Questions

1. GitNexus Code Graph API contract — need to confirm endpoint spec with the team owning that module before Phase 05.

## Validation Log

### Session 1 — 2026-04-18
**Trigger:** Post-plan validation interview
**Questions asked:** 6

#### Questions & Answers

1. **[Architecture]** Phase 08: MCP server cần transport nào?
   - Options: SSE over HTTP | stdio | Cả hai
   - **Answer:** SSE over HTTP
   - **Rationale:** Internal SaaS — SSE is firewall-friendly and works with web clients. stdio removed from Phase 08 scope.

2. **[Architecture]** Phase 10: Embedding model nào cho pgvector RAG?
   - Options: OpenAI text-embedding-3-small | Claude (voyage-code-3) | Self-hosted nomic-embed-text
   - **Answer:** OpenAI text-embedding-3-small (1536d)
   - **Rationale:** Best cost/quality for v1; consistent with research recommendation.

3. **[Architecture]** Khi diff quá lớn (score > 2000), review engine xử lý thế nào?
   - Options: Reject + error | Chunk & merge | Review top N files only
   - **Answer:** Review top N files only
   - **Rationale:** Heuristic: rank files by change size, pick top N (config default 10). Surfaces most impactful changes without blocking large PRs.

4. **[Architecture]** Phase 14: Rate limiter backend?
   - Options: In-memory | Redis từ đầu
   - **Answer:** Redis từ đầu
   - **Rationale:** Add Redis container to Phase 01 Docker Compose. Implement redis rate limiter in Phase 04. Removes Phase 14 migration complexity.

5. **[Scope]** V1 support bao nhiêu AI provider?
   - Options: Claude only | Claude + OpenAI | Claude + OpenAI + Gemini
   - **Answer:** Claude + OpenAI + Gemini (full multi-provider from v1)
   - **Rationale:** Phase 05 implements all 3 provider impls. Interface already abstracted — minimal extra effort.

6. **[Architecture]** Code Graph service expose API dạng gì?
   - Options: REST HTTP/JSON | gRPC | Chưa biết
   - **Answer:** GitNexus (user's internal module)
   - **Custom input:** "mình định dùng gitnexus á bro"
   - **Rationale:** Code Graph client in Phase 05 wraps GitNexus API. Need to confirm GitNexus endpoint spec before Phase 05 starts.

#### Confirmed Decisions
- MCP transport: SSE over HTTP (not stdio)
- Embedding: OpenAI text-embedding-3-small
- Large diff: top-N files heuristic (default N=10, configurable)
- Rate limiter: Redis from day 1 (Phase 01 + 04)
- AI providers: Claude + OpenAI + Gemini all in v1
- Code Graph: GitNexus (REST assumed; confirm spec)

#### Action Items
- [ ] Phase 01: Add Redis container to docker-compose.yml
- [ ] Phase 04: Implement Redis rate limiter (not in-memory)
- [ ] Phase 05: Remove TooLarge rejection; add top-N file selector + implement OpenAI + Gemini providers + GitNexus client
- [ ] Phase 08: Remove stdio transport; SSE only for MCP server
- [ ] Phase 10: Confirmed OpenAI text-embedding-3-small — no change needed
- [ ] Phase 14: Remove Redis migration task (already in Phase 01/04)

#### Impact on Phases
- Phase 01: Add Redis service to Docker Compose
- Phase 04: Redis rate limiter implementation
- Phase 05: top-N selector + 3 AI providers + GitNexus client
- Phase 08: SSE-only MCP transport
- Phase 14: Remove rate-limiter upgrade item; add prompt-injection hardening instead
