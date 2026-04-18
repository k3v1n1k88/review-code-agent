# Phase 07 — Webhook Handler (GitHub/GitLab)

## Context Links
- Parent: [plan.md](plan.md)
- Research: `researcher-01-backend-report.md` §5 (webhook setup, signature verification)
- Depends on: Phase 04 (middleware), Phase 05 (review engine). Phase 09 for async enqueue (runs inline if queue absent — fail-soft).

## Overview
- **Priority**: P1
- **Status**: pending
- **Effort**: 8h
- **Group**: C (parallel with 08, 09)
- **Description**: Webhook endpoints for GitHub + GitLab. Verify signatures, dedupe deliveries, fetch full diff via platform API, enqueue review job (Phase 09 queue) or invoke synchronously if queue disabled, post results back as PR comment + inline review comments after completion.

## Key Insights
- **GitHub signature** (research §5): HMAC-SHA256 via `X-Hub-Signature-256`; use `hmac.Equal` (constant time).
- **GitLab signature**: plain string compare `X-Gitlab-Token`; rely on IP allowlist in prod.
- **Webhook timeout 30s**: return 202 immediately, process async.
- **Diff fetch**: webhook payload only has metadata; fetch real diff via `GET /repos/.../pulls/N/files` (GitHub) or `/merge_requests/IID/changes` (GitLab).
- **Idempotency**: store `(source, delivery_id)` in `webhook_deliveries` table (Phase 02) before processing; 200-noop on duplicate.
- **Repo secret** stored per-team (new column — bubble back to schema? No — use `slack_channels`-style `integrations` table). See step below.

## Requirements

### Functional
- `POST /webhooks/github` (public) — verifies `X-Hub-Signature-256`.
- `POST /webhooks/gitlab` (public) — verifies `X-Gitlab-Token`.
- Accept events: `pull_request.opened`, `pull_request.synchronize` (GitHub); `merge_request.open`, `merge_request.update` (GitLab).
- Fetch diff via API using per-team installation token (GitHub App) OR PAT stored in team integration.
- Enqueue `ReviewJob` via RabbitMQ (Phase 09) with correlation ID.
- Worker (Phase 09) invokes `review.Usecase.Run` → posts PR comment with summary + inline comments for each issue.
- Handle `closed`/`merged` events: no-op.

### Non-functional
- Webhook p95 < 2s (signature + enqueue only; reviewing is async).
- Deliver-once guarantee via delivery_id dedupe.
- 5xx retries are safe (idempotent by delivery_id).

## Architecture

```
internal/
├── delivery/http/webhook/
│   ├── github_handler.go        # <150 lines
│   ├── gitlab_handler.go        # <150 lines
│   ├── signature.go             # HMAC + plain compare
│   └── types.go                 # event DTOs
├── usecase/webhook/
│   ├── dedupe.go                # webhook_deliveries insert (unique violation = dup)
│   ├── dispatch.go              # build ReviewJob, publish to queue
│   └── post_results.go          # (worker side) poster: PR comment + inline comments
├── infrastructure/
│   ├── github/
│   │   ├── client.go            # go-github or minimal custom
│   │   ├── diff.go              # GetPRFiles, GetPRDiff
│   │   └── comments.go          # CreateReview, CreateReviewComment
│   └── gitlab/
│       ├── client.go            # go-gitlab
│       ├── diff.go              # GetMRChanges
│       └── comments.go          # CreateMRNote, CreateMRDiscussion
└── repository/postgres/
    ├── webhook_delivery_repo.go
    └── integration_repo.go      # team_integrations table (see step 1)
```

## Related Code Files

### Create
- `/internal/delivery/http/webhook/{github_handler,gitlab_handler,signature,types}.go`
- `/internal/usecase/webhook/{dedupe,dispatch,post_results}.go`
- `/internal/infrastructure/github/{client,diff,comments}.go`
- `/internal/infrastructure/gitlab/{client,diff,comments}.go`
- `/internal/repository/postgres/{webhook_delivery_repo,integration_repo}.go`
- `/migrations/0011_team_integrations.up.sql` + `.down.sql`

### Modify
- `/internal/delivery/http/routes.go` — add public `/webhooks/*` with bodyLimit middleware (8MiB).
- `/internal/config/config.go` — optional `GitHubAppID`, `GitHubPrivateKey` (fallback to per-team PAT).

## Implementation Steps

1. **Migration `0011_team_integrations`**:
   ```sql
   CREATE TABLE team_integrations (
       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
       provider TEXT NOT NULL CHECK (provider IN ('github','gitlab')),
       webhook_secret TEXT NOT NULL,            -- encrypted at rest (Phase 14)
       api_token TEXT NOT NULL,                 -- PAT or installation token (encrypted)
       repo_filter TEXT[] DEFAULT '{}',         -- empty = all repos for this team
       created_at TIMESTAMPTZ NOT NULL DEFAULT now()
   );
   CREATE UNIQUE INDEX uq_team_integration ON team_integrations(team_id, provider);
   ```

2. **Signature verification** (`signature.go`):
   ```go
   func VerifyGitHub(secret []byte, sig string, body []byte) bool {
       mac := hmac.New(sha256.New, secret)
       mac.Write(body)
       exp := "sha256=" + hex.EncodeToString(mac.Sum(nil))
       return hmac.Equal([]byte(exp), []byte(sig))
   }
   func VerifyGitLab(secret, token string) bool {
       return subtle.ConstantTimeCompare([]byte(secret), []byte(token)) == 1
   }
   ```

3. **GitHub handler** (<150 lines):
   ```go
   func (h *GitHubHandler) Handle(c echo.Context) error {
       body, err := io.ReadAll(c.Request().Body); if err != nil { return echo.ErrBadRequest }
       delivery := c.Request().Header.Get("X-GitHub-Delivery")
       event := c.Request().Header.Get("X-GitHub-Event")
       sig := c.Request().Header.Get("X-Hub-Signature-256")

       if event != "pull_request" { return c.NoContent(204) }

       var payload PREvent; if err := json.Unmarshal(body, &payload); err != nil { return echo.ErrBadRequest }
       if !(payload.Action == "opened" || payload.Action == "synchronize") { return c.NoContent(204) }

       integ, err := h.integ.Find(c.Request().Context(), "github", payload.Repository.FullName)
       if err != nil { return echo.ErrNotFound }
       if !VerifyGitHub([]byte(integ.WebhookSecret), sig, body) { return echo.ErrUnauthorized }

       if dup, _ := h.dedupe.Check(ctx, "github", delivery); dup { return c.NoContent(200) }

       job := ReviewJob{
           TeamID: integ.TeamID, Source: "github", Delivery: delivery,
           RepoFullName: payload.Repository.FullName, PRNumber: payload.Number,
           HeadSHA: payload.PullRequest.Head.SHA, BaseSHA: payload.PullRequest.Base.SHA,
       }
       if err := h.queue.Publish(ctx, job); err != nil {
           // fail-soft: process inline with timeout
           go h.fallback.Run(context.Background(), job)
       }
       return c.JSON(202, map[string]string{"status":"queued","delivery_id": delivery})
   }
   ```

4. **GitLab handler** — symmetric, event `Merge Request Hook`, action `open|update`.

5. **Integration lookup** — `integration_repo.FindByRepo(source, repoFullName)` queries team whose `repo_filter` contains the repo (or is empty).

6. **GitHub client** (`infrastructure/github`): use `github.com/google/go-github/v62` for diff + comments. Wrap in thin interface `GitHubAPI`.
   ```go
   type GitHubAPI interface {
       GetPRFiles(ctx, owner, repo string, num int) ([]File, error)
       GetPRDiff(ctx, owner, repo string, num int) (string, error)
       CreateReview(ctx, owner, repo string, num int, body string, comments []InlineComment) error
   }
   ```

7. **GitLab client** (`infrastructure/gitlab`): `github.com/xanzy/go-gitlab`.

8. **Worker-side poster** (`post_results.go`): invoked by Phase 09 worker AFTER `review.Usecase.Run`:
   ```go
   func (p *Poster) Post(ctx, review Review, issues []Issue, integ TeamIntegration) error {
       summary := renderMarkdown(review.Summary, issues)
       inline := make([]InlineComment, 0, len(issues))
       for _, i := range issues { if i.LineNumber > 0 { inline = append(inline, InlineComment{Path:i.FilePath, Line:i.LineNumber, Body:renderIssue(i)}) } }
       switch review.Source {
       case "github": return p.gh.CreateReview(ctx, owner, repo, num, summary, inline)
       case "gitlab": /* note + discussions */
       }
       return nil
   }
   ```

9. **Update `review_issues.posted_as_comment`** + `external_comment_id` after successful post.

10. **Routes**:
    ```go
    pub := e.Group("/webhooks", mw.BodyLimit("8MB"), mw.RateLimitIP)
    pub.POST("/github", h.GitHub.Handle)
    pub.POST("/gitlab", h.GitLab.Handle)
    ```

## Todo List

- [ ] Migration `0011` applied
- [ ] Signature verification tests (golden sig from GitHub docs sample)
- [ ] GitHub handler: happy path, invalid sig (401), dup delivery (200), unsupported action (204)
- [ ] GitLab handler: same coverage
- [ ] `go-github` diff fetch hits live sandbox OK
- [ ] `go-gitlab` diff fetch hits live sandbox OK
- [ ] Queue publish on success; fail-soft fallback path tested
- [ ] Poster creates GH review + inline comments in sandbox repo
- [ ] `review_issues.posted_as_comment=true` after post

## Success Criteria
- Curl replay of real GitHub webhook payload (with valid sig) → 202 + job published.
- Invalid sig → 401.
- Duplicate delivery → 200 no-op.
- Manual PR creation in sandbox repo → review posted with ≥1 inline comment within 60s.

## Risk Assessment
- **Webhook payload > 8MB**: document limit; PRs with huge files skip diff enrichment (use webhook metadata only).
- **GitHub rate limits** (5000/hr): cache diff per delivery_id.
- **GitLab auth weakness**: enforce IP allowlist at nginx/traefik layer in production (Phase 14).
- **Comment spam on synchronize**: optional — dedupe by `diff_hash` to skip re-review if unchanged (Phase 14 enhancement).

## Security Considerations
- Constant-time signature compare.
- Secrets in `team_integrations` encrypted at rest (Phase 14 adds AES-GCM column encryption).
- Webhook endpoints behind IP allowlist in production.
- Body-limit middleware prevents DoS.

## Next Steps
- **Unblocks**: Phase 11 (Slack posts after webhook review completes).
- **Parallel**: Phase 08, 09.
