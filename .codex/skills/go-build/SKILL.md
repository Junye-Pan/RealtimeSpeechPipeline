---
name: go-build
description: Run Go build diagnostics by executing scripts/go-build.sh and writing reports/logs under .codex/.
---

## When to use
- When Go builds fail or after pulling changes
- Before PRs that touch Go code
- When vet/staticcheck/lint issues are suspected

## Inputs
- `run` (default): runs build diagnostics and emits a report

## Outputs (artifacts)
- Report: `.codex/reports/go-build_<timestamp>.md`
- Log: `.codex/logs/go-build_<timestamp>.log`

## Execution (must follow)
- If no `.go` files exist, the script auto-skips.
- Always run: `./scripts/go-build.sh run` before claiming Go build status.
- Do NOT claim success unless the script ran and exited 0.

## Report format
GO BUILD REPORT
==============
Status: [OK/FAILED/SKIPPED]
Artifacts:
- Report: <path>
- Log: <path>
