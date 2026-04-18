#!/usr/bin/env node
// Sensei L2 anti-drift hook.
// Fires on every user prompt. Injects <system-reminder> when:
//   - Emergency flag active → Tutor Mode OFF notice.
//   - Trigger phrase detected after turn 5 → anti-drift reinforcement.
// Must exit promptly (<500ms — Node ESM startup alone is ~80-100ms).
// No network, no shell-out.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

let input = {};
try {
  const raw = fs.readFileSync(0, 'utf8');
  input = raw ? JSON.parse(raw) : {};
} catch {
  // Malformed stdin — exit silently; do not disrupt Claude Code.
  process.exit(0);
}

// Prefer payload cwd so subagents / monorepo contexts hit the same state as
// the parent session. Fall back to process.cwd() when the field is absent.
const root = input.cwd || process.cwd();
const stateDir = path.join(root, '.claude', 'session-state');
const emergencyFlag = path.join(stateDir, 'sensei-emergency.flag');
const turnFile = path.join(stateDir, 'sensei-turn-count.json');
const triggersPath = path.join(__dirname, '_lib', 'trigger-phrases.json');

const userMessage = input.prompt || '';
const sessionId = input.session_id || 'unknown';

// 1. Emergency flag takes precedence.
if (fs.existsSync(emergencyFlag)) {
  try {
    const flag = JSON.parse(fs.readFileSync(emergencyFlag, 'utf8'));
    process.stdout.write(
      `<system-reminder>Sensei: Tutor Mode DISABLED for this session. Reason: ${flag.reason || 'unspecified'}. Behave as default Claude.</system-reminder>`,
    );
  } catch {
    process.stdout.write(
      '<system-reminder>Sensei: Tutor Mode DISABLED for this session. Behave as default Claude.</system-reminder>',
    );
  }
  process.exit(0);
}

// 2. Turn counter (per-session).
let turns = { count: 0, sessionId };
try {
  if (fs.existsSync(turnFile)) {
    turns = JSON.parse(fs.readFileSync(turnFile, 'utf8'));
    if (turns.sessionId !== sessionId) turns = { count: 0, sessionId };
  }
} catch {
  turns = { count: 0, sessionId };
}
turns.count += 1;
try {
  fs.mkdirSync(stateDir, { recursive: true });
  fs.writeFileSync(turnFile, JSON.stringify(turns));
} catch {
  // Non-fatal — continue.
}

// 3. Trigger-phrase detection (only after turn 5 to reduce false positives).
if (turns.count <= 5) process.exit(0);

let triggers = [];
try {
  triggers = JSON.parse(fs.readFileSync(triggersPath, 'utf8'));
} catch {
  process.exit(0);
}

const msgLower = userMessage.toLowerCase();
const matched = triggers.find((t) => msgLower.includes(t.toLowerCase()));
if (matched) {
  process.stdout.write(
    `<system-reminder>Sensei anti-drift: user phrase "${matched}" detected. Apply decision tree. DO NOT supply full code unless /reveal is invoked. Start at Tier 1 or refer the user to /reveal.</system-reminder>`,
  );
}

process.exit(0);
