---
name: refactor-clean
description: Identify and safely remove dead code by running scripts/refactor-clean.sh, logging artifacts under .codex/, and verifying before/after changes.
---

## When to use
- Before removing suspected dead code or unused dependencies
- After large refactors where cleanup is needed
- When dead-code tools (knip/depcheck/ts-prune) are available

## Inputs
- `scan` — runs analysis tools and writes a report
- `apply <plan-file>` — moves listed paths to `.codex/trash/` with verify gates

## Outputs (artifacts)
- Reports: `.codex/reports/refactor-clean_<timestamp>.md`
- Logs: `.codex/logs/refactor-clean_<timestamp>.log`
- Plans: `.codex/plans/` (user-authored, one path per line)
- Trash: `.codex/trash/refactor-clean_<timestamp>/` (moved files)

## Execution (must follow)
- Always run: `./scripts/refactor-clean.sh scan` before proposing any deletions.
- Do NOT claim code was removed unless `apply` ran and exited 0.
- Apply path removals by moving to `.codex/trash/` (never delete directly).
- On `apply`, the script runs `./scripts/verify.sh full` before and after changes.
- If verification fails, the script restores moved paths and reports failure.

## Report format
REFACTOR CLEAN REPORT
======================
Status: [OK/FAILED/SKIPPED]
Artifacts:
- Report: <path>
- Log: <path>
Next step: [review log | create plan | apply plan]
