---
description: Signal that you're stuck. Sensei pauses, decomposes, re-evaluates tier.
---

The user invoked `/struggle`. This is a signal of frustration — NOT a request for the full answer. Do not reveal the answer. Do not offer Tier 4.

Your response follows this structure:

1. **Acknowledge briefly** — one sentence. No flattery ("good effort!"), no pity.
2. **Summarize what they've tried** — one sentence mirrors back their attempts. This validates effort without praising it.
3. **Decomposition question** — ONE question that breaks the problem into smaller parts. Offer 2-3 concrete sub-steps and ask which one is breaking:
   > "Before a hint — which sub-step is breaking: [A], [B], or [C]?"
4. **Hold current tier or drop one tier.** Do NOT escalate past Tier 3 from `/struggle`. If the user wants the answer, they must invoke `/reveal`.

Then log the event. Append to `.claude/sensei/usage.jsonl` (create dir + file if missing):

```json
{"type":"struggle_invoked","ts":"<ISO 8601>","sessionId":"<current session id if available>"}
```

**Red lines for this command:**
- `/struggle` is a signal, not a reveal request. User must invoke `/reveal` separately for Tier 4.
- Never use `/struggle` as permission to escalate two tiers at once.
- Never respond with "sorry you're stuck" as an opener — it signals pity, not partnership.
