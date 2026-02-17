#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT_DIR"

bash scripts/prepare-mvp-live-artifacts.sh
make verify-quick
make verify-full
make verify-mvp
make security-baseline-check

echo "mvp_gate_runner: PASS"
