# Phase 14 — Security Hardening & Performance Optimization

## Context Links
- Parent: [plan.md](plan.md)
- Research: `researcher-01-backend-report.md` §6 (rate limit, secrets), §8 (stack summary); `researcher-02-ai-feedback-report.md` §2 (pgvector tuning)
- Depends on: all prior phases; runs last

## Overview
- **Priority**: P1
- **Status**: pending
- **Effort**: 8h
- **Group**: E (final)
- **Description**: Production readiness pass. Column encryption for integration secrets, TLS enforcement, observability (metrics + traces), rate-limit upgrade to Redis, pgvector index tuning (ivfflat → HNSW), prompt-injection defense review, security headers, dependency scan. All changes are targeted edits to existing files (no new domains). Produces a `docs/runbook.md` for operators.

## Key Insights
- Encryption at column level via AES-GCM with key from KMS/env; encrypt `team_integrations.webhook_secret`, `.api_token`, `slack_channels.bot_token`.
- HNSW upgrade: better recall than ivfflat at cost of memory. Threshold: switch when `review_feedback` > 500k rows.
- Redis rate limiter already in place from Phase 04 (validated decision) — no migration needed here. Focus on tuning limits and adding per-route overrides.
<!-- Updated: Validation Session 1 - Redis already implemented in Phase 04 -->
- OpenTelemetry with OTLP exporter → visible metrics/traces. Jaeger/Tempo optional.
- Prompt injection via webhook diff content — already partially mitigated (XML wrapping in Phase 10); add explicit escape layer here.

## Requirements

### Functional
- New package `internal/security/crypto` for AES-GCM encrypt/decrypt.
- Migration `0013_encrypt_secrets.up.sql` re-encrypts existing plaintext secrets (re-run-safe).
- OpenTelemetry instrumentation: HTTP handlers, DB calls, AI provider calls, queue publish/consume.
- Prometheus `/metrics` endpoint.
- Redis-backed rate limiter behind feature flag (`RATE_LIMIT_BACKEND=memory|redis`).
- Security headers middleware: HSTS, CSP, X-Content-Type-Options, Referrer-Policy.
- Secrets scrubbing finalized: diff passed through `guardrail.Scrub` everywhere before prompts/logs.
- pgvector HNSW migration (behind feature flag; only if row count > threshold).
- Dependency scan: `govulncheck`, `npm audit`, fail CI on high/critical.
- Runbook: `/docs/runbook.md` with deploy, rotate, incident-response steps.

### Non-functional
- p95 review endpoint latency < 60s end-to-end (compose stack).
- `/metrics` scrape latency < 50ms.
- No plaintext secrets at rest.
- CI fails on CVE severity ≥ high.

## Architecture

```
internal/
├── security/
│   ├── crypto/
│   │   ├── aesgcm.go            # Encrypt(plain, key) / Decrypt(cipher, key)
│   │   ├── keystore.go          # load from env; KMS adapter stub
│   │   └── rotate.go            # re-encrypt helper
│   └── injection/
│       ├── diff_sanitizer.go    # strip prompt markers, length-limit, XML-wrap
│       └── feedback_sanitizer.go
├── observability/
│   ├── tracer.go                # OTel provider
│   ├── meter.go                 # OTel meter + prom bridge
│   ├── middleware.go            # echo middleware using otelecho
│   └── metrics.go               # custom counters/histograms
└── infrastructure/ratelimit/
    ├── memory.go                # existing (from Phase 04)
    ├── redis.go                 # new
    └── factory.go               # picks by config
```

## Related Code Files

### Create
- `/internal/security/crypto/{aesgcm,keystore,rotate}.go`
- `/internal/security/injection/{diff_sanitizer,feedback_sanitizer}.go`
- `/internal/observability/{tracer,meter,middleware,metrics}.go`
- `/internal/infrastructure/ratelimit/{redis,factory}.go`
- `/migrations/0013_encrypt_secrets.up.sql` + `.down.sql`
- `/migrations/0014_vector_hnsw.up.sql` + `.down.sql` (applied conditionally; see runbook)
- `/docs/runbook.md`
- `/scripts/security-scan.sh` — runs govulncheck + npm audit

### Modify
- `/internal/repository/postgres/integration_repo.go` — wrap reads/writes with crypto
- `/internal/repository/postgres/slack_channel_repo.go` — same
- `/internal/delivery/http/middleware/ratelimit.go` — swap to factory
- `/internal/delivery/http/routes.go` — add security headers + `/metrics`
- `/cmd/server/main.go` — init tracer, meter, secure middleware
- `/cmd/worker/main.go` — init tracer, meter
- `/internal/usecase/review/usecase.go` — sanitize diff before prompt build
- `/internal/usecase/feedback/create.go` — sanitize comment before embed text
- `/internal/config/config.go` — add `Telemetry`, `Encryption`, `RateLimitBackend`
- `/.github/workflows/ci.yml` — add security-scan job

## Implementation Steps

1. **AES-GCM** (`security/crypto/aesgcm.go`, <80 lines):
   ```go
   func Encrypt(plain []byte, key []byte) (string, error) {
       block, _ := aes.NewCipher(key); gcm, _ := cipher.NewGCM(block)
       nonce := make([]byte, gcm.NonceSize()); _, _ = rand.Read(nonce)
       ct := gcm.Seal(nil, nonce, plain, nil)
       return base64.StdEncoding.EncodeToString(append(nonce, ct...)), nil
   }
   func Decrypt(b64 string, key []byte) ([]byte, error) {
       raw, _ := base64.StdEncoding.DecodeString(b64)
       block, _ := aes.NewCipher(key); gcm, _ := cipher.NewGCM(block)
       ns := gcm.NonceSize()
       return gcm.Open(nil, raw[:ns], raw[ns:], nil)
   }
   ```

2. **Keystore**:
   ```go
   type KeyStore interface { DataKey() []byte; Version() int }
   type EnvKeyStore struct{ key []byte; v int }
   func LoadFromEnv() (KeyStore, error) { k, _ := base64.StdEncoding.DecodeString(os.Getenv("CRA_ENC_KEY")); if len(k) != 32 { return nil, errors.New("need 32-byte key") }; return &EnvKeyStore{k, 1}, nil }
   ```

3. **Migration `0013`**: add `secret_version SMALLINT` columns; backfill encrypting existing plaintext inline via `pgcrypto.pgp_sym_encrypt` fallback, OR run Go-side migration script. Chosen: Go-side script `scripts/encrypt-existing-secrets.go` called once after deploy.

4. **Wrap repo I/O**:
   ```go
   // integration_repo.go (modify)
   func (r *IntegRepo) Get(ctx, id) (*Integration, error) {
       row := r.pool.QueryRow(ctx, `SELECT ... webhook_secret, api_token FROM team_integrations WHERE id=$1`, id)
       var encSecret, encToken string; row.Scan(..., &encSecret, &encToken)
       plainSecret, err := crypto.Decrypt(encSecret, r.keys.DataKey())
       ...
   }
   ```

5. **Observability**:
   ```go
   // observability/tracer.go
   func New(ctx, svc string, endpoint string) (func(context.Context) error, error) {
       exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(endpoint), otlptracegrpc.WithInsecure())
       if err != nil { return nil, err }
       tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp), sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(svc))))
       otel.SetTracerProvider(tp)
       return tp.Shutdown, nil
   }
   ```
   Use `otelecho.Middleware(svc)` in Echo.
   Meter: export via `prometheus` exporter on `/metrics`.

6. **Custom metrics**:
   - `review_duration_seconds` (histogram, by model)
   - `ai_tokens_total` (counter, by direction, by model)
   - `queue_messages_total` (counter, by queue, by result)
   - `webhook_received_total` (counter, by source, by result)

7. **Redis rate limiter** (`ratelimit/redis.go`):
   ```go
   // Token bucket via Lua script INCRBY + TTL. Use github.com/go-redis/redis_rate.
   ```

8. **Security headers middleware**:
   ```go
   func SecureHeaders() echo.MiddlewareFunc {
       return func(next) echo.HandlerFunc {
           return func(c) error {
               h := c.Response().Header()
               h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
               h.Set("X-Content-Type-Options", "nosniff")
               h.Set("Referrer-Policy", "no-referrer")
               h.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
               return next(c)
           }
       }
   }
   ```

9. **Prompt injection defense** (`injection/diff_sanitizer.go`):
   ```go
   func Sanitize(diff string) string {
       // Strip leading instruction-style blocks outside diff hunks
       // Replace triple-backticks and 'ignore previous instructions' substrings via regex sentinel
       // Wrap whole thing in <diff>...</diff> at prompt build time
       return stripSuspicious(diff)
   }
   ```

10. **pgvector HNSW migration** (`0014`, optional):
    ```sql
    -- Apply ONLY when ivfflat becomes inadequate (manual gate via runbook)
    DROP INDEX IF EXISTS idx_feedback_embedding;
    CREATE INDEX idx_feedback_embedding ON review_feedback
        USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);
    ```

11. **Security scan script**:
    ```bash
    #!/usr/bin/env bash
    set -e
    go install golang.org/x/vuln/cmd/govulncheck@latest
    govulncheck ./...
    (cd web && npm audit --audit-level=high)
    ```

12. **Runbook** (`docs/runbook.md`) — sections: Deploy, Rotate Keys, Rotate Webhook Secrets, Rotate API Keys, Incident: AI Outage, Incident: Queue Backlog, Scale pgvector, Enable Redis Rate Limit, Release Checklist.

13. **Load test** (`scripts/loadtest.sh`):
    - `vegeta` POST /api/v1/reviews at 5 rps for 60s; assert p95 < 60s, error rate < 1%.

## Todo List

- [ ] AES-GCM encrypt/decrypt unit-tested (round-trip + tamper detection)
- [ ] Integration + slack repos transparently encrypt/decrypt
- [ ] Re-encryption script dry-run clean on seeded data
- [ ] OTel traces visible in Jaeger (manual check)
- [ ] Prometheus `/metrics` exposes custom metrics
- [ ] Redis rate limit feature-flagged; tested in compose.test with redis add-on
- [ ] Security headers present in responses (curl -I)
- [ ] Diff sanitizer strips known injection phrases; sanity-check tests
- [ ] `govulncheck` clean; `npm audit --audit-level=high` clean
- [ ] CI security-scan job fails on seeded vuln (test by pinning old dep)
- [ ] Runbook reviewed by ops-counterpart (if available) — otherwise documented as draft
- [ ] Load test meets SLOs

## Success Criteria
- `curl -I https://<host>/healthz` shows all security headers.
- `psql -c "SELECT webhook_secret FROM team_integrations LIMIT 1"` returns base64 blob, not plaintext.
- OTel traces connect webhook → queue → worker → AI → DB spans.
- `govulncheck` exit 0 in CI.
- p95 review latency < 60s under 5 rps sustained.

## Risk Assessment
- **Key rotation**: encrypting with v1 key; decrypt attempts v1 then v2. Old ciphertexts remain readable. Add `secret_version` column to disambiguate (already in migration 0013).
- **HNSW memory**: index can be 2-4x RAM of ivfflat. Roll out only after capacity review.
- **Telemetry overhead**: 5-10% p50 latency. Tolerable; can sample at 10% if painful.
- **CSP may break dashboard**: test `npm run build` output + all routes before deploy.

## Security Considerations
- All external secrets encrypted at rest.
- TLS required on ingress (terminated at reverse proxy; documented in runbook).
- HSTS preloaded.
- Dependency scanning gates merges.
- Key rotation plan documented.
- Log redaction: structured logger strips `Authorization`, `X-API-Key`, `X-Hub-Signature-256`.

## Next Steps
- **Unblocks**: production release.
- **Follow-ups (v2)**: multi-region, tenant-level encryption keys, audit log table, fine-tuned model training from feedback, agent tool-use loop.
