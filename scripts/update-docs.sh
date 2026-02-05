#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-run}"
shift || true

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CODEX_DIR="$ROOT_DIR/.codex"
REPORT_DIR="$CODEX_DIR/reports"
LOG_DIR="$CODEX_DIR/logs"
DOCS_DIR="$ROOT_DIR/docs"

mkdir -p "$REPORT_DIR" "$LOG_DIR" "$DOCS_DIR"

timestamp() {
  date "+%Y%m%d-%H%M%S"
}

usage() {
  cat <<'EOF'
Usage: ./scripts/update-docs.sh [run]

Generates docs/CONTRIB.md and docs/RUNBOOK.md from package.json and .env.example.

Environment overrides:
  UPDATE_DOCS_DAYS_STALE   Days threshold for stale docs (default: 90).
EOF
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

run_action() {
  local ts report log days python_bin
  ts="$(timestamp)"
  report="$REPORT_DIR/update-docs_${ts}.md"
  log="$LOG_DIR/update-docs_${ts}.log"
  days="${UPDATE_DOCS_DAYS_STALE:-90}"

  : >"$log"

  if [[ ! -f "$ROOT_DIR/package.json" ]]; then
    cat >"$report" <<EOF
# Update Docs Report

Generated: $ts
Status: SKIPPED

Reason: package.json not found.
EOF
    echo "update-docs: skipped (no package.json). Report: $report"
    return 0
  fi

  if ! python_bin="$(find_python)"; then
    cat >"$report" <<EOF
# Update Docs Report

Generated: $ts
Status: FAILED

Reason: Python not available to parse package.json and .env.example.
EOF
    return 1
  fi

  "$python_bin" - "$ROOT_DIR" "$DOCS_DIR" "$days" "$log" <<'PY'
import json
import os
import sys
from datetime import datetime, timedelta

root = sys.argv[1]
docs_dir = sys.argv[2]
days_stale = int(sys.argv[3])
log_path = sys.argv[4]

package_path = os.path.join(root, "package.json")
env_path = os.path.join(root, ".env.example")

with open(package_path, "r", encoding="utf-8") as fh:
    package = json.load(fh)

scripts = package.get("scripts", {})

env_vars = []
if os.path.exists(env_path):
    with open(env_path, "r", encoding="utf-8") as fh:
        for raw in fh:
            line = raw.strip()
            if not line or line.startswith("#"):
                continue
            key = line.split("=", 1)[0].strip()
            if key:
                env_vars.append(key)

def fmt_script_row(name, cmd):
    return f"| `{name}` | `{cmd}` |"

contrib_path = os.path.join(docs_dir, "CONTRIB.md")
runbook_path = os.path.join(docs_dir, "RUNBOOK.md")

with open(contrib_path, "w", encoding="utf-8") as fh:
    fh.write("# Contributor Guide\n\n")
    fh.write("## Development Workflow\n")
    fh.write("- Install dependencies with your package manager of choice.\n")
    fh.write("- Run tests and linting before opening a PR.\n\n")
    fh.write("## Scripts\n")
    fh.write("| Script | Command |\n")
    fh.write("| --- | --- |\n")
    for name, cmd in scripts.items():
        fh.write(fmt_script_row(name, cmd) + "\n")
    fh.write("\n")
    fh.write("## Environment Setup\n")
    if env_vars:
        for key in env_vars:
            fh.write(f"- `{key}`: describe usage here.\n")
    else:
        fh.write("- No `.env.example` found.\n")
    fh.write("\n")
    fh.write("## Testing Procedures\n")
    fh.write("- Run unit tests (`npm test` or equivalent).\n")
    fh.write("- Run any project-specific verification scripts.\n")

with open(runbook_path, "w", encoding="utf-8") as fh:
    fh.write("# Runbook\n\n")
    fh.write("## Deployment Procedures\n")
    fh.write("- Document deployment steps here.\n\n")
    fh.write("## Monitoring and Alerts\n")
    fh.write("- Document monitoring/alerting here.\n\n")
    fh.write("## Common Issues and Fixes\n")
    fh.write("- Document common failure modes here.\n\n")
    fh.write("## Rollback Procedures\n")
    fh.write("- Document rollback steps here.\n")

cutoff = datetime.now() - timedelta(days=days_stale)
stale = []
docs_root = os.path.join(root, "docs")
if os.path.isdir(docs_root):
    for dirpath, _, filenames in os.walk(docs_root):
        for name in filenames:
            if not name.endswith(".md"):
                continue
            path = os.path.join(dirpath, name)
            try:
                mtime = datetime.fromtimestamp(os.path.getmtime(path))
            except OSError:
                continue
            if mtime < cutoff:
                rel = os.path.relpath(path, root)
                stale.append((rel, mtime))

with open(log_path, "a", encoding="utf-8") as fh:
    fh.write("Generated docs:\n")
    fh.write(f"- {os.path.relpath(contrib_path, root)}\n")
    fh.write(f"- {os.path.relpath(runbook_path, root)}\n")
    if stale:
        fh.write("\nStale docs (> %d days):\n" % days_stale)
        for rel, mtime in sorted(stale):
            fh.write(f"- {rel} (last modified {mtime.date()})\n")
PY

  cat >"$report" <<EOF
# Update Docs Report

Generated: $ts
Status: OK

Artifacts:
- Log: $log
- docs/CONTRIB.md
- docs/RUNBOOK.md
EOF

  echo "update-docs: report written: $report"
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
