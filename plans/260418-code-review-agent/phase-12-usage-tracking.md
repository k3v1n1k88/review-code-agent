# Phase 12 — AI Usage Tracking & Pricing Dashboard

## Context Links
- Parent: [plan.md](plan.md)
- Research: `researcher-02-ai-feedback-report.md` §5 (token tracking, dedup, billing formula)
- Depends on: Phase 02 (`ai_usage_logs`), Phase 05 (`Usage` from provider), Phase 06 (dashboard shell)

## Overview
- **Priority**: P2
- **Status**: pending
- **Effort**: 8h
- **Group**: D (parallel with 10, 11)
- **Description**: Record every AI call (input/output/cache tokens, latency, cost). Per-team daily aggregates via materialized view. Dashboard page with cost-per-day line, tokens-by-model bar, cache savings card, top-expensive-reviews table. Cost formula from research §5.4.

## Key Insights
- Dedup by `message_id` (Anthropic returns same ID for parallel tool calls) — unique index in Phase 02.
- Cost calculated at write time (not read) for indexing/aggregation simplicity.
- Prices stored in config (env/YAML) — editable without migration; default to known Claude prices.
- Materialized view refreshed every 10 min via cron (pg_cron) OR on-demand. YAGNI → on-demand refresh endpoint hit by dashboard on interval.

## Requirements

### Functional
- `usage.Recorder.Record(ctx, teamID, reviewID, messageID, model, usage, latency)` writes one row.
- Cost calculator supports Anthropic, OpenAI, Google per-model pricing.
- Dashboard page `/usage`:
  - KPI cards: 30-day total cost, cache savings, reviews count, avg cost/review.
  - Line chart: daily cost (last 30 days).
  - Bar chart: tokens by model (last 30 days).
  - Table: top 20 most expensive reviews (link to detail).
- CSV export: `GET /api/v1/usage.csv?team_id=&from=&to=`.
- Admin-level cross-team usage view (sum over all teams).

### Non-functional
- Recorder latency < 10ms (async-friendly; uses same pgxpool).
- Dashboard queries < 500ms (aggregations use MV or indexed roll-ups).
- CSV export up to 1M rows streamed.

## Architecture

```
internal/
├── usecase/usage/
│   ├── recorder.go              # Record + cost calc
│   ├── pricing.go               # price table + calc()
│   ├── query.go                 # aggregation queries
│   └── export.go                # CSV writer
├── repository/postgres/
│   ├── usage_repo.go            # insert + select aggregates
│   └── usage_views.sql          # materialized view DDL (applied as migration 0012)
└── delivery/http/handlers/
    └── usage_handler.go

web/
├── app/(protected)/usage/
│   └── page.tsx
├── components/usage/
│   ├── kpi-cards.tsx
│   ├── cost-line-chart.tsx
│   ├── tokens-model-bar.tsx
│   ├── top-expensive-table.tsx
│   └── export-button.tsx
└── lib/api/usage.ts
```

## Related Code Files

### Create
- `/internal/usecase/usage/{recorder,pricing,query,export}.go`
- `/internal/repository/postgres/usage_repo.go`
- `/internal/delivery/http/handlers/usage_handler.go`
- `/migrations/0012_usage_views.up.sql` + `.down.sql`
- `/web/app/(protected)/usage/page.tsx`
- `/web/components/usage/{kpi-cards,cost-line-chart,tokens-model-bar,top-expensive-table,export-button}.tsx`
- `/web/lib/api/usage.ts`

### Modify
- `/internal/usecase/review/usecase.go` — call `recorder.Record` after provider call (pass message_id + usage).
- `/internal/config/config.go` — add `Pricing` map (per-provider, per-model rates).
- `/internal/delivery/http/routes.go` — register `/api/v1/usage/*`.

## Implementation Steps

1. **Pricing table** (`pricing.go`):
   ```go
   type ModelPrice struct { Input, Output, CacheWrite, CacheRead float64 } // USD per 1M tokens
   var DefaultPricing = map[string]map[string]ModelPrice{
       "anthropic": {
           "claude-opus-4-7":    {Input: 5, Output: 15, CacheWrite: 6.25, CacheRead: 0.5},
           "claude-sonnet-4-6":  {Input: 3, Output: 9,  CacheWrite: 3.75, CacheRead: 0.3},
           "claude-haiku-4-5":   {Input: 1, Output: 5,  CacheWrite: 1.25, CacheRead: 0.1},
       },
       "openai": {
           "gpt-4o":      {Input: 2.5, Output: 10, CacheWrite: 2.5, CacheRead: 1.25},
           "gpt-4o-mini": {Input: 0.15, Output: 0.6, CacheWrite: 0.15, CacheRead: 0.075},
       },
   }
   func Calc(p ModelPrice, u TokenUsage) float64 {
       return ((float64(u.Input)-float64(u.CacheRead))*p.Input +
               float64(u.Output)*p.Output +
               float64(u.CacheCreation)*p.CacheWrite +
               float64(u.CacheRead)*p.CacheRead) / 1_000_000
   }
   ```

2. **Recorder**:
   ```go
   func (r *Recorder) Record(ctx, in RecordIn) error {
       price, _ := r.pricing.Get(in.Provider, in.Model)
       cost := Calc(price, in.Usage)
       return r.repo.Insert(ctx, Row{
           TeamID: in.TeamID, ReviewID: in.ReviewID, MessageID: in.MessageID,
           Provider: in.Provider, Model: in.Model,
           InputTokens: in.Usage.Input, OutputTokens: in.Usage.Output,
           CacheCreationTokens: in.Usage.CacheCreation, CacheReadTokens: in.Usage.CacheRead,
           Latency: in.Latency, CostUSD: cost,
       })
   }
   ```
   Unique index on `message_id` handles dedup (return silently if conflict).

3. **Materialized view migration** (`0012_usage_views.up.sql`):
   ```sql
   CREATE MATERIALIZED VIEW mv_usage_daily AS
   SELECT team_id, DATE(created_at) AS d,
          model_name,
          COUNT(*) AS reviews,
          SUM(input_tokens) AS input_total,
          SUM(output_tokens) AS output_total,
          SUM(cache_read_tokens) AS cache_read_total,
          SUM(cache_creation_tokens) AS cache_creation_total,
          SUM(cost_usd) AS cost_total
   FROM ai_usage_logs
   GROUP BY team_id, DATE(created_at), model_name;
   CREATE UNIQUE INDEX mv_usage_daily_pk ON mv_usage_daily(team_id, d, model_name);
   ```

4. **Refresh strategy**: on-demand via `REFRESH MATERIALIZED VIEW CONCURRENTLY mv_usage_daily` called by dashboard query IF stale > 10min. Tracked via small `pg_meta` table OR simple Go-side cache of `last_refresh`.

5. **Queries** (`query.go`):
   ```go
   func (q *Query) KPIs(ctx, teamID uuid.UUID, days int) (KPI, error) {
       // single-row aggregate
   }
   func (q *Query) DailySeries(ctx, teamID uuid.UUID, days int) ([]DayPoint, error) {
       // SELECT d, SUM(cost_total) FROM mv_usage_daily WHERE team_id=$1 AND d >= CURRENT_DATE - $2 GROUP BY d ORDER BY d
   }
   func (q *Query) TokensByModel(ctx, teamID uuid.UUID, days int) ([]ModelTotal, error) { ... }
   func (q *Query) TopExpensive(ctx, teamID uuid.UUID, limit int) ([]ExpensiveReview, error) {
       // SELECT review_id, SUM(cost_usd), model_name, created_at FROM ai_usage_logs WHERE team_id=$1 GROUP BY review_id ... ORDER BY sum DESC LIMIT $2
   }
   ```

6. **CSV export** (`export.go`) — stream row-by-row via `csv.NewWriter`:
   ```go
   func (e *Exporter) WriteCSV(ctx, w io.Writer, teamID uuid.UUID, from, to time.Time) error {
       cw := csv.NewWriter(w); defer cw.Flush()
       cw.Write([]string{"timestamp","team_id","review_id","model","input","output","cache_read","cache_creation","latency_ms","cost_usd"})
       return e.repo.StreamRows(ctx, teamID, from, to, func(r Row) error {
           return cw.Write([]string{...})
       })
   }
   ```

7. **Handlers**:
   ```go
   auth.GET("/usage/kpis",          h.KPIs)
   auth.GET("/usage/daily",         h.Daily)
   auth.GET("/usage/tokens-by-model", h.ByModel)
   auth.GET("/usage/top-expensive", h.Top)
   auth.GET("/usage.csv",           h.ExportCSV)
   ```

8. **Wire recorder in Phase 05**:
   ```go
   // review/usecase.go (modify)
   r.usage.Record(ctx, usage.RecordIn{ TeamID: req.TeamID, ReviewID: review.ID, MessageID: out.MessageID, Provider: provider.Name(), Model: out.Model, Usage: out.Usage, Latency: latency })
   ```

9. **Frontend** — KPI cards, recharts line + bar, table. Each component <120 lines; use shadcn `Card`, `Table`.
   ```tsx
   // page.tsx
   const { data: kpis } = useQuery({ queryKey: ["usage-kpis"], queryFn: () => api("/api/v1/usage/kpis?days=30") });
   return <div className="grid gap-4">
     <KpiCards data={kpis} />
     <CostLineChart />
     <TokensModelBar />
     <TopExpensiveTable />
   </div>;
   ```

10. **Admin cross-team** (optional, guarded by role=`platform_admin` — new enum value; skip for v1 YAGNI, document for v2).

## Todo List

- [ ] `0012` migration creates MV + unique index
- [ ] `Calc` formula unit-tested against research §5.4 examples
- [ ] Recorder dedup on duplicate `message_id` works
- [ ] All four query endpoints return within 500ms on 100k log rows (seeded)
- [ ] CSV export streams (manual test with 100k rows doesn't OOM)
- [ ] `/usage` page renders all widgets with correct data
- [ ] Currency formatter shows `$0.0012` precision
- [ ] Recorder added to review pipeline — verified by end-to-end review test

## Success Criteria
- Run 10 reviews → `/usage` shows correct cumulative cost.
- Verify cache savings: review 1 writes cache (cost ~1.25x); review 2 reads cache (cost ~0.1x input portion). Both reflected.
- CSV download matches database rows exactly.

## Risk Assessment
- **Pricing drift**: vendor prices change; config-loadable table lets ops update without deploy.
- **MV staleness**: on-demand refresh is fine for internal tool. Phase 14 adds pg_cron if needed.
- **Cost of embedding (Phase 10)**: track separately OR roll into same table with `provider='openai', model='text-embedding-3-small'`. Roll-in chosen → simpler.

## Security Considerations
- `team_id` scope enforced in all queries.
- CSV export route still requires JWT; consider streaming content-length to prevent memory blow-up.
- Pricing config not sensitive, but deploy config should be version-controlled separate from code.

## Next Steps
- **Unblocks**: None on critical path. Dashboards visible pre-integration-test.
- **Parallel**: Phase 10, 11.
