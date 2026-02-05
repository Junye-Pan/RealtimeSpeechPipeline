---
name: test-coverage
description: Analyze test coverage by running scripts/test-coverage.sh and writing reports/logs under .codex/.
---

## When to use
- After adding or changing features to ensure coverage meets targets
- When preparing for release or PR readiness checks
- When coverage regressions are suspected

## Inputs
- `run` (default): runs tests with coverage and analyzes results
- `analyze`: analyzes an existing coverage summary without running tests

Environment overrides:
- `TEST_COVERAGE_CMD` — custom test command
- `COVERAGE_SUMMARY_PATH` — path to coverage-summary.json
- `COVERAGE_THRESHOLD` — minimum acceptable line coverage (default 80)

## Outputs (artifacts)
- Report: `.codex/reports/test-coverage_<timestamp>.md`
- Log: `.codex/logs/test-coverage_<timestamp>.log`

## Execution (must follow)
- Always run: `./scripts/test-coverage.sh run` before reporting coverage status.
- Do NOT claim coverage is improved unless the script ran and exited 0.
- If coverage is below threshold, propose targeted tests and rerun.

## Report format
TEST COVERAGE REPORT
====================
Status: [OK/FAILED/SKIPPED]
Summary path: <path>
Threshold: <n>%
Under threshold: <list>
Artifacts:
- Report: <path>
- Log: <path>
