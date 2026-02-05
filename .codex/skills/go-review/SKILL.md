---
name: go-review
description: Run Go code review diagnostics by executing scripts/go-review.sh and writing reports/logs under .codex/.
---

## When to use
- Before committing Go changes
- During Go-focused code reviews
- After refactors touching concurrency/security-sensitive code

## Inputs
- `run` (default): runs review diagnostics and emits a report

## Outputs (artifacts)
- Report: `.codex/reports/go-review_<timestamp>.md`
- Log: `.codex/logs/go-review_<timestamp>.log`

## Execution (must follow)
- If no `.go` files exist, the script auto-skips.
- Always run: `./scripts/go-review.sh run` before claiming review status.
- Do NOT claim success unless the script ran and exited 0.

## Report format
GO REVIEW REPORT
================
Status: [OK/FAILED/SKIPPED]
Artifacts:
- Report: <path>
- Log: <path>
