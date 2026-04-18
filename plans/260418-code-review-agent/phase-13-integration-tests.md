# Phase 13 — Integration Tests & Full-Stack Docker Compose

## Context Links
- Parent: [plan.md](plan.md)
- Research: implicit — validates all prior phases end-to-end
- Depends on: all prior phases (01-12)

## Overview
- **Priority**: P1
- **Status**: pending
- **Effort**: 8h
- **Group**: E (final)
- **Description**: Full-stack integration test harness. `docker-compose.test.yml` brings up real postgres+pgvector, rabbitmq, mock slack, mock GitHub/GitLab, server, worker. Go integration tests hit real HTTP endpoints and verify end-to-end flows. Adds CI wiring (GitHub Actions) and coverage reports.

## Key Insights
- Use `testcontainers-go` for inline test infra OR lean on compose-driven tests via `make itest`. Compose is simpler + reusable → choose compose.
- Mocks: minimal Go binaries for GitHub (`/repos/.../pulls/.../files`), GitLab, Slack (capture posted blocks). Serve on fixed ports.
- Mock AI provider: deterministic response matching input fixture — stable issue list for assertions.
- Integration tests in `/test/integration/`; unit tests stay alongside code under `internal/`.

## Requirements

### Functional
- `make itest` runs full suite against compose stack.
- Flows covered:
  1. Admin creates team + API key + model config via REST → verify DB.
  2. POST `/api/v1/reviews` with sample diff → review + issues persisted.
  3. POST `/webhooks/github` with signed payload → queue → worker → review persisted → mock GH receives inline comments.
  4. Feedback submission → embed job → vector stored → next review includes RAG samples.
  5. Usage recorded; `/api/v1/usage/*` returns non-zero.
  6. Slack mock receives block payload after webhook review.
  7. MCP `review_diff` called via stdio — returns structured output.
  8. Rate limit triggers 429 after burst.
- Cleanup: each test resets DB via transaction or truncate.

### Non-functional
- Full suite runs in < 5 min in CI.
- Parallelism: independent test files run parallel; DB isolated via per-file schema or tx rollback.
- Deterministic: mock AI has fixed response.

## Architecture

```
test/
├── integration/
│   ├── suite_test.go            # setup/teardown, shared fixtures
│   ├── auth_test.go
│   ├── team_apikey_test.go
│   ├── review_api_test.go
│   ├── webhook_github_test.go
│   ├── webhook_gitlab_test.go
│   ├── feedback_rag_test.go
│   ├── usage_test.go
│   ├── slack_test.go
│   ├── mcp_stdio_test.go
│   └── ratelimit_test.go
├── fixtures/
│   ├── diffs/                   # sample .diff files
│   ├── webhooks/                # GitHub/GitLab JSON samples with pre-computed signatures
│   └── ai_responses/            # mock provider responses
└── mocks/
    ├── github/main.go
    ├── gitlab/main.go
    ├── slack/main.go
    └── ai/main.go               # OpenAI+Anthropic mock
```

## Related Code Files

### Create
- `/test/integration/*.go` (test files listed above)
- `/test/fixtures/**` (diffs, webhooks, responses)
- `/test/mocks/{github,gitlab,slack,ai}/main.go`
- `/docker-compose.test.yml`
- `/scripts/itest.sh` — bring up compose.test, run tests, collect logs, tear down
- `/.github/workflows/ci.yml` — lint + unit + integration matrix
- `/test/helpers/{db,http,fixtures}.go` — shared helpers

### Modify
- `/Makefile` — add `itest`, `itest-logs`, `itest-down` targets.
- `/internal/infrastructure/ai/router.go` — allow `base_url` override so mock can intercept.
- `/internal/infrastructure/slack/client.go` — allow base URL override (env `SLACK_BASE_URL`).

## Implementation Steps

1. **`docker-compose.test.yml`**:
   ```yaml
   services:
     postgres:     { image: pgvector/pgvector:pg16, tmpfs: [/var/lib/postgresql/data] }
     rabbitmq:     { image: rabbitmq:3.13 }
     mock-github:  { build: ./test/mocks/github, ports: ["9100:9100"] }
     mock-gitlab:  { build: ./test/mocks/gitlab, ports: ["9101:9101"] }
     mock-slack:   { build: ./test/mocks/slack,  ports: ["9102:9102"] }
     mock-ai:      { build: ./test/mocks/ai,     ports: ["9103:9103"] }
     server:       { build: ., env_file: .env.test, depends_on: [postgres, rabbitmq, mock-ai] }
     worker:       { build: ., command: /app/worker, env_file: .env.test, depends_on: [rabbitmq, mock-github, mock-slack] }
   ```

2. **`.env.test`**:
   ```
   DB_DSN=postgres://cra:cra@postgres:5432/cra_test?sslmode=disable
   ANTHROPIC_BASE_URL=http://mock-ai:9103
   OPENAI_BASE_URL=http://mock-ai:9103
   GITHUB_API_BASE_URL=http://mock-github:9100
   GITLAB_API_BASE_URL=http://mock-gitlab:9101
   SLACK_BASE_URL=http://mock-slack:9102
   JWT_SECRET=test-secret-for-ci-only-32b
   API_KEY_PEPPER=test-pepper
   ```

3. **Mocks** (each < 120 lines):

   `test/mocks/ai/main.go`:
   ```go
   // Responds to /v1/messages with a deterministic canned review JSON.
   // Also supports /v1/embeddings returning pseudo-random 1536-d vector seeded by request hash.
   func main() {
       http.HandleFunc("/v1/messages", func(w, r) {
           body, _ := io.ReadAll(r.Body)
           fingerprint := sha1.Sum(body)
           resp := buildCannedResponse(hex.EncodeToString(fingerprint[:4]))
           json.NewEncoder(w).Encode(resp)
       })
       http.HandleFunc("/v1/embeddings", embedHandler)
       http.ListenAndServe(":9103", nil)
   }
   ```

   `test/mocks/slack/main.go`: POST `/`* captures body into in-memory ring buffer; GET `/_captures` returns them for test assertion.

   `test/mocks/github/main.go`: stubs `/repos/:owner/:repo/pulls/:num/files` + `POST .../reviews`.

4. **Suite setup** (`suite_test.go`):
   ```go
   var (app *TestApp)
   func TestMain(m *testing.M) {
       app = mustStart(ctx, ".env.test")
       code := m.Run()
       app.Stop()
       os.Exit(code)
   }
   func (a *TestApp) truncateAll() { ... }  // called per test
   ```
   `mustStart` reads from `TEST_STACK_STARTED=1` env (set by `scripts/itest.sh`) — avoids restarting compose between test files.

5. **Example test** (`review_api_test.go`):
   ```go
   func TestReviewAPI_HappyPath(t *testing.T) {
       defer app.truncateAll()
       team := app.seedTeam(t, "acme")
       key := app.issueAPIKey(t, team.ID)
       diff := fixtures.LoadDiff("add_auth.diff")

       resp := app.do(t, "POST", "/api/v1/reviews", key, &ReviewCreateReq{Diff: diff, RepoFullName: "acme/api"})
       require.Equal(t, 200, resp.Code)
       var r ReviewResp; _ = json.Unmarshal(resp.Body, &r)
       require.NotEmpty(t, r.ReviewID); require.NotEmpty(t, r.Issues)
       app.assertDBRow(t, "reviews", "id", r.ReviewID)
   }
   ```

6. **Webhook signed fixture**: precompute HMAC-SHA256 once, commit signature in fixture file or compute in test helper.

7. **Feedback+RAG test**:
   - Seed 3 feedback rows for team, trigger review.
   - Assert mock-AI received request whose body contains "Past lessons" prefix via substring match.

8. **MCP stdio test**: spawn `bin/mcp-server` subprocess, write JSON-RPC `tools/call` frame, read response, verify.

9. **Ratelimit test**: loop 100 requests with same API key → assert 429 by 10th within 1s (based on config).

10. **CI workflow** (`.github/workflows/ci.yml`):
    ```yaml
    jobs:
      lint:   { ... golangci-lint run }
      unit:   { ... go test ./internal/... -race -cover }
      itest:  { ... ./scripts/itest.sh }
      web-build: { ... cd web && npm ci && npm run build && npm run lint }
    ```

11. **Coverage artifacts**: `go tool cover -html` output uploaded on PR.

## Todo List

- [ ] Compose.test stack boots cleanly (healthchecks pass)
- [ ] 4 mock services running, reachable by server/worker
- [ ] Suite harness (`TestMain`, `truncateAll`, `do`, `seed*`) reusable
- [ ] Fixture diffs + webhook samples + precomputed sigs committed
- [ ] All 9 test files pass individually
- [ ] Full suite runs in `make itest` < 5 min
- [ ] CI workflow green on sample PR
- [ ] Coverage ≥ 60% across `internal/`

## Success Criteria
- Cold-start: `make itest` exits 0 on fresh checkout.
- Re-run: idempotent; no leftover state between runs.
- `go test -race ./...` clean.

## Risk Assessment
- **Flaky tests due to timing**: use polling helpers (`eventually(t, 10*time.Second, ...)`) not `time.Sleep`.
- **Worker startup race**: tests wait until health endpoint on server AND `rabbitmqctl list_queues` shows our queue declared.
- **Mock drift from real APIs**: keep mocks minimal; revalidate against real sandbox quarterly — tracked in roadmap.

## Security Considerations
- Test secrets in `.env.test` are NOT real; committed intentionally.
- Mock AI never reaches out; prevents leaking fixtures to vendors.
- CI runs in isolated network; no egress.

## Next Steps
- **Unblocks**: Phase 14 (security/perf hardening uses the same harness for regression testing).
- **Parallel**: none (final group).
