#!/usr/bin/env node
// Sensei session boundary hook.
// Clears per-session state so /emergency-mode re-enables on new session
// and turn counter starts fresh.

import fs from 'node:fs';
import path from 'node:path';

let input = {};
try {
  const raw = fs.readFileSync(0, 'utf8');
  input = raw ? JSON.parse(raw) : {};
} catch {
  input = {};
}

// Prefer payload cwd so subagent/monorepo contexts clear the correct state.
const root = input.cwd || process.cwd();
const stateDir = path.join(root, '.claude', 'session-state');

for (const f of [
  path.join(stateDir, 'sensei-emergency.flag'),
  path.join(stateDir, 'sensei-turn-count.json'),
]) {
  try {
    if (fs.existsSync(f)) fs.unlinkSync(f);
  } catch {
    // Non-fatal.
  }
}

process.exit(0);
