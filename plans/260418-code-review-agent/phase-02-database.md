# Phase 02 — Database Schema & Migrations (PostgreSQL + pgvector)

## Context Links
- Parent: [plan.md](plan.md)
- Research: `research/researcher-02-ai-feedback-report.md` §2 (pgvector), §5 (usage tracking)

## Overview
- **Priority**: P1
- **Status**: pending
- **Effort**: 8h
- **Group**: A (parallel — no deps)
- **Description**: Full schema: teams, users, api_keys, model_configs, reviews, review_issues, review_feedback (pgvector), ai_usage_logs, slack_channels. Migrations with `golang-migrate`. Idempotent up/down pairs.

## Key Insights
- Research §2.1: `vector(1536)` for embedding; ivfflat index with `lists=100` (adjust when rows > 100k → switch to HNSW).
- Research §5.2: usage logs with `cache_creation_tokens` and `cache_read_tokens` broken out — critical for cache-savings metrics.
- Admin issues API keys; hash with SHA256 + pepper before storing (prefix stored in plaintext for lookup hint).
- Team is the tenant unit for reviews, feedback, and usage.

## Requirements

### Functional
- All tables created idempotently via numbered SQL migrations.
- `pgvector` extension loaded.
- Foreign keys + cascade deletes where appropriate (review_issues → reviews).
- `golang-migrate` CLI integration in Makefile.

### Non-functional
- PostgreSQL 16.x, pgvector 0.7+.
- Index coverage for: team scoping, PR lookup, diff hash lookup, cosine similarity.
- `gen_random_uuid()` via pgcrypto for public IDs.

## Architecture

### Entity list

| Table | Purpose |
|-------|---------|
| `teams` | Tenant |
| `users` | Admin users (bcrypt password) |
| `api_keys` | Per-team API tokens |
| `model_configs` | Team-level AI model selection |
| `reviews` | One row per PR review run |
| `review_issues` | Inline findings (file, line, severity) |
| `review_feedback` | Human rating + embedding for RAG |
| `ai_usage_logs` | Token accounting |
| `slack_channels` | Team → channel mapping |
| `webhook_deliveries` | Idempotency log for webhook dedupe |

## Related Code Files

### Create
- `/migrations/0001_init_extensions.up.sql` + `.down.sql`
- `/migrations/0002_teams_users.up.sql` + `.down.sql`
- `/migrations/0003_api_keys.up.sql` + `.down.sql`
- `/migrations/0004_model_configs.up.sql` + `.down.sql`
- `/migrations/0005_reviews.up.sql` + `.down.sql`
- `/migrations/0006_review_issues.up.sql` + `.down.sql`
- `/migrations/0007_review_feedback.up.sql` + `.down.sql`
- `/migrations/0008_ai_usage_logs.up.sql` + `.down.sql`
- `/migrations/0009_slack_channels.up.sql` + `.down.sql`
- `/migrations/0010_webhook_deliveries.up.sql` + `.down.sql`
- `/scripts/migrate.sh` — wraps `migrate -database $DB_DSN -path ./migrations`
- `/internal/repository/postgres/schema.go` — constants for table names + sql.go helpers

### Modify
- `/Makefile` — add `migrate-up`, `migrate-down`, `migrate-new NAME=x`

## Implementation Steps

1. **0001_init_extensions.up.sql**:
   ```sql
   CREATE EXTENSION IF NOT EXISTS vector;
   CREATE EXTENSION IF NOT EXISTS pgcrypto;
   CREATE EXTENSION IF NOT EXISTS pg_trgm;
   ```

2. **0002_teams_users.up.sql**:
   ```sql
   CREATE TABLE teams (
       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       name TEXT NOT NULL UNIQUE,
       slug TEXT NOT NULL UNIQUE,
       created_at TIMESTAMPTZ NOT NULL DEFAULT now()
   );
   CREATE TABLE users (
       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
       username TEXT NOT NULL UNIQUE,
       password_hash TEXT NOT NULL,
       role TEXT NOT NULL CHECK (role IN ('admin','member')),
       created_at TIMESTAMPTZ NOT NULL DEFAULT now()
   );
   CREATE INDEX idx_users_team ON users(team_id);
   ```

3. **0003_api_keys.up.sql**:
   ```sql
   CREATE TABLE api_keys (
       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
       name TEXT NOT NULL,
       prefix TEXT NOT NULL,                -- first 8 chars, plaintext, for lookup
       token_hash TEXT NOT NULL UNIQUE,     -- SHA256(token + pepper)
       last_used_at TIMESTAMPTZ,
       revoked_at TIMESTAMPTZ,
       created_at TIMESTAMPTZ NOT NULL DEFAULT now()
   );
   CREATE INDEX idx_apikeys_prefix ON api_keys(prefix);
   CREATE INDEX idx_apikeys_team ON api_keys(team_id) WHERE revoked_at IS NULL;
   ```

4. **0004_model_configs.up.sql**:
   ```sql
   CREATE TABLE model_configs (
       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
       provider TEXT NOT NULL,               -- 'anthropic','openai','google'
       model_name TEXT NOT NULL,             -- 'claude-opus-4-7', etc
       max_tokens INT NOT NULL DEFAULT 4096,
       temperature NUMERIC(3,2) NOT NULL DEFAULT 0.2,
       system_prompt TEXT,
       is_default BOOLEAN NOT NULL DEFAULT FALSE,
       created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
       updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
   );
   CREATE UNIQUE INDEX uq_default_model_per_team ON model_configs(team_id) WHERE is_default = TRUE;
   ```

5. **0005_reviews.up.sql**:
   ```sql
   CREATE TABLE reviews (
       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
       source TEXT NOT NULL CHECK (source IN ('github','gitlab','api','mcp')),
       external_pr_id TEXT,                  -- owner/repo#123
       repo_full_name TEXT,
       head_sha TEXT,
       base_sha TEXT,
       diff_hash VARCHAR(64) NOT NULL,
       status TEXT NOT NULL CHECK (status IN ('queued','processing','completed','failed')),
       summary TEXT,
       overall_approved BOOLEAN,
       model_used TEXT,
       latency_ms INT,
       error_message TEXT,
       created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
       completed_at TIMESTAMPTZ
   );
   CREATE INDEX idx_reviews_team_created ON reviews(team_id, created_at DESC);
   CREATE INDEX idx_reviews_pr ON reviews(repo_full_name, external_pr_id);
   CREATE INDEX idx_reviews_diff_hash ON reviews(diff_hash);
   ```

6. **0006_review_issues.up.sql**:
   ```sql
   CREATE TABLE review_issues (
       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       review_id UUID NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
       file_path TEXT NOT NULL,
       line_number INT,
       severity TEXT NOT NULL CHECK (severity IN ('low','medium','high','critical')),
       issue_type TEXT NOT NULL,              -- security, performance, style, test, bug
       message TEXT NOT NULL,
       suggestion TEXT,
       confidence TEXT CHECK (confidence IN ('low','medium','high')),
       posted_as_comment BOOLEAN NOT NULL DEFAULT FALSE,
       external_comment_id TEXT
   );
   CREATE INDEX idx_issues_review ON review_issues(review_id);
   ```

7. **0007_review_feedback.up.sql**:
   ```sql
   CREATE TABLE review_feedback (
       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       review_id UUID NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
       team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
       user_id UUID REFERENCES users(id),
       rating INT CHECK (rating BETWEEN 1 AND 5),
       feedback_type TEXT CHECK (feedback_type IN ('positive','negative','correction')),
       comment TEXT,
       embedding vector(1536),
       created_at TIMESTAMPTZ NOT NULL DEFAULT now()
   );
   CREATE INDEX idx_feedback_team ON review_feedback(team_id);
   CREATE INDEX idx_feedback_review ON review_feedback(review_id);
   -- Cosine similarity index (tune `lists` ~ sqrt(row_count))
   CREATE INDEX idx_feedback_embedding ON review_feedback
       USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
   ```

8. **0008_ai_usage_logs.up.sql**:
   ```sql
   CREATE TABLE ai_usage_logs (
       id BIGSERIAL PRIMARY KEY,
       team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
       review_id UUID REFERENCES reviews(id) ON DELETE SET NULL,
       message_id TEXT,                       -- Anthropic message id (dedupe)
       provider TEXT NOT NULL,
       model_name TEXT NOT NULL,
       input_tokens INT NOT NULL DEFAULT 0,
       output_tokens INT NOT NULL DEFAULT 0,
       cache_creation_tokens INT NOT NULL DEFAULT 0,
       cache_read_tokens INT NOT NULL DEFAULT 0,
       num_tool_calls INT NOT NULL DEFAULT 0,
       latency_ms INT,
       cost_usd NUMERIC(12,6) NOT NULL DEFAULT 0,
       created_at TIMESTAMPTZ NOT NULL DEFAULT now()
   );
   CREATE UNIQUE INDEX uq_usage_message ON ai_usage_logs(message_id) WHERE message_id IS NOT NULL;
   CREATE INDEX idx_usage_team_date ON ai_usage_logs(team_id, created_at DESC);
   ```

9. **0009_slack_channels.up.sql**:
   ```sql
   CREATE TABLE slack_channels (
       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
       channel_id TEXT NOT NULL,
       channel_name TEXT,
       webhook_url TEXT,
       notify_on TEXT[] NOT NULL DEFAULT ARRAY['completed','failed'],
       created_at TIMESTAMPTZ NOT NULL DEFAULT now()
   );
   CREATE UNIQUE INDEX uq_slack_team_channel ON slack_channels(team_id, channel_id);
   ```

10. **0010_webhook_deliveries.up.sql** (idempotency):
    ```sql
    CREATE TABLE webhook_deliveries (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source TEXT NOT NULL,                -- github/gitlab
        delivery_id TEXT NOT NULL,           -- X-GitHub-Delivery or x-request-id
        received_at TIMESTAMPTZ NOT NULL DEFAULT now()
    );
    CREATE UNIQUE INDEX uq_delivery ON webhook_deliveries(source, delivery_id);
    ```

11. **Matching `.down.sql`**: `DROP TABLE ... CASCADE;` in reverse dependency order.

12. **`scripts/migrate.sh`**:
    ```bash
    #!/usr/bin/env bash
    set -euo pipefail
    migrate -database "${DB_DSN}" -path ./migrations "${@:-up}"
    ```

13. **`internal/repository/postgres/schema.go`**: define constants for table names to avoid typos in repo code.

## Todo List

- [ ] `0001`..`0010` up/down migrations authored
- [ ] `migrate up` runs clean on empty DB
- [ ] `migrate down` reverses cleanly
- [ ] `\d+` inspection shows all indexes present
- [ ] `INSERT INTO teams` + `INSERT INTO review_feedback` with dummy vector succeeds
- [ ] Cosine query returns expected row
- [ ] pgvector extension verified: `SELECT extname FROM pg_extension`

## Success Criteria
- `make migrate-up` exits 0; `psql -c "\dt"` shows all 10 tables.
- `SELECT indexdef FROM pg_indexes WHERE tablename='review_feedback'` contains `ivfflat` index.
- Round-trip: insert feedback with random `vector(1536)`, cosine-sort returns it.

## Risk Assessment
- **ivfflat tuning**: `lists=100` fine for <100k rows; re-plan to HNSW post-1M rows (Phase 14 perf).
- **Migration drift**: enforce sequential numbering; `migrate` refuses gaps.
- **Cascade scope**: team delete cascades to EVERYTHING — dashboard must show confirmation (Phase 06).

## Security Considerations
- Password stored as bcrypt hash (Phase 04 writes it; schema only needs TEXT).
- API key `token_hash` is SHA256 of `token||pepper`; token never persisted.
- No PII in `reviews` table (diff content not stored; only hash + summary).

## Next Steps
- **Unblocks**: Phase 04 (auth queries), Phase 05 (review writes), Phase 06 (dashboard reads), Phase 10 (feedback+RAG), Phase 12 (usage aggregation).
- **Parallel**: Phase 01, 03.
