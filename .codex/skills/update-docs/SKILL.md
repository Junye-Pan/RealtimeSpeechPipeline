---
name: update-docs
description: Generate docs from package.json and .env.example by running scripts/update-docs.sh, with artifacts under .codex/.
---

## When to use
- When scripts or environment variables change
- Before release or onboarding updates
- When docs are stale or out of sync

## Inputs
- `run` (default): generates docs/CONTRIB.md and docs/RUNBOOK.md

Environment overrides:
- `UPDATE_DOCS_DAYS_STALE` — days threshold for stale docs listing (default 90)

## Outputs (artifacts)
- Report: `.codex/reports/update-docs_<timestamp>.md`
- Log: `.codex/logs/update-docs_<timestamp>.log`
- Generated docs: `docs/CONTRIB.md`, `docs/RUNBOOK.md`

## Execution (must follow)
- Always run: `./scripts/update-docs.sh run` before claiming docs are updated.
- Do NOT claim docs are updated unless the script ran and exited 0.

## Report format
UPDATE DOCS REPORT
==================
Status: [OK/FAILED/SKIPPED]
Artifacts:
- Report: <path>
- Log: <path>
- docs/CONTRIB.md
- docs/RUNBOOK.md
