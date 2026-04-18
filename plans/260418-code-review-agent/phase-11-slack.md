# Phase 11 — Slack Notifications

## Context Links
- Parent: [plan.md](plan.md)
- Research: `researcher-02-ai-feedback-report.md` §4 (Slack webhook + API patterns)
- Depends on: Phase 07 (webhook completion), Phase 09 (queue delivery)

## Overview
- **Priority**: P2
- **Status**: pending
- **Effort**: 6h
- **Group**: D (parallel with 10, 12)
- **Description**: Post review results to Slack per team's configured channel(s). Incoming webhook for simple notifications; Slack Web API with Bot token for interactive buttons (approve/request changes). Minimal by design — YAGNI on threaded conversations.

## Key Insights
- Research §4.1: start with webhook (simplest); graduate to Web API for interactive elements.
- Team-level config table `slack_channels` (Phase 02) already has `webhook_url` + `channel_id` + `notify_on` events.
- Block Kit formatting for rich messages (summary + severity-tagged issues).
- Interactive callbacks via `/slack/interactions` endpoint → verify Slack signature.

## Requirements

### Functional
- Notifier called by queue consumer after `review.Run` completes (success or fail).
- Render Block Kit payload with: header, summary, severity-counted badges, top-3 critical issues, link to dashboard.
- Skip notification if team has no `slack_channels` row or event not in `notify_on`.
- Interactive endpoint: `POST /slack/interactions` handles `approve_review` / `request_changes` button clicks → writes feedback.
- Dashboard page under `/teams/[id]` to manage Slack channels (webhook URL OR channel_id + bot token).

### Non-functional
- Notification latency < 2s after review completion.
- Retry 3× with exp backoff on 5xx from Slack.
- Rate limit honors Slack Tier 2 (20/min per workspace) via global token bucket.

## Architecture

```
internal/
├── infrastructure/slack/
│   ├── client.go               # webhook + Web API clients
│   ├── blocks.go               # Block Kit builders
│   ├── signature.go            # verify Slack signing secret
│   └── types.go
└── usecase/notify/
    ├── review_notify.go        # fn(ReviewResult, TeamID) -> dispatch
    └── interactions.go         # button click handler
```

## Related Code Files

### Create
- `/internal/infrastructure/slack/{client,blocks,signature,types}.go`
- `/internal/usecase/notify/{review_notify,interactions}.go`
- `/internal/delivery/http/handlers/slack_interactions_handler.go`
- `/web/components/teams/slack-config.tsx` — form for webhook URL, bot token, channel, notify_on

### Modify
- `/internal/usecase/worker/handler.go` — call notifier after successful review
- `/internal/delivery/http/routes.go` — add `POST /slack/interactions` (public, verified via signature)
- `/internal/repository/postgres/` — add `slack_channel_repo.go` (if not created in Phase 06; Phase 06 owns team CRUD but not slack subresource — create here)
- `/web/app/(protected)/teams/[id]/page.tsx` — add Slack tab
- `/internal/config/config.go` — `Slack{SigningSecret, BotToken, DefaultChannel}`

## Implementation Steps

1. **Client** (`slack/client.go`, <150 lines):
   ```go
   type Client struct { httpc *http.Client; botToken string; log *slog.Logger }
   func (c *Client) PostWebhook(ctx, webhookURL string, blocks []Block) error {
       body, _ := json.Marshal(map[string]any{"blocks": blocks, "text": "Code review update"})
       return c.postWithRetry(ctx, webhookURL, "", body)
   }
   func (c *Client) PostViaAPI(ctx, channel string, blocks []Block) error {
       body, _ := json.Marshal(map[string]any{"channel": channel, "blocks": blocks})
       return c.postWithRetry(ctx, "https://slack.com/api/chat.postMessage", c.botToken, body)
   }
   func (c *Client) postWithRetry(ctx, url, bearer string, body []byte) error {
       for attempt := 0; attempt < 3; attempt++ {
           req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
           req.Header.Set("Content-Type", "application/json")
           if bearer != "" { req.Header.Set("Authorization", "Bearer "+bearer) }
           resp, err := c.httpc.Do(req)
           if err != nil { time.Sleep(backoff(attempt)); continue }
           if resp.StatusCode < 300 { resp.Body.Close(); return nil }
           if resp.StatusCode < 500 { b,_ := io.ReadAll(resp.Body); resp.Body.Close(); return fmt.Errorf("slack %d: %s", resp.StatusCode, b) }
           resp.Body.Close(); time.Sleep(backoff(attempt))
       }
       return errors.New("slack retry exhausted")
   }
   ```

2. **Block Kit builder** (`slack/blocks.go`):
   ```go
   func ReviewSummaryBlocks(r ReviewResult, dashboardURL string) []Block {
       emoji := map[domain.Severity]string{"critical":":rotating_light:","high":":red_circle:","medium":":large_yellow_circle:","low":":large_blue_circle:"}
       sevCount := countBySeverity(r.Issues)
       blocks := []Block{
           {Type: "header", Text: &Text{Type: "plain_text", Text: fmt.Sprintf("Code review: %s#%d", r.Repo, r.PRNumber)}},
           {Type: "section", Text: &Text{Type: "mrkdwn", Text: r.Summary}},
           {Type: "context", Elements: []Element{
               {Type: "mrkdwn", Text: fmt.Sprintf("critical %d | high %d | medium %d | low %d", sevCount["critical"], ...)},
           }},
       }
       for _, iss := range topCritical(r.Issues, 3) {
           blocks = append(blocks, Block{Type: "section", Text: &Text{Type: "mrkdwn", Text: fmt.Sprintf("%s *%s* `%s:%d`\n%s", emoji[iss.Severity], iss.Type, iss.FilePath, iss.LineNumber, iss.Message)}})
       }
       blocks = append(blocks, Block{Type: "actions", Elements: []Element{
           {Type: "button", Text: &Text{Type:"plain_text",Text:"View in Dashboard"}, URL: dashboardURL, ActionID:"view_dashboard"},
           {Type: "button", Text: &Text{Type:"plain_text",Text:"Approve"}, Value: r.ReviewID.String(), ActionID:"approve_review", Style:"primary"},
           {Type: "button", Text: &Text{Type:"plain_text",Text:"Request changes"}, Value: r.ReviewID.String(), ActionID:"request_changes", Style:"danger"},
       }})
       return blocks
   }
   ```

3. **Signature verification** (`slack/signature.go`):
   ```go
   // Slack signing: https://api.slack.com/authentication/verifying-requests-from-slack
   func Verify(signingSecret, timestamp, body, signature string) bool {
       if math.Abs(float64(time.Now().Unix()-parseInt(timestamp))) > 300 { return false }
       base := "v0:" + timestamp + ":" + body
       mac := hmac.New(sha256.New, []byte(signingSecret))
       mac.Write([]byte(base))
       exp := "v0=" + hex.EncodeToString(mac.Sum(nil))
       return hmac.Equal([]byte(exp), []byte(signature))
   }
   ```

4. **Notifier usecase**:
   ```go
   type Notifier struct { client *slack.Client; repo domain.SlackChannelRepo; log *slog.Logger; dashURL string }
   func (n *Notifier) NotifyReview(ctx context.Context, r review.Result) error {
       channels, err := n.repo.ListForTeam(ctx, r.TeamID); if err != nil { return err }
       for _, ch := range channels {
           if !sliceContains(ch.NotifyOn, r.Status) { continue }
           blocks := slack.ReviewSummaryBlocks(r, fmt.Sprintf("%s/reviews/%s", n.dashURL, r.ReviewID))
           if ch.WebhookURL != "" {
               _ = n.client.PostWebhook(ctx, ch.WebhookURL, blocks)
           } else if ch.ChannelID != "" {
               _ = n.client.PostViaAPI(ctx, ch.ChannelID, blocks)
           }
       }
       return nil
   }
   ```

5. **Interactions handler**:
   ```go
   func (h *Handler) Handle(c echo.Context) error {
       body, _ := io.ReadAll(c.Request().Body)
       ts := c.Request().Header.Get("X-Slack-Request-Timestamp")
       sig := c.Request().Header.Get("X-Slack-Signature")
       if !slack.Verify(h.secret, ts, string(body), sig) { return echo.ErrUnauthorized }
       payloadJSON := parseFormPayload(body)
       action := payloadJSON.Actions[0]
       reviewID := uuid.MustParse(action.Value)
       switch action.ActionID {
       case "approve_review":  _ = h.feedback.Create(ctx, CreateIn{ReviewID: reviewID, Rating: 5, Type: "positive"})
       case "request_changes": _ = h.feedback.Create(ctx, CreateIn{ReviewID: reviewID, Rating: 2, Type: "correction"})
       }
       return c.JSON(200, map[string]string{"text": "Recorded, thanks!"})
   }
   ```

6. **Wire worker** (modify Phase 09 handler):
   ```go
   // After review.Run success:
   _ = h.Notifier.NotifyReview(ctx, res)
   ```
   Use goroutine with timeout to avoid blocking ack.

7. **Frontend Slack config tab**:
   - Form fields: webhook URL OR channel_id + bot token (choose one), notify_on multi-select (`queued|completed|failed`).
   - Test button: calls `POST /api/v1/teams/:id/slack-channels/:cid/test` → dispatches dummy payload.

8. **Routes**:
   ```go
   e.POST("/slack/interactions", h.SlackInteractions.Handle)
   auth.POST("/teams/:id/slack-channels", h.SlackConfig.Create)
   auth.DELETE("/teams/:id/slack-channels/:cid", h.SlackConfig.Delete)
   ```

## Todo List

- [ ] Slack client tested against Slack's webhook URL (staging)
- [ ] Block Kit renders correctly for 0, 1, many issues (Block Kit Builder verification)
- [ ] Signature verification unit-test with fixture
- [ ] Notifier respects `notify_on` filter
- [ ] Interactions endpoint writes feedback row on button click
- [ ] Retry with exp backoff confirmed via fault-injection test
- [ ] Frontend Slack config CRUD + Test button works
- [ ] No Slack calls when team has no channels

## Success Criteria
- PR merge via webhook → Slack message appears in configured channel within 2s of review complete.
- Click "Approve" in Slack → `review_feedback` row inserted with rating 5.
- Dashboard Slack settings page allows CRUD of channels.

## Risk Assessment
- **Slack rate limits**: Tier 2 = 20/min/workspace. Protect via global limiter; log 429 for operator visibility.
- **Bot token compromise**: store in `team_integrations`-style encrypted column (Phase 14 encryption); mask in logs.
- **Interactive signing secret rotation**: requires redeploy or config reload — document in runbook.

## Security Considerations
- Constant-time signature compare.
- Bot tokens encrypted at rest.
- Signing secret from env only; never stored per-team unless necessary.
- Truncate issue messages in Block Kit to prevent leaked secret echo (already scrubbed in Phase 04, but belt+suspenders).

## Next Steps
- **Unblocks**: Nothing on critical path. Phase 13 tests include Slack stub assertion.
- **Parallel**: Phase 10, 12.
