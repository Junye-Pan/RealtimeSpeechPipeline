#!/usr/bin/env bash
set -euo pipefail

mkdir -p .codex/ops

LOG_PATH=".codex/ops/security-baseline-check.log"
JSON_PATH=".codex/ops/security-baseline-report.json"
MD_PATH=".codex/ops/security-baseline-report.md"

START_TS="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
STATUS="pass"

{
  echo "security-baseline-check started_at=${START_TS}"
  echo "running focused security baseline test suites"
} | tee "${LOG_PATH}"

if ! go test ./api/eventabi ./api/observability ./internal/security/policy ./internal/runtime/transport ./internal/observability/replay ./internal/observability/timeline 2>&1 | tee -a "${LOG_PATH}"; then
  STATUS="fail"
fi

END_TS="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

cat > "${JSON_PATH}" <<EOF
{
  "started_at_utc": "${START_TS}",
  "finished_at_utc": "${END_TS}",
  "status": "${STATUS}",
  "checks": [
    "go test ./api/eventabi",
    "go test ./api/observability",
    "go test ./internal/security/policy",
    "go test ./internal/runtime/transport",
    "go test ./internal/observability/replay",
    "go test ./internal/observability/timeline"
  ]
}
EOF

cat > "${MD_PATH}" <<EOF
# Security Baseline Check

- Started (UTC): ${START_TS}
- Finished (UTC): ${END_TS}
- Status: ${STATUS}

## Checks
- \`go test ./api/eventabi\`
- \`go test ./api/observability\`
- \`go test ./internal/security/policy\`
- \`go test ./internal/runtime/transport\`
- \`go test ./internal/observability/replay\`
- \`go test ./internal/observability/timeline\`
EOF

if [[ "${STATUS}" != "pass" ]]; then
  exit 1
fi
