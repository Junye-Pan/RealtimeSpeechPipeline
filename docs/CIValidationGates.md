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

Required evidence content for live chain reports:
1. provider combination selection and cap metadata
2. per-step input/output snapshots (subject to capture-mode redaction policy)
3. per-attempt latency/chunk metrics and streaming-used flags
4. fail/skip reason detail when modality or provider requirements are not met

## 5. Live end-to-end latency standard (phase-in policy)

For streaming-handoff rollout, live provider gates must emit end-to-end latency metrics defined by `internal/tooling/tooling_and_gates_guide.md`.

Policy:
1. Phase 1: metrics required in artifacts, non-blocking for merge.
2. Phase 2: thresholds promoted to blocking once matrix stability criteria are met.

## 5.1 Streaming vs non-streaming fairness policy (planned)

Before using live A/B comparisons for gate promotion decisions, artifacts must prove comparison fairness:
1. Same provider combination and same input snapshot identity across compared runs.
2. Explicit execution mode labeling (`streaming` vs `non_streaming`).
3. Explicit semantic parity marker:
   - `true`: comparison is gate-valid
   - `false`: comparison is diagnostic-only
4. Invalid-comparison reason recorded when fairness checks fail.

Planned promotion rule:
1. Latency threshold promotion may rely on A/B deltas only when fairness checks pass.
2. Missing fairness markers keeps live-provider comparisons non-blocking.

## 6. Replay gate fail policy (repo-level)

Replay gates fail on unexplained or disallowed divergence outcomes, including authority divergence and disallowed timing/order conditions.

Detailed divergence classification logic is canonical in `internal/tooling/tooling_and_gates_guide.md`.

## 7. Branch protection policy

Required status checks for merge/promotion should include:

1. `codex-artifact-policy`
2. `verify-quick`
3. `verify-full`
4. `security-baseline`

Optional live gates remain non-required until environment stability/security posture requires promotion.

## 8. Canonical detail pointers

- Command behavior: `cmd/command_entrypoints_guide.md`
- Tooling gate implementation: `internal/tooling/tooling_and_gates_guide.md`
- Test and suite mapping: `test/test_suite_guide.md`
- Transport/provider smoke context: `transports/transport_adapter_guide.md`, `providers/provider_adapter_guide.md`
