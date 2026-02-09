#!/usr/bin/env bash
set -euo pipefail

MODE="${1:-quick}"

# Allow explicit override from env
# e.g. export VERIFY_QUICK_CMD="uv run -m pytest -q"
#      export VERIFY_FULL_CMD="uv run -m pytest"
if [[ "$MODE" == "quick" ]]; then
  CMD="${VERIFY_QUICK_CMD:-}"
else
  CMD="${VERIFY_FULL_CMD:-}"
fi

detect_cmd() {
  if [[ -n "$CMD" ]]; then
    echo "$CMD"; return
  fi

  if [[ -f "Makefile" ]] && make -n "verify-$MODE" >/dev/null 2>&1; then
    echo "make verify-$MODE"; return
  fi

  if [[ -f "package.json" ]]; then
    if [[ "$MODE" == "quick" ]]; then
      echo "npm test --silent"; return
    else
      echo "npm test"; return
    fi
  fi

  if [[ -f "pyproject.toml" || -f "requirements.txt" ]]; then
    if command -v pytest >/dev/null 2>&1; then
      if [[ "$MODE" == "quick" ]]; then
        echo "pytest -q"; return
      else
        echo "pytest"; return
      fi
    fi
  fi

  echo ""
}

RUN_CMD="$(detect_cmd)"
if [[ -z "$RUN_CMD" ]]; then
  MODE_UPPER="$(printf '%s' "$MODE" | tr '[:lower:]' '[:upper:]')"
  echo "verify.sh: No default verify command found."
  echo "Set VERIFY_${MODE_UPPER}_CMD to enforce checks."
  exit 0
fi

echo "verify.sh ($MODE): running: $RUN_CMD"
bash -lc "$RUN_CMD"
