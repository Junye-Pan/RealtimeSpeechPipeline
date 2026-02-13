# CI Validation Gates (Repository-Level Policy)

## 1. Purpose

Define repository-wide verification policy and gate composition for pull requests and protected-branch promotion.

Module-level test details are documented in domain guide files.

## 2. Gate tiers

### Blocking gates

| Gate | Command | Intent |
| --- | --- | --- |
| `codex-artifact-policy` | `make codex-artifact-policy-check` | Enforce tracked `.codex` path policy |
| `verify-quick` | `make verify-quick` | Fast correctness baseline: contracts + replay smoke + SLO + targeted tests |
| `verify-full` | `make verify-full` | Full regression: contracts + replay regression + SLO + full test suite |
| `security-baseline` | `make security-baseline-check` | Security/data-handling baseline enforcement |

### Conditional blocking gate (MVP promotion)

| Gate | Command | Intent |
| --- | --- | --- |
| `verify-mvp` | `make verify-mvp` | Live-enforced MVP readiness: baseline gates + live end-to-end + fixed-decision checks |

### Optional non-blocking gates

| Gate | Command | Intent |
| --- | --- | --- |
| `live-provider-smoke` | `make live-provider-smoke` | Real provider integration smoke checks, including chained `STT->LLM->TTS` workflow evidence |
| `livekit-smoke` | `make livekit-smoke` | Real LiveKit connectivity/adapter smoke checks |
| `a2-runtime-live` | `make a2-runtime-live` | Extended live runtime/provider matrix |

## 3. Verify entrypoint convention

`bash scripts/verify.sh <mode>` is the standard wrapper used by CI.

- `quick` mode should execute `make verify-quick` equivalent command chain.
- `full` mode should execute `make verify-full` equivalent command chain.
- `mvp` mode should execute `make verify-mvp` equivalent command chain.

## 4. Required artifacts by gate family

| Family | Artifact roots |
| --- | --- |
| Contract/release ops | `.codex/ops/` |
| Replay outputs | `.codex/replay/` |
| Provider/live smoke outputs | `.codex/providers/` |

Artifacts under `.codex/` outside allowlisted tracked paths are generated outputs and must stay untracked.

Live chain report artifacts:
- `.codex/providers/live-provider-chain-report.json`
- `.codex/providers/live-provider-chain-report.md`
- `.codex/providers/live-provider-chain-report.streaming.json`
- `.codex/providers/live-provider-chain-report.streaming.md`
- `.codex/providers/live-provider-chain-report.nonstreaming.json`
- `.codex/providers/live-provider-chain-report.nonstreaming.md`
- `.codex/providers/live-latency-compare.json`
- `.codex/providers/live-latency-compare.md`

MVP SLO report artifacts:
- `.codex/ops/slo-gates-report.json`
- `.codex/ops/slo-gates-mvp-report.json`

Required evidence content for live chain reports:
1. provider combination selection and cap metadata
2. per-step input/output snapshots (subject to capture-mode redaction policy)
3. per-attempt latency/chunk metrics and streaming-used flags
4. fail/skip reason detail when modality or provider requirements are not met
5. effective execution controls:
   - effective handoff policy values (`enabled`, edge toggles, partial trigger thresholds)
   - effective provider streaming posture (`enable_streaming`, `disable_streaming`)
6. mode-proof checks:
   - streaming runs must include overlap evidence (`handoffs` and/or non-zero `first_assistant_audio_e2e_latency_ms`)
   - non-streaming runs must not record `streaming_used=true` attempts

## 5. Live end-to-end latency standard (MVP policy)

For MVP-mode validation (`verify-mvp`), live provider artifacts must satisfy end-to-end thresholds and fixed-decision checks.

Policy:
1. Baseline SLO gates remain mandatory and deterministic.
2. Live `STT->LLM->TTS` completion latencies are evaluated against PRD gate #7 (`P50<=1500ms`, `P95<=2000ms`).
3. A latency waiver is allowed only after multiple measured attempts; waiver reason and attempt files must be recorded in `.codex/ops/slo-gates-mvp-report.json`.

## 5.1 Streaming vs non-streaming fairness policy

Live A/B comparisons are gate-valid only when artifacts prove comparison fairness:
1. Same provider combination and same input snapshot identity across compared runs.
2. Explicit execution mode labeling (`streaming` vs `non_streaming`).
3. Explicit semantic parity marker:
   - `true`: comparison is gate-valid
   - `false`: comparison is diagnostic-only
4. Invalid-comparison reason recorded when fairness checks fail.

Gate rule:
1. MVP live verification fails when parity markers are missing or parity comparison validity is false.
2. Missing streaming/non-streaming counterpart evidence fails MVP live verification.
3. Missing or invalid `live-latency-compare` artifact fails MVP live verification.

## 5.2 Implementation progress snapshot (2026-02-13)

1. Mode-proof evidence checks are implemented in live-chain reporting:
   - streaming pass requires overlap proof
   - non-streaming pass requires no `streaming_used=true` attempts
2. Runtime invocation path now supports explicit provider-streaming disable in non-streaming mode.
3. MVP gate execution includes compare artifact generation in the `verify-mvp` command path.
4. Current sampled live compare result remains diagnostic-negative for streaming on the tested combo (`stt-assemblyai|llm-anthropic|tts-elevenlabs`), so further latency optimization work remains in scope.

## 6. Replay gate fail policy (repo-level)

Replay gates fail on unexplained or disallowed divergence outcomes, including authority divergence and disallowed timing/order conditions.

Detailed divergence classification logic is canonical in `internal/tooling/tooling_and_gates_guide.md`.

## 7. Branch protection policy

Required status checks for merge/promotion should include:

1. `codex-artifact-policy`
2. `verify-quick`
3. `verify-full`
4. `security-baseline`

For MVP promotion branches/releases, include:

1. `verify-mvp`

Optional live gates remain non-required until environment stability/security posture requires promotion.

## 8. Canonical detail pointers

- Command behavior: `cmd/command_entrypoints_guide.md`
- Tooling gate implementation: `internal/tooling/tooling_and_gates_guide.md`
- Test and suite mapping: `test/test_suite_guide.md`
- Transport/provider smoke context: `transports/transport_adapter_guide.md`, `providers/provider_adapter_guide.md`
