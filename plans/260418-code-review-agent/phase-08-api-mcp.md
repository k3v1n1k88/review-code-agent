# Phase 08 — REST API Endpoints & MCP Server (Streaming)

## Context Links
- Parent: [plan.md](plan.md)
- Research: `researcher-01-backend-report.md` §3 (MCP SDK), `researcher-02-ai-feedback-report.md` §1.3 (streaming)
- Depends on: Phase 04 (middleware), Phase 05 (review engine)

## Overview
- **Priority**: P1
- **Status**: pending
- **Effort**: 10h
- **Group**: C (parallel with 07, 09)
- **Description**: Two additional entry channels. REST API for programmatic review (`POST /api/v1/reviews` with raw diff, JSON response). MCP server exposing `review_code` and `review_diff` tools with streaming support via SSE transport.

## Key Insights
- **REST**: sync (no queue) with server-side timeout OR async-handoff. v1 = sync mode (simplest; YAGNI). Queue mode for REST deferred.
- **MCP transport**: SSE over HTTP only (validated decision). stdio removed — internal SaaS serves remote clients via HTTP. Run MCP server on a separate port (e.g. `:8081`).
<!-- Updated: Validation Session 1 - SSE-only transport -->
- **Streaming**: Anthropic `client.messages.stream` yields deltas; MCP server forwards as `progress` notifications. REST endpoint returns final JSON; streaming variant `POST /api/v1/reviews/stream` returns SSE stream.

## Requirements

### Functional

REST:
- `POST /api/v1/reviews` — body `{source:"api", diff, repo_full_name?, head_sha?, base_sha?}` → 200 `{review_id, summary, overall_approved, issues[], usage}`.
- `POST /api/v1/reviews/stream` — same input, returns `text/event-stream` with events: `progress` (chunks), `final` (full JSON), `error`.
- `GET /api/v1/reviews/:id` — reuse Phase 06.
- API-key auth; per-key rate limit.

MCP:
- Binary `cmd/mcp/main.go` with two modes: `--transport stdio` (default) and `--transport sse --addr :8090`.
- Tools:
  - `review_code(code, language, file_path)` — one-file review.
  - `review_diff(diff, repo?, head_sha?)` — full diff review.
- Streaming via `jsonrpc` progress notifications.
- MCP server authenticates via env `CRA_API_KEY` (SSE mode also accepts `?api_key=`).

### Non-functional
- REST sync mode: timeout 90s (review engine p95 is 30s — 3x margin).
- SSE keep-alive every 15s with comment ping.
- MCP startup < 500ms.

## Architecture

```
internal/
├── delivery/
│   ├── http/
│   │   ├── api/
│   │   │   ├── review_api.go           # POST /api/v1/reviews
│   │   │   └── types.go
│   │   └── stream/
│   │       ├── sse.go                  # generic SSE helpers
│   │       └── review_stream.go        # POST /api/v1/reviews/stream
│   └── mcp/
│       ├── server.go                   # NewServer() wiring
│       ├── tools.go                    # tool definitions + handlers
│       └── transport.go                # stdio vs sse selector
cmd/
└── mcp/main.go                         # entrypoint (small)
```

## Related Code Files

### Create
- `/internal/delivery/http/api/{review_api,types}.go`
- `/internal/delivery/http/stream/{sse,review_stream}.go`
- `/internal/delivery/mcp/{server,tools,transport}.go`
- `/cmd/mcp/main.go` (fleshed out from Phase 01 stub)

### Modify
- `/internal/infrastructure/ai/provider.go` — add `ReviewStream(ctx, in, sink chan<- StreamDelta) error` method (optional; providers can choose to implement).
- `/internal/infrastructure/ai/anthropic.go` — implement `ReviewStream`.
- `/internal/usecase/review/usecase.go` — add `RunStream(ctx, req, sink)` parallel method.
- `/internal/delivery/http/routes.go` — register new routes.

## Implementation Steps

### REST sync endpoint
1. **Request DTO + validation** (`api/types.go`):
   ```go
   type ReviewCreateReq struct {
       Diff string `json:"diff"`
       RepoFullName string `json:"repo_full_name,omitempty"`
       HeadSHA string `json:"head_sha,omitempty"`
       BaseSHA string `json:"base_sha,omitempty"`
   }
   func (r ReviewCreateReq) Validate() error {
       return validation.ValidateStruct(&r,
           validation.Field(&r.Diff, validation.Required, validation.Length(1, 1_000_000)))
   }
   ```

2. **Handler**:
   ```go
   func (h *ReviewAPI) Create(c echo.Context) error {
       var req ReviewCreateReq; if err := c.Bind(&req); err != nil { return echo.ErrBadRequest }
       if err := req.Validate(); err != nil { return echo.NewHTTPError(400, err.Error()) }
       p := c.Get("principal").(*domain.Principal)
       ctx, cancel := context.WithTimeout(c.Request().Context(), 90*time.Second)
       defer cancel()
       res, err := h.uc.Run(ctx, review.Request{
           TeamID: p.TeamID, Source: "api", DiffContent: req.Diff,
           RepoFullName: req.RepoFullName, HeadSHA: req.HeadSHA, BaseSHA: req.BaseSHA,
       })
       if err != nil { return mapReviewError(err) }
       return c.JSON(200, toDTO(res))
   }
   ```

### SSE streaming
3. **SSE helper** (`stream/sse.go`, < 80 lines):
   ```go
   type SSEWriter struct{ w http.ResponseWriter; f http.Flusher }
   func NewSSE(c echo.Context) (*SSEWriter, error) {
       w := c.Response().Writer
       f, ok := w.(http.Flusher); if !ok { return nil, errors.New("streaming unsupported") }
       w.Header().Set("Content-Type", "text/event-stream")
       w.Header().Set("Cache-Control", "no-cache")
       w.Header().Set("Connection", "keep-alive")
       return &SSEWriter{w, f}, nil
   }
   func (s *SSEWriter) Event(event string, data any) error {
       b, _ := json.Marshal(data)
       if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, b); err != nil { return err }
       s.f.Flush(); return nil
   }
   func (s *SSEWriter) Ping() { fmt.Fprintf(s.w, ": ping\n\n"); s.f.Flush() }
   ```

4. **Stream handler**:
   ```go
   func (h *ReviewStream) Create(c echo.Context) error {
       // validate like sync path
       sse, _ := NewSSE(c)
       sink := make(chan review.StreamDelta, 16)
       resCh := make(chan *review.Result, 1); errCh := make(chan error, 1)
       go func() { r, err := h.uc.RunStream(ctx, reqObj, sink); close(sink); if err != nil { errCh <- err } else { resCh <- r } }()
       ticker := time.NewTicker(15 * time.Second); defer ticker.Stop()
       for {
           select {
           case d, ok := <-sink:
               if !ok { continue }
               sse.Event("progress", d)
           case <-ticker.C:
               sse.Ping()
           case r := <-resCh:
               sse.Event("final", toDTO(r)); return nil
           case err := <-errCh:
               sse.Event("error", map[string]string{"message": err.Error()}); return nil
           case <-c.Request().Context().Done():
               return nil
           }
       }
   }
   ```

5. **Provider stream** (`ai/anthropic.go` addition):
   ```go
   func (p *Anthropic) ReviewStream(ctx, in, sink chan<- StreamDelta) error {
       stream := p.client.Messages.NewStreaming(ctx, paramsFrom(in))
       for stream.Next() {
           ev := stream.Current()
           if delta, ok := ev.(anthropic.ContentBlockDeltaEvent); ok {
               sink <- StreamDelta{Text: delta.Delta.Text}
           }
       }
       return stream.Err()
   }
   ```

6. **Usecase `RunStream`**: identical to `Run` until provider call; pipes streamed text through sink, buffers full text for parse at end.

### MCP server
7. **`cmd/mcp/main.go`** (< 60 lines):
   ```go
   func main() {
       cfg := config.Load()
       transport := flag.String("transport", "stdio", "stdio|sse"); addr := flag.String("addr", ":8090", "")
       flag.Parse()
       deps := bootstrap.NewMCPDeps(cfg)                 // review usecase + provider
       srv := mcpdel.NewServer(deps)
       switch *transport {
       case "stdio": srv.RunStdio(context.Background())
       case "sse":   srv.RunSSE(context.Background(), *addr, cfg.Auth.APIKeyPepper)
       }
   }
   ```

8. **`mcp/server.go`** using official `modelcontextprotocol/go-sdk`:
   ```go
   func NewServer(d *Deps) *Server {
       s := mcp.NewServer("code-review-agent", "1.0")
       tools.Register(s, d)
       return &Server{mcp: s}
   }
   func (s *Server) RunStdio(ctx) error { return s.mcp.Run(ctx, mcp.StdioTransport()) }
   func (s *Server) RunSSE(ctx, addr, _pepper) error { return s.mcp.Run(ctx, mcp.SSETransport(addr)) }
   ```

9. **`mcp/tools.go`**:
   ```go
   type ReviewDiffIn struct { Diff, Repo, HeadSHA string }
   type ReviewDiffOut struct { Summary string; Approved bool; Issues []Issue }
   func Register(s *mcp.Server, d *Deps) {
       s.AddTool("review_diff", "Review a unified diff and return issues", func(ctx context.Context, in ReviewDiffIn, progress mcp.ProgressEmitter) (*ReviewDiffOut, error) {
           sink := make(chan review.StreamDelta, 16)
           go func() { for x := range sink { progress.Emit(mcp.ProgressUpdate{Message: x.Text}) } }()
           r, err := d.ReviewUC.RunStream(ctx, review.Request{TeamID: d.TeamID, Source:"mcp", DiffContent: in.Diff, ...}, sink)
           if err != nil { return nil, err }
           close(sink)
           return toMCPOut(r), nil
       })
       s.AddTool("review_code", "Review a single file's code", func(ctx, in ReviewCodeIn) (*ReviewCodeOut, error) { /* wrap as synthetic diff */ })
   }
   ```

10. **Routes**:
    ```go
    api := e.Group("/api/v1", mw.APIKey, mw.RateLimitByKey)
    api.POST("/reviews", h.ReviewAPI.Create)
    api.POST("/reviews/stream", h.ReviewStream.Create)
    api.GET("/reviews/:id", h.Reviews.Get)            // Phase 06
    ```

11. **Compose**: optional `mcp` service for SSE mode (not exposed publicly by default).

## Todo List

REST:
- [ ] `POST /api/v1/reviews` sync — happy, 400 (empty diff), 401 (bad key), 429 (over limit), 504 (timeout)
- [ ] `POST /api/v1/reviews/stream` emits progress + final events
- [ ] Client reconnection tolerated (new delivery_id each call; no session state)

MCP:
- [ ] `review_diff` tool invoked via stdio returns valid JSON
- [ ] SSE transport reachable locally; auth via API key
- [ ] Progress notifications visible in Claude Code client
- [ ] Tool input JSON schemas published in list_tools response

## Success Criteria
- `curl -X POST -H "X-API-Key: ..." -d @sample.json /api/v1/reviews` returns parsed review.
- `curl -N -X POST .../reviews/stream` emits `event: progress` lines then `event: final`.
- `claude mcp add cra ./bin/mcp-server` and `review_diff` callable.
- MCP streaming delivers partial text within 2s of provider first-token.

## Risk Assessment
- **Long-running sync REST**: 90s timeout acceptable; clients should prefer `/stream`. Document.
- **SSE behind reverse proxies**: disable buffering in nginx/traefik via header hint (`X-Accel-Buffering: no`).
- **MCP SDK churn**: pin exact version; add a wrapper to isolate SDK-specific types.

## Security Considerations
- API key auth on all endpoints.
- MCP stdio mode inherits shell env — warn about leaking keys in process listings.
- No diff content logged.
- Per-key rate limit prevents client loops.

## Next Steps
- **Unblocks**: Phase 10 (feedback can be submitted via REST too), Phase 13 (integration tests hit REST).
- **Parallel**: Phase 07, 09.
