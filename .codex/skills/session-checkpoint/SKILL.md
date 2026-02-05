---
name: session-checkpoint
description: Create or append a session checkpoint log under .codex/sessions with repo state, decisions, evidence, and next steps so a future Codex session can resume reliably. Use when ending a session, after milestones, or before risky changes.
---

# Session Checkpoint

## Goal
Create or append a single session log file that persists across Codex sessions. Keep it short, evidence-based, and optimized for resuming work later.

## Log location
- Primary (repo-scoped): `.codex/sessions/`
- Optional (user-scoped): `~/.codex/sessions/` (use only if permitted)

Always keep **one file per session** and append to it until the session ends.

## Naming convention
- New file per session: `YYYY-MM-DD-<topic>-<shortid>.tmp`
- Maintain pointer: `.codex/sessions/LATEST` containing the current session log path

Store the path in `LATEST` as a repo-relative path (preferred). If an absolute path is used, keep it consistent for the session.

If `LATEST` points to an existing session log for today/topic, append. Otherwise create a new file and update `LATEST`.
If `LATEST` points to a missing file, treat it as missing and create a new session file.

## Required steps

### 1) Determine topic + session file
- Propose a short `<topic>` (kebab-case, <= 6 words).
- Generate `<shortid>` (8 chars). Prefer: first 8 of `YYYYMMDDHHMMSS` or a random hex. If unavailable, use `manual`.
- Decide final file path (default: `.codex/sessions/<filename>.tmp`).
- Create directories if missing.
- If file exists, append a new checkpoint section; do not rewrite the whole file.

### 2) Collect repo state (evidence-oriented)
Record:
- local date/time
- repo root path
- current working directory
- current git branch
- HEAD commit hash (short)
- whether there are uncommitted changes

If any are unavailable, state that explicitly.

### 3) Append the checkpoint section (exact structure)
Append a new section using this exact structure:

```
---
## Checkpoint: <timestamp> <topic>

### Objective (1-3 bullets)
- ...

### What changed (verified)
- (Link to files / mention functions touched)
- (Include commands run + key outputs when relevant)

### What worked (with evidence)
- ...
  - Evidence: (short excerpt only: test output, command result, diff summary, or file reference)

### What didn’t work (with evidence)
- ...
  - Evidence: (short excerpt only: error message excerpt, failing test name, etc.)

### Decisions & rationale
- Decision:
- Why:
- Trade-offs:
- Follow-ups:

### Open questions / unknowns
- ...

### Next steps (ordered, smallest-to-largest)
1. ...
2. ...

### Quick resume prompt
Paste this into a new Codex session:
> Read and follow the session log at: <absolute-or-relative-path>
> First, restate current status, then execute the Next steps in order.
---
```

### 4) Show user a review snippet
After writing, show:
- the session log path
- the last checkpoint section (or a tight summary of it)

Ask the user to request edits if anything is wrong **before continuing work**.

## Style constraints
- Prefer bullet lists.
- No speculation: if unsure, mark as “Unverified”.
- Keep it short; prioritize actionable next steps over narrative.

## When to run
- After a milestone (e.g., tests pass, refactor complete, decision made)
- Before stopping work
- Before a risky change (big refactor, dependency bump)
