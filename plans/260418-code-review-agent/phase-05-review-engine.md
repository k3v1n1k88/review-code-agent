# Phase 05 — Review Engine (Diff Analyzer, Context Resolver, AI Call)

## Context Links
- Parent: [plan.md](plan.md)
- Research: `researcher-02-ai-feedback-report.md` §1 (Claude best practices), §3 (ReAct), `researcher-01-backend-report.md` §5 (diff fetch)
- Depends on: Phase 01, 02

## Overview
- **Priority**: P1
- **Status**: pending
- **Effort**: 14h
- **Group**: B (parallel with 04, 06)
- **Description**: Core intelligence. Parses unified diffs, scores size/complexity, resolves context (external Code Graph service with file-fetch fallback), calls configurable AI provider (Claude via `anthropic-sdk-go`) with prompt caching, parses JSON response into `review_issues`. Exposes `ReviewUsecase.Run(ctx, ReviewRequest) (ReviewResult, error)`.

## Key Insights
- **Provider abstraction** (research §1.4): `Provider` interface with impls for `anthropic`, `openai`, `google`. LiteLLM-style router.
- **Prompt caching** (research §1.5): cache team system prompt + codebase guidelines → 90% cost savings on repeated reviews.
- **Context resolution**: first call Code Graph `/resolve` endpoint with file paths → fallback to `git fetch --depth=1` + `git show` on remote if 404. Limit to 500 lines/file per research §3.3.
- **Structured output** (research §3.4): enforce JSON schema; reject + retry once on parse failure.
- **Model per team** (from design): query `model_configs` by `team_id`, pick `is_default=true`.
- **NOT building a tool-use agent in v1** — YAGNI. Structured output with pre-resolved context is simpler and ships. Tool-use agent deferred to Phase 10 or v2.
- **Top-N file selector** (validation decision): if diff score > 2000, rank files by `additions+deletions` descending, pick top N (config default 10). Warn caller via `result.Truncated=true`.
- **Code Graph = GitNexus**: client wraps GitNexus API (REST assumed). Confirm endpoint spec with GitNexus team before implementation.
- **3 providers from v1** (validation decision): Implement Anthropic, OpenAI, and Google Gemini. Interface already abstracted — add `openai.go` + `gemini.go` alongside `anthropic.go`.
<!-- Updated: Validation Session 1 - top-N file selector, GitNexus, 3 providers -->

## Requirements

### Functional
- `ReviewRequest{TeamID, Source, DiffContent, RepoFullName, PRNumber, HeadSHA, BaseSHA, FileList}`.
- Parse unified diff into `[]FileChange{Path, Hunks, Additions, Deletions}`.
- Score complexity: `complexity = lines_changed + 5*files_changed`. Flag `high` if > 500.
- Resolve context: fetch surrounding code for each hunk (50 lines above/below).
- Generate embedding of diff summary for RAG lookup (Phase 10 consumes).
- Call AI provider with system (cached) + user (diff+context) messages.
- Parse JSON response → persist review + issues.
- Return `ReviewResult{ReviewID, Summary, Issues[], OverallApproved, ModelUsed, Usage}`.

### Non-functional
- Single review p95 < 30s (excluding queue wait).
- Prompt cache hit ratio > 60% after 10 reviews on same team.
- No-context fallback works if Code Graph down.

## Architecture

```
internal/
├── usecase/review/
│   ├── usecase.go               # Run(ctx, req) (result, err)
│   ├── diff_parser.go           # parse unified diff
│   ├── complexity.go            # scoring
│   ├── context_resolver.go      # codegraph -> git fallback
│   ├── prompt_builder.go        # build system + user
│   ├── result_parser.go         # AI JSON -> domain
│   └── persist.go               # write reviews + issues
├── infrastructure/
│   ├── ai/
│   │   ├── provider.go          # Provider interface
│   │   ├── anthropic.go         # Claude impl (official sdk)
│   │   ├── openai.go            # stub for Phase 14
│   │   └── router.go            # pick impl from model_configs
│   ├── codegraph/
│   │   ├── client.go            # HTTP client
│   │   └── types.go
│   └── git/
│       ├── fetcher.go           # `git archive` / GitHub raw URL fallback
│       └── types.go
```

## Related Code Files

### Create
- `/internal/usecase/review/{usecase,diff_parser,complexity,context_resolver,prompt_builder,result_parser,persist}.go`
- `/internal/infrastructure/ai/{provider,anthropic,openai,router}.go`
- `/internal/infrastructure/codegraph/{client,types}.go`
- `/internal/infrastructure/git/{fetcher,types}.go`
- `/internal/repository/postgres/review_repo.go` (writes `reviews` + `review_issues`)
- `/internal/repository/postgres/model_config_repo.go`

### Modify
- `/internal/config/config.go` — add `CodeGraph`, `AI` provider map.
- `/internal/domain/review.go` — expand `Review`, `Issue`, `Diff` types.

## Implementation Steps

1. **Diff parser** (`diff_parser.go`, ~150 lines) — hand-written for unified-diff format. Handles `--- a/`, `+++ b/`, `@@ -a,b +c,d @@`, collects hunks.

2. **Complexity scoring**:
   ```go
   type ComplexityLabel string
   const (Low="low"; Medium="medium"; High="high"; TooLarge="too_large")
   func Score(fc []FileChange) (int, ComplexityLabel) {
       lines := 0; for _, f := range fc { lines += f.Additions + f.Deletions }
       score := lines + 5*len(fc)
       switch { case score > 2000: return score, TooLarge; case score > 500: return score, High; case score > 100: return score, Medium; default: return score, Low }
   }
   ```

3. **Context resolver** (`context_resolver.go`):
   ```go
   type ContextResolver interface { Resolve(ctx context.Context, req ContextReq) (map[string]string, error) }
   type compositeResolver struct { cg *codegraph.Client; git *git.Fetcher }
   func (c *compositeResolver) Resolve(ctx, req) (map[string]string, error) {
       out, err := c.cg.ResolveFiles(ctx, req.Repo, req.SHA, req.Paths, 50)
       if err == nil && len(out) > 0 { return out, nil }
       return c.git.FetchRanges(ctx, req.Repo, req.SHA, req.Paths, 50)
   }
   ```

4. **Provider interface**:
   ```go
   type Provider interface {
       Review(ctx context.Context, in ProviderInput) (ProviderOutput, error)
       Name() string
   }
   type ProviderInput struct {
       System []Block            // Block.CacheControl = "ephemeral" for stable prefix
       UserMessage string
       MaxTokens int
       Temperature float32
   }
   type ProviderOutput struct {
       Text string
       MessageID string
       Usage TokenUsage           // input, output, cache_creation, cache_read
       Model string
   }
   ```

5. **Anthropic impl** (`ai/anthropic.go`) using official `anthropic-sdk-go`:
   ```go
   import anthropic "github.com/anthropics/anthropic-sdk-go"
   func (p *Anthropic) Review(ctx, in) (out, err) {
       msg, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
           Model: p.model,
           MaxTokens: in.MaxTokens,
           Temperature: in.Temperature,
           System: convertSystem(in.System),      // cache_control=ephemeral on stable blocks
           Messages: []anthropic.MessageParam{{ Role: "user", Content: in.UserMessage }},
       })
       return ProviderOutput{
           Text: msg.Content[0].Text,
           MessageID: msg.ID,
           Usage: TokenUsage{
               Input: msg.Usage.InputTokens,
               Output: msg.Usage.OutputTokens,
               CacheCreation: msg.Usage.CacheCreationInputTokens,
               CacheRead: msg.Usage.CacheReadInputTokens,
           },
           Model: msg.Model,
       }, nil
   }
   ```

6. **Router** (`ai/router.go`):
   ```go
   type Router struct { providers map[string]Provider; cfgRepo domain.ModelConfigRepo }
   func (r *Router) For(ctx, teamID) (Provider, ModelConfig, error) {
       cfg, _ := r.cfgRepo.GetDefault(ctx, teamID)
       p, ok := r.providers[cfg.Provider]
       if !ok { return nil, cfg, fmt.Errorf("provider %q not configured", cfg.Provider) }
       return p, cfg, nil
   }
   ```

7. **Prompt builder** (`prompt_builder.go`):
   ```go
   const systemBase = `You are a senior code reviewer.
   OUTPUT ONLY JSON matching: {"summary": "...", "overall_approved": bool, "issues": [{file, line, severity, type, message, suggestion, confidence}]}
   Severity: low|medium|high|critical. Type: security|performance|style|test|bug.
   Cite exact file:line. If unsure, set confidence=low.`

   func Build(teamCfg ModelConfig, diff string, ctxFiles map[string]string, ragSamples []string) ProviderInput {
       sys := []Block{
           {Text: systemBase, Cache: "ephemeral"},                         // stable
           {Text: teamCfg.SystemPrompt, Cache: "ephemeral"},                // per-team stable
           {Text: formatRAG(ragSamples), Cache: ""},                        // volatile
       }
       user := formatUser(diff, ctxFiles)
       return ProviderInput{ System: sys, UserMessage: user, MaxTokens: teamCfg.MaxTokens, Temperature: teamCfg.Temperature }
   }
   ```
   Keep static prefix stable → cache hit.

8. **Result parser** (`result_parser.go`):
   ```go
   type aiResult struct { Summary string; OverallApproved bool `json:"overall_approved"`; Issues []aiIssue }
   func Parse(text string) (*aiResult, error) {
       text = extractJSONBlock(text)                 // strip ```json fences
       var r aiResult; if err := json.Unmarshal([]byte(text), &r); err != nil { return nil, err }
       return &r, nil
   }
   ```
   On parse fail: one retry with stricter instruction appended. If still fails: persist `status=failed`.

9. **Usecase** (`usecase.go`, the conductor, <200 lines):
   ```go
   func (u *Usecase) Run(ctx context.Context, req Request) (*Result, error) {
       diff := parseDiff(req.DiffContent)
       score, label := Score(diff)
       if label == TooLarge { return nil, ErrDiffTooLarge }
       ctxFiles, _ := u.resolver.Resolve(ctx, buildCtxReq(req, diff))
       ragSamples := u.feedback.SimilarForTeam(ctx, req.TeamID, diff, 5) // Phase 10 plugs in
       provider, modelCfg, _ := u.router.For(ctx, req.TeamID)
       in := prompt.Build(modelCfg, req.DiffContent, ctxFiles, ragSamples)
       start := time.Now()
       out, err := provider.Review(ctx, in)
       latency := time.Since(start)
       if err != nil { return u.persistFailed(ctx, req, err) }
       parsed, err := result.Parse(out.Text)
       if err != nil { /* retry once then persistFailed */ }
       review := domain.Review{ ... }
       issues := mapIssues(parsed.Issues, review.ID)
       if err := u.repo.Persist(ctx, &review, issues); err != nil { return nil, err }
       u.usage.Record(ctx, req.TeamID, review.ID, out.MessageID, out.Model, out.Usage, latency) // Phase 12
       return &Result{ Review: review, Issues: issues, Usage: out.Usage }, nil
   }
   ```

10. **Persist** (`persist.go`) wraps repo tx: one `INSERT reviews`, bulk `INSERT review_issues`.

11. **Model config repo**:
    ```go
    func (r *ModelConfigRepo) GetDefault(ctx, teamID) (ModelConfig, error) {
        // SELECT * FROM model_configs WHERE team_id=$1 AND is_default=TRUE
    }
    ```

## Todo List

- [ ] `diff_parser_test.go` passes on GitHub-style unified diff samples
- [ ] `complexity_test.go` labels Low/Medium/High/TooLarge correctly
- [ ] `codegraph.Client` tested with httptest server (200, 404 paths)
- [ ] `git.Fetcher` tested via local bare repo fixture
- [ ] Provider interface has Anthropic impl; unit test with recorded HTTP cassette
- [ ] Router picks provider by team config
- [ ] Prompt builder emits stable prefix (golden-file test)
- [ ] Result parser handles fenced + raw JSON
- [ ] Usecase integration test: in-memory repos + fake provider → assert review + issues written
- [ ] Cache header verified present in Anthropic request

## Success Criteria
- `go test ./internal/usecase/review/... ./internal/infrastructure/ai/... -cover` coverage ≥ 70%.
- Manual run against real Anthropic sandbox with 50-line diff returns JSON result with ≥1 issue.
- `review_issues` populated correctly (inspect via `psql`).

## Risk Assessment
- **AI hallucinated file paths**: result_parser checks `issue.File` is in `diff.Files`; drops or flags mismatch.
- **Prompt drift after team guideline edit**: cache invalidates automatically on prefix change — intentional.
- **Code Graph outage**: fallback resolver; if both fail, run with diff-only context (reduced quality, warned).
- **Anthropic rate limits**: respect 429; exponential backoff up to 3 tries.

## Security Considerations
- Never send secrets to AI: pipe through `guardrail.Scrub` (Phase 04) before prompt build — scrub replaces detected secrets with `<REDACTED>`.
- Log AI prompt hash, never full content.
- Don't persist full diff; only `diff_hash` (Phase 02 schema).

## Next Steps
- **Unblocks**: Phase 07 (webhook calls `Run` async), Phase 08 (REST/MCP call `Run`), Phase 10 (feedback + RAG plugs into `SimilarForTeam`), Phase 12 (usage recorder plugs in).
- **Parallel**: Phase 04, 06.
