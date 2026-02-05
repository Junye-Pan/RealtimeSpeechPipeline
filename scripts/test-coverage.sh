#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-run}"
shift || true

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CODEX_DIR="$ROOT_DIR/.codex"
REPORT_DIR="$CODEX_DIR/reports"
LOG_DIR="$CODEX_DIR/logs"

mkdir -p "$REPORT_DIR" "$LOG_DIR"

timestamp() {
  date "+%Y%m%d-%H%M%S"
}

usage() {
  cat <<'EOF'
Usage: ./scripts/test-coverage.sh [run|analyze]

Commands:
  run       Run tests with coverage and analyze results.
  analyze   Analyze an existing coverage summary without running tests.

Environment overrides:
  TEST_COVERAGE_CMD       Override the test command to run.
  COVERAGE_SUMMARY_PATH   Path to coverage-summary.json (default: coverage/coverage-summary.json).
  COVERAGE_THRESHOLD      Minimum acceptable line coverage (default: 80).
EOF
}

detect_pkg_manager() {
  if [[ -n "${CLAUDE_PACKAGE_MANAGER:-}" ]]; then
    echo "$CLAUDE_PACKAGE_MANAGER"
    return
  fi
  if [[ -f "$ROOT_DIR/pnpm-lock.yaml" ]]; then
    echo "pnpm"
    return
  fi
  if [[ -f "$ROOT_DIR/yarn.lock" ]]; then
    echo "yarn"
    return
  fi
  if [[ -f "$ROOT_DIR/bun.lockb" ]]; then
    echo "bun"
    return
  fi
  echo "npm"
}

detect_cmd() {
  if [[ -n "${TEST_COVERAGE_CMD:-}" ]]; then
    echo "$TEST_COVERAGE_CMD"
    return
  fi

  if [[ ! -f "$ROOT_DIR/package.json" ]]; then
    echo ""
    return
  fi

  local pm
  pm="$(detect_pkg_manager)"
  case "$pm" in
    pnpm)
      echo "pnpm test -- --coverage"
      ;;
    yarn)
      echo "yarn test --coverage"
      ;;
    bun)
      echo "bun test --coverage"
      ;;
    npm|*)
      echo "npm test -- --coverage"
      ;;
  esac
}

find_python() {
  if command -v python3 >/dev/null 2>&1; then
    echo "python3"
    return 0
  fi
  if command -v python >/dev/null 2>&1; then
    echo "python"
    return 0
  fi
  return 1
}

analyze_summary() {
  local summary_path="$1"
  local threshold="$2"
  local python_bin
  if ! python_bin="$(find_python)"; then
    echo "Python not available; unable to parse coverage summary."
    return 2
  fi

  "$python_bin" - "$summary_path" "$threshold" <<'PY'
import json
import sys

path = sys.argv[1]
threshold = float(sys.argv[2])

with open(path, "r", encoding="utf-8") as fh:
    data = json.load(fh)

total = data.get("total", {})
def pct(section):
    if not isinstance(section, dict):
        return None
    value = section.get("pct")
    return value if value is not None else None

total_lines = pct(total.get("lines"))
total_statements = pct(total.get("statements"))
total_branches = pct(total.get("branches"))
total_functions = pct(total.get("functions"))

under = []
for name, stats in data.items():
    if name == "total":
        continue
    lines = stats.get("lines", {})
    line_pct = lines.get("pct")
    if line_pct is None:
        continue
    if line_pct < threshold:
        under.append((line_pct, name))

under.sort()

print("## Coverage Summary")
print()
print(f"- Lines: {total_lines if total_lines is not None else 'n/a'}%")
print(f"- Statements: {total_statements if total_statements is not None else 'n/a'}%")
print(f"- Branches: {total_branches if total_branches is not None else 'n/a'}%")
print(f"- Functions: {total_functions if total_functions is not None else 'n/a'}%")
print()
print(f"## Under {threshold:.0f}% Line Coverage")
if under:
    for pct_value, name in under:
        print(f"- {name} ({pct_value}%)")
else:
    print("- None")
PY
}

run_action() {
  local ts report log summary_path threshold cmd
  ts="$(timestamp)"
  report="$REPORT_DIR/test-coverage_${ts}.md"
  log="$LOG_DIR/test-coverage_${ts}.log"
  summary_path="${COVERAGE_SUMMARY_PATH:-coverage/coverage-summary.json}"
  threshold="${COVERAGE_THRESHOLD:-80}"

  : >"$log"

  if [[ "$ACTION" == "run" ]]; then
    cmd="$(detect_cmd)"
    if [[ -z "$cmd" ]]; then
      cat >"$report" <<EOF
# Test Coverage Report

Generated: $ts
Status: SKIPPED

Reason: No package.json found and TEST_COVERAGE_CMD not set.
EOF
      echo "test-coverage: skipped (no command). Report: $report"
      return 0
    fi

    {
      echo "Command: $cmd"
      echo "Summary path: $summary_path"
      echo "Threshold: $threshold%"
      echo
    } >>"$log"

    if ! bash -lc "$cmd" >>"$log" 2>&1; then
      {
        echo "# Test Coverage Report"
        echo
        echo "Generated: $ts"
        echo "Status: FAILED"
        echo
        echo "Reason: test command failed."
        echo "Log: $log"
      } >"$report"
      return 1
    fi
  fi

  if [[ ! -f "$ROOT_DIR/$summary_path" ]]; then
    cat >"$report" <<EOF
# Test Coverage Report

Generated: $ts
Status: FAILED

Reason: Coverage summary not found at $summary_path.
Log: $log
EOF
    return 1
  fi

  local analysis
  if ! analysis="$(analyze_summary "$ROOT_DIR/$summary_path" "$threshold")"; then
    cat >"$report" <<EOF
# Test Coverage Report

Generated: $ts
Status: FAILED

Reason: Unable to parse coverage summary.
Log: $log
EOF
    return 1
  fi

  {
    echo "# Test Coverage Report"
    echo
    echo "Generated: $ts"
    echo "Status: OK"
    echo
    echo "Summary path: $summary_path"
    echo "Threshold: $threshold%"
    echo
    echo "$analysis"
    echo
    echo "## Artifacts"
    echo "- Log: $log"
  } >"$report"

  echo "test-coverage: report written: $report"
}

case "$ACTION" in
  run|analyze)
    run_action
    ;;
  *)
    usage
    exit 2
    ;;
esac
