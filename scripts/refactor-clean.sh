#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-}"
shift || true

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CODEX_DIR="$ROOT_DIR/.codex"
REPORT_DIR="$CODEX_DIR/reports"
LOG_DIR="$CODEX_DIR/logs"
PLAN_DIR="$CODEX_DIR/plans"
TRASH_DIR="$CODEX_DIR/trash"

mkdir -p "$REPORT_DIR" "$LOG_DIR" "$PLAN_DIR" "$TRASH_DIR"

timestamp() {
  date "+%Y%m%d-%H%M%S"
}

usage() {
  cat <<'EOF'
Usage: ./scripts/refactor-clean.sh [scan|apply] [plan-file]

Commands:
  scan               Run dead-code analysis tools and write report/log artifacts.
  apply <plan-file>  Move listed paths to .codex/trash/, run verify before/after.

Plan file format:
  - One relative path per line (relative to repo root)
  - Blank lines and lines starting with # are ignored
EOF
}

resolve_tool_cmd() {
  local tool="$1"

  if [[ -x "$ROOT_DIR/node_modules/.bin/$tool" ]]; then
    echo "$ROOT_DIR/node_modules/.bin/$tool"
    return 0
  fi

  if command -v "$tool" >/dev/null 2>&1; then
    echo "$tool"
    return 0
  fi

  if command -v npx >/dev/null 2>&1; then
    echo "npx --no-install $tool"
    return 0
  fi

  return 1
}

scan_action() {
  local ts
  ts="$(timestamp)"
  local report="$REPORT_DIR/refactor-clean_${ts}.md"
  local log="$LOG_DIR/refactor-clean_${ts}.log"

  : >"$log"

  if [[ ! -f "$ROOT_DIR/package.json" ]]; then
    cat >"$report" <<EOF
# Refactor Clean Report

Generated: $ts
Status: SKIPPED

Reason: No package.json found; dead-code tooling is JS/TS-specific.
EOF
    echo "refactor-clean: skipped (no package.json). Report: $report"
    return 0
  fi

  local ran_any="false"
  local exit_code=0
  local statuses=()

  run_tool() {
    local name="$1"
    local cmd="$2"
    local args="${3:-}"

    if [[ -z "$cmd" ]]; then
      statuses+=("$name|SKIPPED (tool not found)")
      return 0
    fi

    ran_any="true"
    {
      echo "==== $name ===="
      echo "Command: $cmd $args"
    } >>"$log"

    if bash -lc "$cmd $args" >>"$log" 2>&1; then
      statuses+=("$name|OK")
    else
      statuses+=("$name|FAILED (see log)")
      exit_code=1
    fi

    echo >>"$log"
  }

  local knip_cmd depcheck_cmd ts_prune_cmd
  knip_cmd="$(resolve_tool_cmd knip || true)"
  depcheck_cmd="$(resolve_tool_cmd depcheck || true)"
  ts_prune_cmd="$(resolve_tool_cmd ts-prune || true)"

  run_tool "knip" "$knip_cmd"
  run_tool "depcheck" "$depcheck_cmd"
  run_tool "ts-prune" "$ts_prune_cmd"

  if [[ "$ran_any" != "true" ]]; then
    cat >"$report" <<EOF
# Refactor Clean Report

Generated: $ts
Status: SKIPPED

Reason: No supported dead-code tools found (knip, depcheck, ts-prune).
EOF
    echo "refactor-clean: skipped (no tools). Report: $report"
    return 0
  fi

  {
    echo "# Refactor Clean Report"
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
    echo
    echo "## Next Steps"
    echo "- Review the log output and categorize findings into SAFE / CAUTION / DANGER."
    echo "- Create a plan file under $PLAN_DIR (one relative path per line)."
    echo "- Apply the plan with: ./scripts/refactor-clean.sh apply <plan-file>"
  } >"$report"

  echo "refactor-clean: report written: $report"
  return "$exit_code"
}

apply_action() {
  local plan_file="${1:-}"
  if [[ -z "$plan_file" || ! -f "$plan_file" ]]; then
    echo "refactor-clean: apply requires an existing plan file."
    usage
    return 2
  fi

  local ts
  ts="$(timestamp)"
  local report="$REPORT_DIR/refactor-clean_apply_${ts}.md"
  local log="$LOG_DIR/refactor-clean_apply_${ts}.log"
  local trash_run="$TRASH_DIR/refactor-clean_${ts}"

  mkdir -p "$trash_run"
  : >"$log"

  {
    echo "# Refactor Clean Apply"
    echo
    echo "Generated: $ts"
    echo "Plan: $plan_file"
    echo "Trash: $trash_run"
    echo
  } >"$report"

  echo "Running verify (pre)" >>"$log"
  if ! "$ROOT_DIR/scripts/verify.sh" full >>"$log" 2>&1; then
    {
      echo "Status: FAILED"
      echo
      echo "Reason: pre-apply verification failed. See log: $log"
    } >>"$report"
    return 1
  fi

  local moved=()
  while IFS= read -r raw || [[ -n "$raw" ]]; do
    local line
    line="${raw%%#*}"
    line="${line%$'\r'}"
    # Trim leading/trailing whitespace without extglob.
    line="${line#"${line%%[![:space:]]*}"}"
    line="${line%"${line##*[![:space:]]}"}"
    if [[ -z "$line" ]]; then
      continue
    fi
    if [[ "$line" = /* ]]; then
      echo "Skipping absolute path in plan: $line" >>"$log"
      continue
    fi

    local src="$ROOT_DIR/$line"
    if [[ ! -e "$src" ]]; then
      echo "Missing path in plan (skipped): $line" >>"$log"
      continue
    fi

    local dest="$trash_run/$line"
    mkdir -p "$(dirname "$dest")"
    mv "$src" "$dest"
    moved+=("$line")
  done <"$plan_file"

  if [[ ${#moved[@]} -eq 0 ]]; then
    {
      echo "Status: SKIPPED"
      echo
      echo "Reason: no valid paths to move from plan file."
      echo "Log: $log"
    } >>"$report"
    return 0
  fi

  echo "Running verify (post)" >>"$log"
  if ! "$ROOT_DIR/scripts/verify.sh" full >>"$log" 2>&1; then
    echo "Post-apply verify failed; restoring." >>"$log"
    for rel in "${moved[@]}"; do
      local src="$trash_run/$rel"
      local dest="$ROOT_DIR/$rel"
      mkdir -p "$(dirname "$dest")"
      if [[ -e "$src" ]]; then
        mv "$src" "$dest"
      fi
    done

    {
      echo "Status: FAILED"
      echo
      echo "Reason: post-apply verification failed; changes restored."
      echo "Log: $log"
    } >>"$report"
    return 1
  fi

  {
    echo "Status: OK"
    echo
    echo "Moved paths: ${#moved[@]}"
    for rel in "${moved[@]}"; do
      echo "- $rel"
    done
    echo
    echo "Log: $log"
  } >>"$report"

  echo "refactor-clean: apply complete. Report: $report"
}

case "$ACTION" in
  scan)
    scan_action
    ;;
  apply)
    apply_action "$@"
    ;;
  *)
    usage
    exit 2
    ;;
esac
