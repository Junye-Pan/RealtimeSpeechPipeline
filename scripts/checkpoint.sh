#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-}"
NAME="${2:-}"
KEEP="${3:-5}"

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

LOG_DIR=".codex"
LOG_FILE="$LOG_DIR/checkpoints.log"
META_DIR="$LOG_DIR/checkpoints"

mkdir -p "$LOG_DIR" "$META_DIR"

now_ts() { date +%Y-%m-%d-%H:%M; }
slugify() { echo "$1" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g' | sed -E 's/^-+|-+$//g'; }

git_clean() {
  [[ -z "$(git status --porcelain)" ]]
}

coverage_read() {
  # Optional: export CHECKPOINT_COVERAGE_CMD='python scripts/coverage_pct.py'
  if [[ -n "${CHECKPOINT_COVERAGE_CMD:-}" ]]; then
    bash -lc "$CHECKPOINT_COVERAGE_CMD" || true
  fi
}

tests_run() {
  local mode="$1"
  ./scripts/verify.sh "$mode"
}

latest_checkpoint_line() {
  local name="$1"
  # last matching line
  grep -F " | $name | " "$LOG_FILE" | tail -n 1 || true
}

write_meta() {
  local path="$1"
  shift
  printf "%s\n" "$@" > "$path"
}

create_checkpoint() {
  if [[ -z "$NAME" ]]; then
    echo "checkpoint.sh create <name>"; exit 2
  fi

  local TS SHA BRANCH SLUG META_PATH
  TS="$(now_ts)"
  SHA="$(git rev-parse --short HEAD)"
  BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo detached)"
  SLUG="$(slugify "$NAME")"
  META_PATH="$META_DIR/${TS}_${SLUG}.txt"

  echo "1) verify quick..."
  tests_run "quick" || { echo "verify quick failed; aborting checkpoint create"; exit 1; }

  local KIND REF EXTRA=""
  if git_clean; then
    KIND="tag"
    REF="checkpoint/${SLUG}/${TS}"
    git tag -a "$REF" -m "checkpoint $NAME @ $TS" "$SHA"
  else
    KIND="stash"
    git stash push -u -m "checkpoint:${NAME} @ ${TS}" >/dev/null
    REF="$(git rev-parse --short stash@{0})"
    EXTRA="stash_ref=stash@{0}"
  fi

  local COV
  COV="$(coverage_read | tail -n 1 || true)"

  echo "${TS} | ${NAME} | ${SHA} | kind=${KIND} ref=${REF} branch=${BRANCH} cov=${COV} ${EXTRA}" >> "$LOG_FILE"

  write_meta "$META_PATH" \
    "timestamp=${TS}" \
    "name=${NAME}" \
    "sha=${SHA}" \
    "branch=${BRANCH}" \
    "kind=${KIND}" \
    "ref=${REF}" \
    "coverage=${COV}"

  echo "Checkpoint created: ${NAME}"
  echo "  base_sha: ${SHA}"
  echo "  kind: ${KIND}  ref: ${REF}"
  echo "  log:  ${LOG_FILE}"
  echo "  meta: ${META_PATH}"
}

verify_checkpoint() {
  if [[ -z "$NAME" ]]; then
    echo "checkpoint.sh verify <name>"; exit 2
  fi
  if [[ ! -f "$LOG_FILE" ]]; then
    echo "No checkpoints yet: $LOG_FILE missing"; exit 1
  fi

  local LINE
  LINE="$(latest_checkpoint_line "$NAME")"
  if [[ -z "$LINE" ]]; then
    echo "Checkpoint not found: $NAME"; exit 1
  fi

  local BASE_TS BASE_SHA
  BASE_TS="$(echo "$LINE" | awk -F' | ' '{print $1}')"
  BASE_SHA="$(echo "$LINE" | awk -F' | ' '{print $3}')"

  local CUR_SHA
  CUR_SHA="$(git rev-parse --short HEAD)"

  # Files changed since checkpoint (commit diff)
  local CHANGES
  CHANGES="$(git diff --name-status "${BASE_SHA}..HEAD" || true)"
  local FILES_CHANGED
  FILES_CHANGED="$(echo "$CHANGES" | sed '/^\s*$/d' | wc -l | tr -d ' ')"

  echo "Running tests (quick) for current state..."
  local NOW_TEST="PASS"
  if ! tests_run "quick" >/dev/null 2>&1; then
    NOW_TEST="FAIL"
  fi

  local NOW_COV
  NOW_COV="$(coverage_read | tail -n 1 || true)"

  echo
  echo "CHECKPOINT COMPARISON: ${NAME}"
  echo "============================"
  echo "Base:    ${BASE_SHA} (@ ${BASE_TS})"
  echo "Current: ${CUR_SHA}"
  echo "Files changed: ${FILES_CHANGED}"
  if [[ -n "$CHANGES" ]]; then
    echo "$CHANGES" | sed 's/^/  /'
  fi
  echo "Tests: ${NOW_TEST}"
  echo "Coverage: ${NOW_COV}"
  echo "Build: (wire to your build cmd if needed)"
}

list_checkpoints() {
  if [[ ! -f "$LOG_FILE" ]]; then
    echo "No checkpoints yet."
    exit 0
  fi

  local HEAD
  HEAD="$(git rev-parse --short HEAD)"

  echo "NAME | TIMESTAMP | SHA | STATUS"
  echo "--------------------------------"
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    ts="$(echo "$line" | awk -F' | ' '{print $1}')"
    name="$(echo "$line" | awk -F' | ' '{print $2}')"
    sha="$(echo "$line" | awk -F' | ' '{print $3}')"
    status="behind"
    if [[ "$sha" == "$HEAD" ]]; then
      status="current"
    else
      # ancestor check: if sha is NOT ancestor -> diverged/ahead (best-effort)
      if ! git merge-base --is-ancestor "$sha" HEAD >/dev/null 2>&1; then
        status="diverged"
      fi
    fi
    echo "${name} | ${ts} | ${sha} | ${status}"
  done < "$LOG_FILE"
}

clear_checkpoints() {
  if [[ ! -f "$LOG_FILE" ]]; then exit 0; fi
  local total
  total="$(wc -l < "$LOG_FILE" | tr -d ' ')"
  if [[ "$total" -le "$KEEP" ]]; then
    echo "Nothing to clear (<= ${KEEP} checkpoints)."
    exit 0
  fi

  tail -n "$KEEP" "$LOG_FILE" > "$LOG_FILE.tmp"
  mv "$LOG_FILE.tmp" "$LOG_FILE"
  echo "Kept last ${KEEP} checkpoints in ${LOG_FILE}."
  echo "Note: old meta files under ${META_DIR} are not pruned automatically (safe default)."
}

case "$ACTION" in
  create) create_checkpoint ;;
  verify) verify_checkpoint ;;
  list)   list_checkpoints ;;
  clear)  clear_checkpoints ;;
  *)
    echo "Usage:"
    echo "  ./scripts/checkpoint.sh create <name>"
    echo "  ./scripts/checkpoint.sh verify <name>"
    echo "  ./scripts/checkpoint.sh list"
    echo "  ./scripts/checkpoint.sh clear [keepN]"
    exit 2
    ;;
esac
