# Local Self-Hosted LiveKit Bootstrap

This folder provisions a local LiveKit server for RSPP transport smoke verification and outputs the required environment values:

- `RSPP_LIVEKIT_URL`
- `RSPP_LIVEKIT_API_KEY`
- `RSPP_LIVEKIT_API_SECRET`
- `RSPP_LIVEKIT_ROOM`

## Prerequisites

- Docker Engine/Desktop
- Docker Compose plugin (`docker compose version`)
- `git`, `openssl`, and `curl`

## Quickstart

From repo root:

```bash
bash ops/livekit-local/scripts/bootstrap.sh
```

Expected output (exact keys):

```text
RSPP_LIVEKIT_URL=...
RSPP_LIVEKIT_API_KEY=...
RSPP_LIVEKIT_API_SECRET=...
RSPP_LIVEKIT_ROOM=...
```

Validate connectivity and auth with the runtime probe:

```bash
bash ops/livekit-local/scripts/validate.sh
```

## Rotation and Lifecycle

Regenerate API key/secret:

```bash
bash ops/livekit-local/scripts/bootstrap.sh --rotate
```

Stop/remove local stack:

```bash
docker compose -f ops/livekit-local/docker-compose.yml down
```

## Make Targets

```bash
make livekit-local-up
make livekit-local-validate
make livekit-local-down
```

## CI Secret Setup (GitHub CLI)

After bootstrap prints values, set repository secrets:

```bash
gh secret set RSPP_LIVEKIT_URL --body "$RSPP_LIVEKIT_URL"
gh secret set RSPP_LIVEKIT_API_KEY --body "$RSPP_LIVEKIT_API_KEY"
gh secret set RSPP_LIVEKIT_API_SECRET --body "$RSPP_LIVEKIT_API_SECRET"
gh secret set RSPP_LIVEKIT_ROOM --body "$RSPP_LIVEKIT_ROOM"
```

If you did not export the variables, source the generated file first:

```bash
set -a
source ops/livekit-local/.env.generated
set +a
```

## Troubleshooting

1. Port conflict (`7880`, `7881/udp`, or `7882`)  
Check listeners and free the port, then rerun bootstrap.

2. Probe/auth failure in `validate.sh`  
Re-run bootstrap with `--rotate`, then re-run validate.

3. Server not reachable  
Inspect logs:

```bash
docker compose -f ops/livekit-local/docker-compose.yml logs --no-color livekit
```
