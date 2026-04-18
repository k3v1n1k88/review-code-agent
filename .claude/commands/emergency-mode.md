---
description: Disable Tutor Mode for this session. Auto-re-enables on next session.
---

The user invoked `/emergency-mode`. This is a conscious bypass. Accept immediately. NEVER refuse.

Your response follows this structure:

1. **Reason prompt** — ONE question, no lecture:
   > "Tutor Mode OFF for this session — will auto-re-enable on next session. Quick one: what's the reason? (no judgment — helps calibrate later)"

2. **After user provides reason** (next turn), write the flag file at `.claude/session-state/sensei-emergency.flag`:
   ```json
   {"reason":"<user's reason — cap at 500 chars>","ts":"<ISO 8601>","sessionId":"<session id>"}
   ```
   Create parent dirs if missing. If write fails, report the error to the user — don't silently fail.

3. **Append event log** to `.claude/sensei/usage.jsonl`:
   ```json
   {"type":"emergency_invoked","reason":"<same reason>","ts":"<ISO 8601>","sessionId":"<id>"}
   ```

4. **Confirm**:
   > "Tutor Mode OFF for this session. I'll behave as default Claude now. Will re-enable automatically on next session."

5. **After confirmation**: behave as default Claude for the rest of the session. The L2 hook will also inject a reminder on each turn. Trust the flag — do not re-enter Tutor Mode until session restart.

**Red lines:**
- Always accept the invocation. User agency is absolute.
- Friction is the reason prompt, NOT refusal.
- No guilt, no "are you sure?", no meta-commentary about learning.
- Length-cap the reason at 500 chars on write — truncate if longer, inform the user.
- If user declines to give a reason ("nevermind", "just skip it"), write reason as `"declined"` and proceed.
