# Phase 10 — Feedback Loop (Human Feedback UI + pgvector RAG)

## Context Links
- Parent: [plan.md](plan.md)
- Research: `researcher-02-ai-feedback-report.md` §2 (pgvector), §2.4 (RAG pattern)
- Depends on: Phase 05 (hooks `SimilarForTeam`), Phase 06 (feedback widget placeholder), Phase 09 (async embedding via queue)

## Overview
- **Priority**: P1
- **Status**: pending
- **Effort**: 10h
- **Group**: D (parallel with 11, 12)
- **Description**: Close the learning loop. UI for users to rate + comment on past reviews. Backend persists feedback, generates embedding via embedding provider, stores in pgvector. Review engine queries similar past feedback before each AI call (RAG) and injects as extra system context. Feedback dashboard shows rating trends and correction counts.

## Key Insights
- Embedding provider abstracted: OpenAI `text-embedding-3-small` (1536d) for cost OR Voyage-code-3 (tuned for code). Choose OpenAI for v1 (research §2.2 recommends for cost).
- Embed the **feedback semantic content** (review summary + human comment), NOT raw diff — research §2.2 explicitly.
- Similarity threshold: cosine distance < 0.3 (research §2.3).
- Async embedding via queue: submitting feedback enqueues `EmbedJob{feedback_id}`; worker generates + persists. Keeps UI snappy.
- RAG retrieval during new review: top-5 similar past `(review+feedback)` for same team → inject as "Past lessons" in system prompt with `cache_control=ephemeral`.

## Requirements

### Functional

UI:
- On review detail page: feedback widget with 1-5 star rating + optional comment + `feedback_type` dropdown (positive/negative/correction).
- `/feedback` page: team-scoped feed, filters by type, rating histogram card.

Backend:
- `POST /api/v1/reviews/:id/feedback` → `{rating, type, comment}`.
- `GET /api/v1/feedback?team_id=&type=&cursor=`.
- `GET /api/v1/feedback/stats?team_id=&from=&to=` → histogram, avg, correction_rate.
- Embedding generation: enqueue on feedback create; worker calls provider, writes `embedding`.
- `FeedbackRepo.SimilarForTeam(ctx, teamID, queryEmbedding, limit)` used by Phase 05.

### Non-functional
- Feedback POST returns in <100ms (embedding async).
- RAG lookup adds <200ms to review pipeline (single indexed query).
- Embedding cost ~$0.00002 per feedback (OpenAI small).

## Architecture

```
internal/
├── infrastructure/embedding/
│   ├── provider.go              # Embedder interface
│   ├── openai.go                # text-embedding-3-small
│   └── anthropic.go             # (optional alt)
├── usecase/feedback/
│   ├── create.go
│   ├── list.go
│   ├── stats.go
│   ├── similar.go               # RAG lookup (consumed by review UC)
│   └── embed_job.go              # worker: generate + persist embedding
├── repository/postgres/
│   └── feedback_repo.go          # CRUD + cosine search
└── delivery/http/handlers/
    └── feedback_handler.go

web/
├── app/(protected)/
│   ├── feedback/page.tsx
│   └── reviews/[id]/page.tsx     # augment with widget
└── components/feedback/
    ├── feedback-widget.tsx       # stars + type + comment
    ├── feedback-list.tsx
    ├── feedback-filters.tsx
    └── rating-histogram.tsx
```

## Related Code Files

### Create
- `/internal/infrastructure/embedding/{provider,openai,anthropic}.go`
- `/internal/usecase/feedback/{create,list,stats,similar,embed_job}.go`
- `/internal/repository/postgres/feedback_repo.go`
- `/internal/delivery/http/handlers/feedback_handler.go`
- `/web/app/(protected)/feedback/page.tsx`
- `/web/components/feedback/{feedback-widget,feedback-list,feedback-filters,rating-histogram}.tsx`
- `/web/lib/api/feedback.ts`

### Modify
- `/internal/usecase/review/usecase.go` — inject `Similar` dependency; wire `SimilarForTeam` into prompt builder.
- `/internal/usecase/review/prompt_builder.go` — accept ragSamples; render "Past lessons" block.
- `/internal/usecase/worker/handler.go` — dispatch `EmbedJob` in addition to `ReviewJob` (use routing key).
- `/internal/infrastructure/queue/topology.go` — add `embed-jobs` queue.
- `/internal/delivery/http/routes.go` — register feedback routes.
- `/web/app/(protected)/reviews/[id]/page.tsx` — embed widget under issues.

## Implementation Steps

1. **Embedding provider** (`embedding/provider.go`):
   ```go
   type Embedder interface {
       Embed(ctx context.Context, text string) ([]float32, error)
       Dimension() int
       Name() string
   }
   ```

2. **OpenAI impl** (`embedding/openai.go`):
   ```go
   func (o *OpenAI) Embed(ctx, text) ([]float32, error) {
       resp, err := o.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
           Model: "text-embedding-3-small", Input: []string{text},
       })
       if err != nil { return nil, err }
       return resp.Data[0].Embedding, nil
   }
   func (o *OpenAI) Dimension() int { return 1536 }
   ```

3. **Feedback create**:
   ```go
   func (uc *FeedbackUC) Create(ctx, in CreateIn) (*Feedback, error) {
       fb := domain.Feedback{ ReviewID: in.ReviewID, TeamID: in.TeamID, UserID: in.UserID,
           Rating: in.Rating, FeedbackType: in.Type, Comment: in.Comment }
       if err := uc.repo.Insert(ctx, &fb); err != nil { return nil, err }
       _ = uc.queue.PublishEmbed(ctx, EmbedJob{ FeedbackID: fb.ID })
       return &fb, nil
   }
   ```

4. **Embed job worker**:
   ```go
   func (h *EmbedHandler) Handle(ctx, j EmbedJob) error {
       fb, err := h.repo.Get(ctx, j.FeedbackID); if err != nil { return err }
       rev, err := h.reviewRepo.Get(ctx, fb.ReviewID); if err != nil { return err }
       text := fmt.Sprintf("Review summary: %s\nHuman feedback (%s, %d/5): %s", rev.Summary, fb.FeedbackType, fb.Rating, fb.Comment)
       vec, err := h.embedder.Embed(ctx, text); if err != nil { return err }
       return h.repo.UpdateEmbedding(ctx, fb.ID, vec)
   }
   ```

5. **Repository — similarity query** (`feedback_repo.go`):
   ```go
   func (r *FeedbackRepo) SimilarForTeam(ctx, teamID uuid.UUID, query []float32, limit int) ([]SimilarHit, error) {
       rows, err := r.pool.Query(ctx, `
           SELECT rf.id, r.summary, rf.comment, rf.rating, rf.feedback_type, (rf.embedding <=> $1) AS dist
           FROM review_feedback rf
           JOIN reviews r ON r.id = rf.review_id
           WHERE rf.team_id = $2 AND rf.embedding IS NOT NULL
             AND (rf.embedding <=> $1) < 0.3
           ORDER BY rf.embedding <=> $1
           LIMIT $3`, pgvector.NewVector(query), teamID, limit)
       // scan and return
   }
   ```

6. **RAG integration in review engine** (modify Phase 05):
   ```go
   // usecase/review/usecase.go
   // Before provider call:
   diffSummary := summarizeDiff(diff)                  // simple: top N changed files + heuristics
   qvec, _ := u.embedder.Embed(ctx, diffSummary)       // nil-tolerant; skip RAG if embedder errors
   samples, _ := u.feedback.SimilarForTeam(ctx, req.TeamID, qvec, 5)
   in := prompt.Build(modelCfg, diff, ctxFiles, toRagStrings(samples))
   ```
   Note: use `text-embedding-3-small` for BOTH feedback AND query embedding (same space).

7. **Prompt builder RAG block** (modify Phase 05):
   ```go
   // prompt_builder.go
   func formatRAG(samples []string) string {
       if len(samples) == 0 { return "" }
       b := strings.Builder{}; b.WriteString("Similar past feedback on this team's code:\n")
       for i, s := range samples { fmt.Fprintf(&b, "%d. %s\n", i+1, truncate(s, 500)) }
       return b.String()
   }
   ```
   Mark RAG block as non-cached (changes per PR).

8. **Feedback handler**:
   ```go
   func (h *FeedbackHandler) Create(c echo.Context) error {
       reviewID := uuid.MustParse(c.Param("id"))
       var req CreateReq; c.Bind(&req); req.Validate()
       p := c.Get("principal").(*domain.Principal)
       fb, err := h.uc.Create(c.Request().Context(), CreateIn{ ReviewID: reviewID, TeamID: p.TeamID, UserID: p.UserID, ...})
       if err != nil { return mapErr(err) }; return c.JSON(201, fb)
   }
   ```

9. **Stats query**:
   ```sql
   SELECT rating, COUNT(*) FROM review_feedback
   WHERE team_id = $1 AND created_at BETWEEN $2 AND $3
   GROUP BY rating ORDER BY rating;
   ```
   Plus `correction_rate = COUNT(*) FILTER (WHERE feedback_type='correction') / COUNT(*)`.

10. **Frontend widget** (`feedback-widget.tsx`, <100 lines):
    ```tsx
    "use client";
    const [rating, setRating] = useState(0);
    const [type, setType] = useState<"positive"|"negative"|"correction">("positive");
    const [comment, setComment] = useState("");
    async function submit() { await api(`/api/v1/reviews/${reviewId}/feedback`, {method:"POST", body: JSON.stringify({rating, type, comment})}); toast.success("Thanks!"); }
    ```

11. **Feedback page**:
    - Rating histogram (recharts BarChart).
    - Filterable feed.
    - Avg rating + correction rate cards.

12. **Topology**: add `embed-jobs` queue to Phase 09 `DeclareTopology`. Worker consumes both main and embed queues (routed by exchange or separate consumer).

## Todo List

Backend:
- [ ] `Embedder` interface with OpenAI impl; mock impl for tests
- [ ] `feedback_repo.Insert`, `UpdateEmbedding`, `SimilarForTeam` tested against real pg+pgvector
- [ ] `POST /reviews/:id/feedback` creates row, enqueues embed job
- [ ] Embed worker generates vector, updates row; <5s per item
- [ ] Review usecase integrates RAG; feature flag `enable_rag=true`
- [ ] RAG failure path (embedder down) degrades gracefully — review still runs without RAG samples

Frontend:
- [ ] Feedback widget on review detail
- [ ] `/feedback` page with filters, list, histogram
- [ ] Toast on success; inline error on failure

## Success Criteria
- Create 5 feedback rows for same team → similarity query returns them for a close-diff prompt.
- Avg review quality (manual spot-check) improves after 10+ feedback entries on the team — qualitative check.
- `correction_rate` metric visible on `/feedback` page.

## Risk Assessment
- **Embedding drift**: if we switch provider later, existing vectors are in old space. Store `embedding_model` per row; re-embed in background job when changed.
- **Similarity false-positives**: threshold 0.3 tuned via manual eval; expose as config.
- **pgvector scale**: ivfflat fine until ~1M rows; switch to HNSW in Phase 14 if needed.
- **Prompt injection via feedback comment**: sanitize — truncate to 500 chars, strip backticks, quote inside `<past_feedback>` XML.

## Security Considerations
- User input in feedback may contain prompt injection; wrap in XML tags in prompt.
- Store embedding model id per row for auditability.
- Feedback CRUD scoped by `team_id`; users can't see other teams' feedback.

## Next Steps
- **Unblocks**: Phase 11 (Slack surfaces feedback summary weekly), Phase 12 (feedback rows contribute to usage display).
- **Parallel**: Phase 11, 12.
