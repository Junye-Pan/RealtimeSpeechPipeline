# Test Suite Guide

This folder contains contract, integration, replay, failover, and load suites that enforce deterministic runtime/control-plane behavior and CI gate policy.

## Ownership

- Primary owner: `DevEx-Team` for gate wiring and artifact policy
- Domain owners co-own behavior assertions:
  - `Runtime-Team` for lifecycle, cancellation, authority, buffering
  - `CP-Team` for control-plane outcomes and plan provenance
  - `ObsReplay-Team` for replay evidence/divergence behavior
  - `Transport-Team` for transport-path conformance
  - `Provider-Team` for provider invocation behavior

## Folder map

| Path | Purpose |
| --- | --- |
| `test/contract` | schema + typed contract conformance for valid and invalid fixtures |
| `test/integration` | runtime/control-plane/provider/transport integration behavior |
| `test/replay` | replay determinism and divergence policy behavior |
| `test/failover` | failure-injection coverage for `F1`-`F8` classes |
| `test/load` | latency/backpressure/cancel stress suites scaffold |

## Conformance ID families

- `CT-*`: contract artifacts and schema/typed validation
- `RD-*`: replay determinism and divergence policy
- `CF-*`: cancellation fencing behavior
- `AE-*`: authority epoch safety behavior
- `ML-*`: merge/drop lineage behavior
- `SH-*`: orchestration streaming handoff behavior (`STT->LLM`, `LLM->TTS`)
- `LK-*`: LiveKit transport closure behavior

## Implemented baseline matrix summary

Contract (`CT`):
- `CT-001` schema and fixture validation
- `CT-002` control-signal ownership validation
- `CT-003` legal lifecycle transition validation
- `CT-004` deterministic `ResolvedTurnPlan` freeze/provenance validation
- `CT-005` decision outcome emitter/phase/scope constraints

Replay (`RD`):
- `RD-001` smoke no-divergence baseline
- `RD-002` timing tolerance behavior
- `RD-003` OR-02 completeness behavior
- `RD-004` snapshot provenance divergence classification
- `RD-005` invocation latency threshold enforcement (`not implemented` as a dedicated `RD-005` replay fixture/test ID)

Runtime behavior suites:
- `CF-001`, `CF-002`, `CF-003`, `CF-004`
- `AE-001`, `AE-004`, `AE-005`
- `AE-002`, `AE-003` (`not implemented` as dedicated `AE-*` suites)
- `ML-001`, `ML-002`, `ML-003`, `ML-004`
- LiveKit transport coverage mapped to `LK` family scope (`LK-001`..`LK-003`) via transport/integration/command tests; dedicated `LK-*` test function IDs are not used in current test names.

Streaming handoff coverage (implemented baseline + planned expansion):
- Implemented baseline:
  - `internal/runtime/executor/streaming_handoff_test.go` verifies overlap start ordering (`STT->LLM`, `LLM->TTS`) and sequential fallback when policy is disabled.
  - `internal/runtime/executor/streaming_handoff_test.go` verifies handoff queue saturation/recovery signaling (`flow_xoff`/`flow_xon`) on `stt_to_llm` and `llm_to_tts` edges.
  - `test/integration/provider_live_smoke_test.go` (`TestLiveProviderSmokeChainedWorkflow`) executes live chain combinations and records handoff/latency evidence.
  - `test/integration/provider_live_smoke_test.go` writes streaming/non-streaming artifacts with parity markers and comparison identity for MVP gate validity.
- Planned expansion:
  - `SH-003`: upstream revision supersedes downstream deterministic restart behavior
  - `SH-005`: cancel/authority-loss fencing across active handoff windows

Representative evidence paths:
- `CT-001`..`CT-005`: `test/contract/*`, `api/*/types_test.go`, `internal/tooling/validation/*`
- CP backend parity/fallback/stale determinism coverage: `test/integration/runtime_chain_test.go` (currently not tracked as a dedicated `CT-006` fixture/test ID)
- `RD-*`: `test/replay/*`, `internal/tooling/regression/*`, `cmd/rspp-cli` replay reports
- `CF-*`, `AE-*`, `ML-*`: `test/integration/*`, `test/failover/*`, runtime package tests
- `LK` family coverage: `transports/livekit/*_test.go`, `test/integration/livekit_transport_integration_test.go`, `test/integration/livekit_live_smoke_test.go`, `cmd/*livekit*` tests

## Replay fixture metadata policy

Metadata file:
- `test/replay/fixtures/metadata.json`

Fields used by regression tooling:
- `gate` (`quick`, `full`, `both`)
- `timing_tolerance_ms`
- `expected_divergences`
- `final_attempt_latency_threshold_ms`
- `total_invocation_latency_threshold_ms`
- `invocation_latency_scopes`

Policy invariants:
1. `AUTHORITY_DIVERGENCE` always fails.
2. `PLAN_DIVERGENCE` and `OUTCOME_DIVERGENCE` fail unless explicitly expected.
3. `ORDERING_DIVERGENCE` requires explicit approval in metadata.
4. Invocation-latency timing scopes are hard-fail conditions.

## Failure injection execution rules

Execution sequencing:
1. Inject one primary failure class per run.
2. For authority/migration tests, include ingress and egress epoch mismatch checks.
3. For pressure tests, include queue-depth and time-in-queue variants.
4. For cancel-sensitive tests, assert output fencing before terminal assertions.

Required assertions per run:
1. Deterministic outcomes for identical input/seed/snapshot conditions.
2. Accepted turns preserve single terminal sequence (`commit` or `abort`, then `close`).
3. Pre-turn outcomes do not emit `abort`/`close`.
4. OR-02 evidence includes lifecycle/authority/provenance markers for the injected class.
5. Replay regression contains no unexplained plan/outcome divergences.
6. `AUTHORITY_DIVERGENCE` remains zero.
7. `ORDERING_DIVERGENCE` is only allowed when explicitly approved.
8. `TIMING_DIVERGENCE` must stay within configured tolerance.
9. Streaming handoff sequence is monotonic and revision-consistent.
10. Slow-stage policy actions are present in evidence when thresholds trigger.

## Failure coverage policy

Quick/smoke set:
- `F1`, `F3`, `F7`

Full set:
- `F1` through `F8`

## Gate mapping

- `make verify-quick`: contract/integration/replay suites + failover smoke subset, plus baseline artifact/report generation (`validate-contracts-report`, `replay-smoke-report`, `generate-runtime-baseline`, `slo-gates-report`)
- `make verify-full`: full `go test ./...` plus replay regression and baseline artifact/report checks
- `make verify-mvp`: full baseline gates plus `live-latency-compare-report` and `slo-gates-mvp-report` live end-to-end/fixed-decision checks
- Optional non-blocking: live-provider smoke, LiveKit smoke, A2 runtime live matrix

Live-provider smoke chain evidence:
1. `.codex/providers/live-provider-chain-report.json` stores per-step inputs/outputs and per-attempt latency/chunk metrics.
2. `.codex/providers/live-provider-chain-report.md` provides operator-readable summary with pass/fail/skip and latency rollups.
3. `.codex/providers/live-provider-chain-report.streaming.json` and `.codex/providers/live-provider-chain-report.nonstreaming.json` preserve mode-specific evidence with parity markers.
4. `.codex/providers/live-provider-chain-report.streaming.md` and `.codex/providers/live-provider-chain-report.nonstreaming.md` provide mode-specific operator summaries.

## Streaming vs non-streaming compare protocol (implemented MVP baseline)

Use this protocol when evaluating latency impact of streaming handoff and provider streaming paths:
1. Keep provider combination fixed:
   - same STT/LLM/TTS IDs for compared runs
2. Keep workload fixed:
   - recommended operator practice: keep input snapshot fields and combination cap consistent
   - current compare-gate enforcement is based on mode markers and shared provider-combination identity
3. Record execution mode explicitly:
   - run A: `execution_mode=streaming`
   - run B: `execution_mode=non_streaming`
4. Verify semantic parity before comparing latency:
   - both runs must represent equivalent completion semantics
   - if parity is false, comparison is diagnostic-only and fails MVP fixed-decision gate validity
5. Report deltas on common metrics:
   - `first_assistant_audio_e2e_latency_ms`
   - `turn_completion_e2e_latency_ms`
   - handoff latency fields and attempt-level `streaming_used`
6. Include invalid-comparison reason in artifacts when parity or comparison-identity checks fail.
7. Generate compare artifact:
   - `go run ./cmd/rspp-cli live-latency-compare-report .codex/providers/live-latency-compare.json .codex/providers/live-provider-chain-report.streaming.json .codex/providers/live-provider-chain-report.nonstreaming.json`
8. Ensure provider enable toggles are present for selected providers (`RSPP_*_ENABLE=1`) in addition to API-key variables; otherwise live-chain artifacts may be skip-only and not comparable.

## Baseline exit criteria

1. `make verify-quick` passes.
2. `make verify-full` passes.
3. Required replay and SLO artifacts are generated in `.codex/`.
4. No unresolved contract ambiguity remains for baseline artifacts.

## Expansion backlog (post-baseline)

1. Extend replay regression latency-scope coverage as new fixtures are added.
2. Extend failover/provider live-coverage depth while preserving deterministic gate behavior.

## Recommended local commands

- `go test ./test/contract ./test/integration ./test/replay ./test/failover`
- `make verify-quick`
- `make verify-full`

## Related repo-level docs

- `docs/repository_docs_index.md`
- `docs/CIValidationGates.md`
- `docs/ContractArtifacts.schema.json`
- `docs/SecurityDataHandlingBaseline.md`
