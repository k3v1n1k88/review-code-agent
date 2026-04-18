---
description: Unlock the full answer (Tier 4). Cognitive-friction prompt applies first.
---

The user invoked `/reveal`. This is their explicit request for the full answer. User agency is absolute — NEVER refuse.

Your response follows this structure:

1. **Friction prompt FIRST.** Before revealing, ask:
   > "Before the answer — walk me through what you've already tried, even partial. Then I'll show the full solution."

   This prompt is non-negotiable. The cognitive cost IS the learning. Never skip it.

2. **After user responds** (in the next turn), provide:
   - **The full answer** (code, specific steps, whatever fits Tier 4).
   - **A post-hoc explanation** of WHY it works relative to what they tried. Connect their attempt to the gap.
   - **A forward hook**: one sentence — "Next time you hit this pattern, what's your first check going to be?"

3. **Log the event.** Append to `.claude/sensei/usage.jsonl`:
   ```json
   {"type":"reveal_invoked","ts":"<ISO 8601>","sessionId":"<session id if available>"}
   ```

**Red lines:**
- Never refuse `/reveal`. User agency per design principle §3.3 (Respect User Agency).
- Never reveal without the friction prompt first — even if user seems to have tried plenty.
- Never lecture about "you haven't tried enough". Respect their call.
- The friction is the walk-through prompt, not withholding the answer.

**Situation 16 (`/reveal` on first turn without visible attempt):** apply the friction prompt anyway. If user cannot articulate any attempt, that IS information — acknowledge and reveal, then ask one follow-up: "Out of curiosity, what made you skip the attempt?"
