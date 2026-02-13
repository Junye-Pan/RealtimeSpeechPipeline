#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  bash ops/livekit-local/scripts/bootstrap.sh [--rotate]

Options:
  --rotate    Regenerate API key/secret even if .env.generated already exists.
EOF
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "bootstrap: missing required command: $cmd" >&2
    exit 1
  fi
}

parse_env_var() {
  local name="$1"
  local file="$2"
  local value
  value="$(grep -E "^${name}=" "$file" 2>/dev/null | head -n1 | cut -d'=' -f2- || true)"
  printf '%s' "$value"
}

ROTATE=0
if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi
if [[ "${1:-}" == "--rotate" ]]; then
  ROTATE=1
elif [[ -n "${1:-}" ]]; then
  echo "bootstrap: unknown argument: $1" >&2
  usage
  exit 1
fi

require_cmd git
require_cmd docker
require_cmd openssl
require_cmd curl

if ! docker compose version >/dev/null 2>&1; then
  echo "bootstrap: docker compose plugin not found" >&2
  exit 1
fi

REPO_ROOT="$(git rev-parse --show-toplevel)"
OPS_DIR="$REPO_ROOT/ops/livekit-local"
TEMPLATE_PATH="$OPS_DIR/livekit.yaml.template"
CONFIG_PATH="$OPS_DIR/livekit.yaml"
ENV_PATH="$OPS_DIR/.env.generated"
COMPOSE_PATH="$OPS_DIR/docker-compose.yml"

if [[ ! -f "$TEMPLATE_PATH" ]]; then
  echo "bootstrap: missing template: $TEMPLATE_PATH" >&2
  exit 1
fi
if [[ ! -f "$COMPOSE_PATH" ]]; then
  echo "bootstrap: missing docker-compose file: $COMPOSE_PATH" >&2
  exit 1
fi

URL="http://127.0.0.1:7880"
ROOM="rspp-smoke-room"
KEY=""
SECRET=""

if [[ "$ROTATE" -eq 0 && -f "$ENV_PATH" ]]; then
  KEY="$(parse_env_var "RSPP_LIVEKIT_API_KEY" "$ENV_PATH")"
  SECRET="$(parse_env_var "RSPP_LIVEKIT_API_SECRET" "$ENV_PATH")"
  EXISTING_URL="$(parse_env_var "RSPP_LIVEKIT_URL" "$ENV_PATH")"
  EXISTING_ROOM="$(parse_env_var "RSPP_LIVEKIT_ROOM" "$ENV_PATH")"
  if [[ -n "$EXISTING_URL" ]]; then
    URL="$EXISTING_URL"
  fi
  if [[ -n "$EXISTING_ROOM" ]]; then
    ROOM="$EXISTING_ROOM"
  fi
fi

if [[ -z "$KEY" ]]; then
  KEY="rspp_local_$(openssl rand -hex 4)"
fi
if [[ -z "$SECRET" ]]; then
  SECRET="$(openssl rand -base64 48 | tr '+/' '-_' | tr -d '=' | cut -c1-64)"
fi

if [[ -z "$KEY" || -z "$SECRET" ]]; then
  echo "bootstrap: failed to generate livekit credentials" >&2
  exit 1
fi

ESCAPED_KEY="$(printf '%s' "$KEY" | sed 's/[\\/&]/\\&/g')"
ESCAPED_SECRET="$(printf '%s' "$SECRET" | sed 's/[\\/&]/\\&/g')"
sed \
  -e "s/__RSPP_LIVEKIT_API_KEY__/${ESCAPED_KEY}/g" \
  -e "s/__RSPP_LIVEKIT_API_SECRET__/${ESCAPED_SECRET}/g" \
  "$TEMPLATE_PATH" >"$CONFIG_PATH"

cat >"$ENV_PATH" <<EOF
RSPP_LIVEKIT_URL=${URL}
RSPP_LIVEKIT_API_KEY=${KEY}
RSPP_LIVEKIT_API_SECRET=${SECRET}
RSPP_LIVEKIT_ROOM=${ROOM}
EOF
chmod 600 "$ENV_PATH"

docker compose -f "$COMPOSE_PATH" up -d

READY=0
for _ in $(seq 1 30); do
  if curl -sS -o /dev/null -m 2 "$URL/"; then
    READY=1
    break
  fi
  sleep 2
done

if [[ "$READY" -ne 1 ]]; then
  echo "bootstrap: LiveKit did not become reachable at $URL within timeout" >&2
  echo "bootstrap: inspect logs with:" >&2
  echo "  docker compose -f ops/livekit-local/docker-compose.yml logs --no-color livekit" >&2
  exit 1
fi

echo "RSPP_LIVEKIT_URL=${URL}"
echo "RSPP_LIVEKIT_API_KEY=${KEY}"
echo "RSPP_LIVEKIT_API_SECRET=${SECRET}"
echo "RSPP_LIVEKIT_ROOM=${ROOM}"
