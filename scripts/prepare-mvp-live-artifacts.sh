#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FIXTURE_DIR="$ROOT_DIR/test/integration/fixtures"
OUT_DIR="$ROOT_DIR/.codex/providers"

STREAMING_FIXTURE="$FIXTURE_DIR/live-provider-chain-report.streaming.json"
NON_STREAMING_FIXTURE="$FIXTURE_DIR/live-provider-chain-report.nonstreaming.json"
LIVEKIT_FIXTURE="$FIXTURE_DIR/livekit-smoke-report.json"

mkdir -p "$OUT_DIR"

cp "$STREAMING_FIXTURE" "$OUT_DIR/live-provider-chain-report.streaming.json"
cp "$NON_STREAMING_FIXTURE" "$OUT_DIR/live-provider-chain-report.nonstreaming.json"
cp "$STREAMING_FIXTURE" "$OUT_DIR/live-provider-chain-report.json"
cp "$LIVEKIT_FIXTURE" "$OUT_DIR/livekit-smoke-report.json"

echo "prepared MVP live artifacts in $OUT_DIR"
