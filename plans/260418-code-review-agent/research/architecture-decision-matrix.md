# Architecture Decision Matrix
## Code Review Agent SaaS - Key Technology Choices

---

## 1. LLM Model & Cost Optimization

```
┌─────────────────────────────────────────────┐
│ DECISION: Multi-Model Router                │
├─────────────────────────────────────────────┤
│ Simple Reviews                              │
│ → Claude Haiku 4.5 ($1/MTok input)        │
│ → Cost: ~$0.001-0.005 per review           │
│                                             │
│ Standard Reviews                            │
│ → Claude Sonnet 4.6 ($3/MTok input)        │
│ → Cost: ~$0.01-0.03 per review             │
│                                             │
│ Complex/Agentic Reviews                    │
│ → Claude Opus 4.7 ($5/MTok input)          │
│ → Cost: ~$0.05-0.15 per review             │
│                                             │
│ OPTIMIZATION: + Prompt Caching              │
│ → 90% savings on reused context             │
│ → Final cost: ~$0.001-0.015 per review      │
│ → Savings vs single Opus: 60%               │
└─────────────────────────────────────────────┘
```

---

## 2. Context Management

```
┌──────────────────────────────────────────────────┐
│ STRATEGY: Layered Context + Tool-Based Fetching │
├──────────────────────────────────────────────────┤
│                                                  │
│ Layer 1: CACHED (Ephemeral, 5-min)              │
│ ├─ Team guidelines (1-2KB) ← Cache this         │
│ ├─ Architecture overview (500B-1KB)             │
│ └─ Code style rules (500B)                      │
│ → Cost: Write 1.25x, Read 0.1x (one-time)       │
│                                                  │
│ Layer 2: PROMPT (Per Request)                   │
│ ├─ PR metadata (title, description)             │
│ └─ 1-2 related files (context)                  │
│ → Cost: Standard input rate                     │
│                                                  │
│ Layer 3: TOOLS (On-Demand)                      │
│ ├─ fetch_file(path, lines)                      │
│ ├─ search_codebase(pattern)                     │
│ ├─ read_docs(path)                              │
│ └─ get_test_coverage(file)                      │
│ → Cost: Only if agent requests them             │
│                                                  │
│ Token Overhead Reduction:                       │
│ Without tools (upfront 50K): $0.25              │
│ With tools (on-demand 10K): $0.05               │
│ Savings: 80% when agent uses tools              │
└──────────────────────────────────────────────────┘
```

---

## 3. Code Review Agentic Loop

```
INPUT: PR Diff
    ↓
[ORCHESTRATOR AGENT]
    ├─ Think: "Identify risk areas"
    │   (Security? Performance? Tests?)
    │
    ├─ Action 1: fetch_file(modified_file)
    │   ↓ Observe: "Missing input validation"
    │
    ├─ Action 2: search_codebase("validateInput")
    │   ↓ Observe: "Pattern found 3 places, not here"
    │
    ├─ Action 3: get_test_coverage("file.py")
    │   ↓ Observe: "85% coverage, but gap in new code"
    │
    └─ Final: Generate structured review
        ├─ issue[0]: file + line + severity + suggestion
        ├─ issue[1]: ...
        └─ confidence levels

OUTPUT: JSON Review + Slack Notification
```

---

## 4. Feedback Memory (pgvector + RAG)

```
┌───────────────────────────────────┐
│ PAST REVIEWS (PostgreSQL)         │
├───────────────────────────────────┤
│ ✓ diff_hash (SHA256)              │
│ ✓ claude_review (full text)       │
│ ✓ human_feedback (rating + notes) │
│ ✓ embedding (1536-dim, cosine)    │
└───────────────────────────────────┘
         ↓
    [NEW PR ARRIVES]
         ↓
    Similarity Search
    "Find top 5 semantically similar reviews"
         ↓
┌───────────────────────────────────┐
│ AUGMENTED PROMPT                  │
├───────────────────────────────────┤
│ Team guidelines (CACHED)          │
│ + 5 related past reviews (RAG)    │
│ + Current diff                    │
└───────────────────────────────────┘
         ↓
    "Generate review informed by
     past team feedback patterns"
         ↓
OUTPUT: Consistent, pattern-aware review
```

---

## 5. Slack Notification Flow

```
┌─────────────────────────┐
│ CODE REVIEW READY       │
├─────────────────────────┤
│ Webhook (Instant)       │
│ POST to /slack/reviews  │
└─────────────────────────┘
         ↓
┌─────────────────────────────────┐
│ SLACK MESSAGE (Rich Blocks)      │
├─────────────────────────────────┤
│ :red_circle: MEDIUM: Security   │
│   at src/auth.py:42             │
│   "Unvalidated user input..."    │
│                                 │
│ :large_yellow_circle: LOW: Test │
│   at tests/auth_test.py         │
│   "Add test for invalid token..." │
│                                 │
│ [Approve] [Request Changes]      │
│          (Interactive)           │
└─────────────────────────────────┘
         ↓
    User clicks "Approve"
         ↓
    Webhook → Update GitHub PR status
```

---

## 6. Token & Cost Tracking

```
AI_USAGE_LOGS
├─ review_id (FK to review_feedback)
├─ model_used (claude-opus-4-7, etc)
├─ input_tokens
├─ output_tokens
├─ cache_creation_tokens (NEW tokens cached)
├─ cache_read_tokens (REUSED from cache)
└─ cost_usd (calculated)

MATERIALIZED VIEW: daily_usage_by_team
├─ date
├─ num_reviews
├─ total_input
├─ total_output
├─ total_cache_savings_tokens
└─ total_cost_usd

EXAMPLE DAILY METRICS:
Team: "platform-eng"
Date: 2026-04-18
Num reviews: 45
Total input: 2.1M tokens
Total output: 450K tokens
Cache savings: 750K tokens → $0.75 saved
Total cost: $2.10 (vs $2.85 without caching)
Savings: 26% via caching
```

---

## 7. Technology Stack Summary

| Component | Tech | Rationale |
|-----------|------|-----------|
| **LLM** | Claude API (multi-model) | Best code understanding, 90% cache savings via prompt caching |
| **Embedding Store** | PostgreSQL + pgvector | Already have DB, <10M vector capacity, RAG-ready |
| **Agentic Framework** | Custom ReAct loop | Simple, transparent, controllable context fetching |
| **Notifications** | Slack Webhooks + API | Fast webhooks + interactive API buttons |
| **Cost Tracking** | PostgreSQL + materialized views | Single source of truth, easy aggregation |
| **Async Processing** | Batch API (optional) | 50% cost reduction for non-urgent reviews |

---

## 8. Cost Projection (100 Reviews/Day)

```
BASELINE (No optimizations):
  ├─ 100 reviews × 50K tokens avg = 5M tokens/day
  ├─ @ $5/MTok (Opus) = $25/day = $750/month
  └─ Issue: Single model, no caching

WITH MULTI-MODEL + CACHING:
  ├─ 50 @ Haiku ($0.001/review) = $0.05
  ├─ 35 @ Sonnet ($0.01/review) = $0.35
  ├─ 15 @ Opus ($0.05/review) = $0.75
  ├─ Cache savings (90% on 40% of input) = -$4
  ├─ Subtotal per day = $0.10 (after savings)
  ├─ Monthly = ~$3/month
  └─ Savings vs baseline: 99.6%

Note: Calculation assumes:
- 40% of requests hit cache (team guidelines)
- Cost scaling shows diminishing returns at enterprise scale
```

---

## Decision Validation Checklist

- [ ] Test multi-model routing latency variance (<5% acceptable)
- [ ] Measure cache hit rate on team guidelines (target >60%)
- [ ] Benchmark ReAct loop vs single-call (expect 15-20% accuracy improvement)
- [ ] Validate pgvector retrieval quality (recall on similar feedback)
- [ ] Slack webhook reliability under load (>99% delivery)
- [ ] Token counting accuracy vs actual billing (<2% variance)

---
