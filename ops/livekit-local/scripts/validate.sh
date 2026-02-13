#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
ENV_PATH="$REPO_ROOT/ops/livekit-local/.env.generated"
COMPOSE_PATH="$REPO_ROOT/ops/livekit-local/docker-compose.yml"
REPORT_PATH="$REPO_ROOT/.codex/transports/livekit-report.json"

if [[ ! -f "$ENV_PATH" ]]; then
  echo "validate: missing $ENV_PATH" >&2
  echo "validate: run bootstrap first:" >&2
  echo "  bash ops/livekit-local/scripts/bootstrap.sh" >&2
  exit 1
fi

set -a
# shellcheck disable=SC1090
source "$ENV_PATH"
set +a

if [[ -z "${RSPP_LIVEKIT_URL:-}" || -z "${RSPP_LIVEKIT_API_KEY:-}" || -z "${RSPP_LIVEKIT_API_SECRET:-}" ]]; then
  echo "validate: required RSPP_LIVEKIT_* variables are missing in $ENV_PATH" >&2
  exit 1
fi
if [[ -z "${RSPP_LIVEKIT_ROOM:-}" ]]; then
  RSPP_LIVEKIT_ROOM="rspp-smoke-room"
fi

cd "$REPO_ROOT"
go run ./cmd/rspp-runtime livekit -dry-run=false -probe=true -room "$RSPP_LIVEKIT_ROOM"

if [[ ! -f "$REPORT_PATH" ]]; then
  echo "validate: expected report not found at $REPORT_PATH" >&2
  exit 1
fi

echo "validate: probe succeeded and report written to $REPORT_PATH"
echo "validate: on failure, inspect server logs with:"
echo "  docker compose -f $COMPOSE_PATH logs --no-color livekit"
