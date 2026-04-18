---
description: Weekly Level 1 metrics summary from local event log.
---

The user invoked `/sensei-review`. Read `.claude/sensei/usage.jsonl` and produce a summary.

### Read logic

1. Open `.claude/sensei/usage.jsonl`. Each line is a JSON event.
2. If file missing → respond: "No usage data logged yet. Run a session with Sensei active, then check back." Exit.
3. If file present but empty → same message.
4. Parse tolerantly — skip malformed lines silently. Do NOT abort on parse error.

### Aggregation

Compute, over all events in the file:
- **Session count**: distinct `sessionId` values.
- **`/emergency-mode` rate**: count of `emergency_invoked` / session count.
- **`/reveal` rate**: count of `reveal_invoked` / session count.
- **`/struggle` rate**: count of `struggle_invoked` / session count.
- **Top emergency reasons**: cluster similar reason strings (case-insensitive substring match is enough), report top 3 with counts.
- **Trend**: if data spans >7 days, split into last-7 vs prior-7. Report rate direction (↑ / ↓ / flat) per metric.

### Output format

```
Sensei Review — <period covered>

Sessions:           <N>
Emergency rate:     <X>% of sessions  (trend: <dir>)
Reveal rate:        <X>% of sessions  (trend: <dir>)
Struggle rate:      <X>% of sessions  (trend: <dir>)

Top emergency reasons:
  1. <reason cluster> — <count>
  2. ...

Reflection:
  <ONE open-ended question to the user>
```

### Reflection question rules

- Exactly ONE question. No bullet list of reflections.
- The question must be user-specific (reference a pattern from their data), not generic.
- Do NOT autotag sessions as "good" or "bad" — user interprets their own data.
- Examples:
  - If emergency rate is high: "Looking at the emergency reasons, is the scope of your current problem too ambitious, or is the friction poorly calibrated?"
  - If reveal rate is high: "High reveal count this week — were these genuine stuck-points or signals that the hints weren't specific enough?"
  - If struggle rate is high but reveal rate low: "You hit struggle a lot but rarely revealed — that's unusual. How did those sessions end?"

### Red lines

- Never share this data externally. All processing local.
- Never infer "the user is struggling / thriving" — show numbers, ask the question.
- Never recommend `/emergency-mode` as a solution to high emergency rates — that reads as sarcasm.
