---
name: checkpoint
description: Create/verify/list/clear workflow checkpoints for this repo by running scripts/checkpoint.sh, logging to .codex/checkpoints.log, and producing a comparison report.
---

## When to use
- Before risky refactors, big dependency changes, or multi-file rewrites
- After finishing a logical milestone (e.g. core-done, refactor-done)
- When asked “what changed since X?” or “compare vs X”

## How to use (user-facing)
Preferred:
- `$checkpoint create "<name>"`
- `$checkpoint verify "<name>"`
- `$checkpoint list`
- `$checkpoint clear` (keeps last 5 by default)

If the user asks in natural language (“create a checkpoint named core-done”), interpret it as the same.

## Execution rules (agent behavior)
- Do NOT claim a checkpoint was created/verified unless the script actually ran.
- Always use the repo scripts:
  - `./scripts/checkpoint.sh create "<name>"`
  - `./scripts/checkpoint.sh verify "<name>"`
  - `./scripts/checkpoint.sh list`
  - `./scripts/checkpoint.sh clear [keepN]`
- On create: run quick verify first (handled by script), then snapshot (stash or tag), then log + metadata.
- On verify: produce a report in this format:

CHECKPOINT COMPARISON: <NAME>
============================
Files changed: <X>
Tests: <base PASS/FAIL> -> <now PASS/FAIL>
Coverage: <base %> -> <now %>
Build: <PASS/FAIL>

## Naming conventions
- Use kebab-case; include milestone intent:
  - feature-start, core-done, refactor-done, pre-pr
- If no name is given, propose a sensible default based on the task.
