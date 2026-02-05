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
Usage: ./scripts/go-build.sh [run]

Runs Go build diagnostics: go build, go vet, staticcheck (if available),
golangci-lint (if available), and go mod verify.
EOF
}

has_go_files() {
  if command -v rg >/dev/null 2>&1; then
    rg --files -g '*.go' "$ROOT_DIR" >/dev/null 2>&1
    return $?
  fi
  find "$ROOT_DIR" -type f -name '*.go' | grep -q .
}

run_action() {
  local ts report log
  ts="$(timestamp)"
  report="$REPORT_DIR/go-build_${ts}.md"
  log="$LOG_DIR/go-build_${ts}.log"

  : >"$log"

  if ! has_go_files; then
    cat >"$report" <<EOF
# Go Build Report

Generated: $ts
Status: SKIPPED

Reason: No .go files found in repository.
EOF
    echo "go-build: skipped (no Go files). Report: $report"
    return 0
  fi

  if ! command -v go >/dev/null 2>&1; then
    cat >"$report" <<EOF
# Go Build Report

Generated: $ts
Status: FAILED

Reason: Go toolchain not available in PATH.
EOF
    return 1
  fi

  local exit_code=0
  local statuses=()

  run_tool() {
    local name="$1"
    local cmd="$2"
    if [[ -z "$cmd" ]]; then
      statuses+=("$name|SKIPPED (tool not found)")
      return 0
    fi
    {
      echo "==== $name ===="
      echo "Command: $cmd"
    } >>"$log"
    if bash -lc "$cmd" >>"$log" 2>&1; then
      statuses+=("$name|OK")
    else
      statuses+=("$name|FAILED (see log)")
      exit_code=1
    fi
    echo >>"$log"
  }

  run_tool "go build" "go build ./..."
  run_tool "go vet" "go vet ./..."
  if command -v staticcheck >/dev/null 2>&1; then
    run_tool "staticcheck" "staticcheck ./..."
  else
    statuses+=("staticcheck|SKIPPED (tool not found)")
  fi
  if command -v golangci-lint >/dev/null 2>&1; then
    run_tool "golangci-lint" "golangci-lint run"
  else
    statuses+=("golangci-lint|SKIPPED (tool not found)")
  fi
  run_tool "go mod verify" "go mod verify"

  {
    echo "# Go Build Report"
    echo
    echo "Generated: $ts"
    if [[ $exit_code -eq 0 ]]; then
      echo "Status: OK"
    else
      echo "Status: FAILED"
    fi
    echo
    echo "## Tool Results"
    for entry in "${statuses[@]}"; do
      IFS='|' read -r name status <<<"$entry"
      echo "- $name: $status"
    done
    echo
    echo "## Artifacts"
    echo "- Log: $log"
  } >"$report"

  echo "go-build: report written: $report"
  return "$exit_code"
}

case "$ACTION" in
  run)
    run_action
    ;;
  *)
    usage
    exit 2
    ;;
esac
