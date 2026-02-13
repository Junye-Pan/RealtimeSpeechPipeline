# Realtime Speech Pipeline: Local MVP Validation

This runbook validates the MVP standards locally for:

- Go implementation baseline
- LiveKit-only transport path (`RK-22` / `RK-23`)
- Simple mode defaults (`ExecutionProfile=simple`)
- Single-region authority lease/epoch safety
- OR-02 baseline replay evidence requirements
- Live provider checks using only providers with API keys in `.env`

## MVP baseline decisions and scope

### Fixed decisions

- language/runtime baseline: Go runtime + control-plane implementation
- transport baseline: LiveKit path for `RK-22`/`RK-23`
- profile baseline: `ExecutionProfile=simple`
- replay baseline: OR-02 accepted-turn evidence is mandatory
- authority baseline: single-authority lease/epoch safety

### In scope for this runbook

- deterministic gate execution (`verify-quick`, `verify-full`, `security-baseline-check`)
- LiveKit transport smoke path
- live-provider smoke and A2 runtime live validations

### Out of scope for this runbook

- non-LiveKit production transport implementations
- non-baseline profile expansion work
- long-term release planning and roadmap tracking

## 1. Prerequisites

Run from repo root:

```bash
go version
docker --version
docker compose version
```

## 2. Load `.env` and enable only keyed providers

Load your `.env`:

```bash
set -a
source .env
set +a
```

Enable only providers backed by API keys in `.env` (and explicitly keep Google STT + Gemini off for now):

```bash
export RSPP_LIVE_PROVIDER_SMOKE=1

export RSPP_STT_DEEPGRAM_ENABLE=$([ -n "${RSPP_STT_DEEPGRAM_API_KEY:-}" ] && echo 1 || echo 0)
export RSPP_STT_ASSEMBLYAI_ENABLE=$([ -n "${RSPP_STT_ASSEMBLYAI_API_KEY:-}" ] && echo 1 || echo 0)
export RSPP_STT_GOOGLE_ENABLE=0
# AssemblyAI requires speech_models; default is universal-2
export RSPP_STT_ASSEMBLYAI_SPEECH_MODELS="${RSPP_STT_ASSEMBLYAI_SPEECH_MODELS:-universal-2}"

export RSPP_LLM_ANTHROPIC_ENABLE=$([ -n "${RSPP_LLM_ANTHROPIC_API_KEY:-}" ] && echo 1 || echo 0)
export RSPP_LLM_COHERE_ENABLE=$([ -n "${RSPP_LLM_COHERE_API_KEY:-}" ] && echo 1 || echo 0)
export RSPP_LLM_GEMINI_ENABLE=0

export RSPP_LLM_COHERE_OPENROUTER=1
export RSPP_LLM_COHERE_MODEL="${COHERE_MODEL:-${RSPP_LLM_COHERE_MODEL:-cohere/command-r-08-2024}}"

export RSPP_TTS_ELEVENLABS_ENABLE=$([ -n "${RSPP_TTS_ELEVENLABS_API_KEY:-}" ] && echo 1 || echo 0)
export RSPP_TTS_GOOGLE_ENABLE=0
export RSPP_TTS_POLLY_ENABLE=0

# Optional: cap chained STT->LLM->TTS combos (default 12)
export RSPP_LIVE_PROVIDER_CHAIN_MAX_COMBOS="${RSPP_LIVE_PROVIDER_CHAIN_MAX_COMBOS:-12}"
# Optional: provider invocation payload capture mode for OR-02 evidence (redacted|full|hash)
export RSPP_PROVIDER_IO_CAPTURE_MODE="${RSPP_PROVIDER_IO_CAPTURE_MODE:-redacted}"
# Optional: payload capture byte cap (minimum 256; default 8192)
export RSPP_PROVIDER_IO_CAPTURE_MAX_BYTES="${RSPP_PROVIDER_IO_CAPTURE_MAX_BYTES:-8192}"
# Streaming is default-on for adapters with native InvokeStream support.
# Optional emergency kill-switches:
# export RSPP_PROVIDER_STREAMING_DISABLE=1
# export RSPP_PROVIDER_STREAMING_STT_DISABLE=1
# export RSPP_PROVIDER_STREAMING_LLM_DISABLE=1
# export RSPP_PROVIDER_STREAMING_TTS_DISABLE=1
# Optional artifact overrides
# export RSPP_LIVE_PROVIDER_CHAIN_REPORT_PATH=.codex/providers/live-provider-chain-report.json
# export RSPP_LIVE_PROVIDER_CHAIN_REPORT_MD_PATH=.codex/providers/live-provider-chain-report.md
```

## 3. Start local LiveKit and validate transport probe

```bash
make livekit-local-up
set -a
source ops/livekit-local/.env.generated
set +a
make livekit-local-validate
```

## 4. Run deterministic MVP gates (blocking)

```bash
make verify-quick
make verify-full
make security-baseline-check
```

These cover contract conformance, replay regression, SLO gates, simple-mode enforcement, authority lease/epoch paths, and OR-02 baseline checks.

## 5. Run LiveKit smoke (real transport lane)

```bash
RSPP_LIVEKIT_SMOKE=1 make livekit-smoke
```

## 6. Run live-provider validation with keyed providers only

```bash
make live-provider-smoke
make a2-runtime-live
```

`live-provider-smoke` now validates:

- per-provider live invoke success
- switch/fallback routing behavior
- chained STT->LLM->TTS logical workflow across enabled provider combinations (deterministic cap)
- adapter-native `InvokeStream` path usage (default-on when provider supports it)

Included live-provider tests:

- `TestLiveProviderSmoke`
- `TestLiveProviderSmokeSwitchAndFallbackRouting`
- `TestLiveProviderSmokeChainedWorkflow`

Run only the chained workflow test directly:

```bash
RSPP_LIVE_PROVIDER_SMOKE=1 go test -tags=liveproviders ./test/integration -run TestLiveProviderSmokeChainedWorkflow -v
```

Chained workflow report fields now include OR-02 provider invocation payload evidence (`raw_input_payload`, `raw_output_payload`, status/truncation metadata) per STT/LLM/TTS step.
The report also includes per-step streaming latency/chunk metrics (`attempt_latency_ms`, `first_chunk_latency_ms`, `chunk_count`, `bytes_out`, `streaming_used`).

Chained STT seed inputs are provider-configured audio source values and are recorded in the chain report:

- `stt-deepgram`: `RSPP_STT_DEEPGRAM_AUDIO_URL` (default deepgram sample WAV URL)
- `stt-assemblyai`: `RSPP_STT_ASSEMBLYAI_AUDIO_URL` (default deepgram sample WAV URL)
- `stt-google`: `RSPP_STT_GOOGLE_AUDIO_URI` (default Google sample `gs://` URI)

`a2-runtime-live` runs strict scenario validation but allows unenabled providers to remain skipped; enabled providers must pass.

## 7. Artifacts to inspect

- `.codex/replay/smoke-report.json`
- `.codex/replay/regression-report.json`
- `.codex/replay/runtime-baseline.json`
- `.codex/ops/slo-gates-report.json`
- `.codex/ops/security-baseline-report.json`
- `.codex/transports/livekit-report.json`
- `.codex/providers/livekit-smoke-report.json`
- `.codex/providers/live-provider-chain-report.json`
- `.codex/providers/live-provider-chain-report.md`
- `.codex/providers/a2-runtime-live-report.json`
- `.codex/providers/a2-runtime-live-report.md`

## 8. Expected pass criteria

- `verify-quick`, `verify-full`, and `security-baseline-check` exit `0`.
- `livekit-smoke` passes against local LiveKit.
- `live-provider-smoke` passes for enabled providers; chained workflow skips only when any modality is fully unconfigured.
- `a2-runtime-live` exits `0` with no scenario failures.
- SLO report shows OR-02 baseline completeness satisfied and stale-epoch accepted outputs at `0`.

## 9. Teardown

```bash
make livekit-local-down
```
