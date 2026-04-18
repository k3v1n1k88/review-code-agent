# Phase 06 — Dashboard Features (Teams, Models, Reviews, Metrics)

## Context Links
- Parent: [plan.md](plan.md)
- Research: `researcher-02-ai-feedback-report.md` §5.4 (dashboard queries)
- Depends on: Phase 02 (schema), Phase 03 (scaffold). Backend CRUD endpoints from Phase 04+05+12 are needed at RUN time; this phase develops against typed API stubs and enables mock-driven UI first.

## Overview
- **Priority**: P1
- **Status**: pending
- **Effort**: 10h
- **Group**: B (parallel with 04, 05)
- **Description**: Fill the Phase-03 stubs. Teams CRUD, users+API keys per team, model config editor, reviews list+detail, metrics overview. Consumes backend via `lib/api`. This phase ALSO adds the corresponding backend handlers (`/api/v1/teams`, `/api/v1/models`, `/api/v1/reviews`) — keeping UI + API parallel avoids frontend-backend round trips later.

## Key Insights
- Keep each page < 150 lines by splitting components (list, form, detail).
- React Query for server state; avoid Redux.
- Reviews list uses cursor pagination; metrics uses daily aggregates (materialized view planned Phase 12).
- Model config form: dropdowns for provider + model_name (enum), numeric for max_tokens/temperature.

## Requirements

### Functional (Frontend)
- `/teams`: list teams + create form (admin-only). Each row links to detail.
- `/teams/[id]`: members, API keys (generate/revoke with copy-once modal).
- `/models`: per-team list, form to create/edit, toggle default.
- `/reviews`: paginated, filter by team/status, links to detail.
- `/reviews/[id]`: summary, issues grouped by severity, diff link, feedback widget (placeholder → Phase 10).
- `/metrics`: cards (total reviews, avg latency, failure rate), 30-day line chart.

### Functional (Backend)
- `GET/POST /api/v1/teams`, `GET/PATCH/DELETE /api/v1/teams/:id`
- `GET/POST /api/v1/teams/:id/api-keys`, `DELETE .../api-keys/:kid` (revoke)
- `GET/POST /api/v1/models`, `PATCH/DELETE /api/v1/models/:id`
- `GET /api/v1/reviews?team_id=&cursor=&status=`
- `GET /api/v1/reviews/:id` (includes issues)
- `GET /api/v1/metrics/overview?team_id=&from=&to=`

### Non-functional
- Reviews list renders 50 items < 500ms.
- Admin-only gates enforced server-side.
- Charts lazy-loaded (recharts dynamic import).

## Architecture

### Backend additions
```
internal/
├── usecase/
│   ├── team/{list,create,update,delete,member}.go
│   ├── apikey/{issue,revoke,list}.go
│   ├── modelcfg/{list,upsert,delete}.go
│   ├── reviews/{list,get}.go
│   └── metrics/overview.go
├── repository/postgres/
│   ├── review_query.go          (list + detail queries)
│   └── metrics_repo.go
└── delivery/http/handlers/
    ├── team_handler.go
    ├── apikey_handler.go
    ├── modelcfg_handler.go
    ├── reviews_handler.go
    └── metrics_handler.go
```

### Frontend additions
```
web/
├── app/(protected)/
│   ├── teams/{page,[id]/page}.tsx
│   ├── models/{page,[id]/page}.tsx
│   ├── reviews/{page,[id]/page}.tsx
│   └── metrics/page.tsx
├── components/
│   ├── teams/{team-list,team-form,member-list,apikey-list,apikey-modal}.tsx
│   ├── models/{model-list,model-form}.tsx
│   ├── reviews/{review-list,review-filters,review-detail,issue-card}.tsx
│   └── metrics/{stat-cards,trend-chart}.tsx
└── lib/api/
    ├── teams.ts
    ├── models.ts
    ├── reviews.ts
    └── metrics.ts
```

## Related Code Files

### Create (backend)
- `/internal/usecase/team/*.go` (5 files)
- `/internal/usecase/apikey/*.go` (3 files)
- `/internal/usecase/modelcfg/*.go` (3 files)
- `/internal/usecase/reviews/*.go` (2 files)
- `/internal/usecase/metrics/overview.go`
- `/internal/delivery/http/handlers/{team,apikey,modelcfg,reviews,metrics}_handler.go`
- `/internal/repository/postgres/{review_query,metrics_repo}.go`

### Create (frontend)
- Pages & components as tree above
- `web/lib/api/{teams,models,reviews,metrics}.ts` typed fetchers

### Modify
- `/internal/delivery/http/routes.go` — register new routes under JWT group
- `/internal/repository/postgres/model_config_repo.go` — add list/upsert methods (Phase 05 created skeleton)

## Implementation Steps

### Backend
1. **Team usecase + handler**: standard CRUD, `role=admin` required for write ops.
2. **API key endpoints**: issue returns plaintext ONCE; response `{id, name, prefix, token}` (token field hidden thereafter).
3. **Model config handler**: on create/update with `is_default=true`, unset other defaults in same tx.
4. **Reviews list query** (cursor = `created_at + id`):
   ```sql
   SELECT id, source, repo_full_name, external_pr_id, status, overall_approved, model_used, latency_ms, created_at
   FROM reviews
   WHERE team_id = $1 AND ($2::timestamp IS NULL OR created_at < $2)
     AND ($3::text IS NULL OR status = $3)
   ORDER BY created_at DESC
   LIMIT $4;
   ```
5. **Review detail**: join with `review_issues`.
6. **Metrics overview**:
   ```sql
   SELECT
     COUNT(*) FILTER (WHERE status='completed') AS completed,
     COUNT(*) FILTER (WHERE status='failed') AS failed,
     AVG(latency_ms) FILTER (WHERE status='completed') AS avg_latency,
     COUNT(DISTINCT repo_full_name) AS active_repos
   FROM reviews WHERE team_id = $1 AND created_at BETWEEN $2 AND $3;
   ```
7. Register all handlers under `auth` middleware group.

### Frontend
1. **API layer**: one file per resource with typed functions.
   ```ts
   // lib/api/teams.ts
   export const listTeams = () => api<Team[]>("/api/v1/teams");
   export const createTeam = (b: {name: string; slug: string}) => api<Team>("/api/v1/teams", {method:"POST", body: JSON.stringify(b)});
   ```
2. **Teams page**: React Query `useQuery("teams", listTeams)`, list + modal-form.
3. **Team detail**: tabs (members | api keys); API key create uses shadcn Dialog, shows token in monospace with Copy button — cleared on close.
4. **Models page**: table + drawer form. Enums:
   ```ts
   const PROVIDERS = ["anthropic","openai","google"] as const;
   const MODELS = {
     anthropic: ["claude-opus-4-7","claude-sonnet-4-6","claude-haiku-4-5"],
     openai: ["gpt-4o","gpt-4o-mini"],
     google: ["gemini-2.5-pro","gemini-2.5-flash"],
   } as const;
   ```
5. **Reviews list**: sticky filters (team, status, source), virtualized if rows > 100 (use `@tanstack/react-virtual`, only if needed; else skip per KISS).
6. **Review detail**: group issues by severity with color chip (red=critical, orange=high, yellow=medium, blue=low). Feedback widget → placeholder.
7. **Metrics page**: four StatCards + trend chart (lazy-loaded recharts).
8. **Admin guard**: if `user.role !== 'admin'`, hide team/model write actions client-side AND rely on server 403.

## Todo List

Backend:
- [ ] Team CRUD handlers + tests (list, create, get, update, delete)
- [ ] API key issue/list/revoke handlers + one-time-token semantics
- [ ] Model config CRUD with default-uniqueness enforcement
- [ ] Reviews list with cursor pagination
- [ ] Review detail returns issues inline
- [ ] Metrics overview aggregation query

Frontend:
- [ ] Typed fetchers for all resources
- [ ] Teams list + create + detail (members + API keys)
- [ ] API key copy-once modal
- [ ] Models list + form + default toggle
- [ ] Reviews list with filters + pagination
- [ ] Review detail with issue cards
- [ ] Metrics overview with 4 stat cards + trend chart
- [ ] Admin-only controls hidden for non-admins

## Success Criteria
- Full CRUD loop passes integration test: create team → add user → issue API key → create model config → list reviews.
- `npm run build` + `go build` clean.
- Manual smoke: navigate all pages, no console errors.

## Risk Assessment
- **Race on default model toggle**: handled in tx with `UPDATE ... SET is_default=FALSE WHERE team_id=$1; then INSERT/UPDATE ... is_default=TRUE`.
- **Frontend typed drift**: generate TS types from Go via OpenAPI stub OR hand-maintain with matching tests. Given YAGNI, hand-maintain for v1.
- **Metrics N+1**: single aggregation query per page load.

## Security Considerations
- All write endpoints require `role=admin` check in handler.
- API key plaintext shown ONCE; UI doesn't cache in state beyond modal close.
- Server-side admin enforcement (client checks are UX only).

## Next Steps
- **Unblocks**: Phase 10 (feedback widget), Phase 11 (slack settings added here later), Phase 12 (usage tab).
- **Parallel**: Phase 04, 05.
