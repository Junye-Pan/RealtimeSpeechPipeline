# Internal Tooling Guide

This folder contains DevEx tooling for contract validation, replay regression policy, SLO gate evaluation, release readiness, and authoring scaffolds.

PRD alignment scope:
- `docs/PRD.md` section 3.2 (spec-first CI/CD and release flow)
- `docs/PRD.md` section 5 (MVP quality gates and fixed MVP decisions)

## Ownership and module map

| Module | Path | Primary owner | Cross-review | Status |
| --- | --- | --- | --- | --- |
| `DX-02` Validation Harness | `internal/tooling/validation` | `DevEx-Team` | Runtime-Team | implemented baseline |
| `DX-03` Replay Regression | `internal/tooling/regression` | `DevEx-Team` | ObsReplay-Team | implemented baseline |
| `DX-04` Release Workflow | `internal/tooling/release` | `DevEx-Team` | CP-Team | implemented baseline |
| `DX-05` Ops SLO Pack | `internal/tooling/ops` | `DevEx-Team` | Runtime-Team | implemented baseline |
| `DX-06` Node Authoring Kit | `internal/tooling/authoring` | `DevEx-Team` | Runtime-Team | scaffolded |

`cmd/rspp-cli` is the primary command surface for this folder.

## Gate behavior implemented by this folder

Quick path (`make verify-quick`):
- contract artifact validation and report generation
- replay smoke report generation
- runtime baseline generation and SLO gate reporting
- targeted package tests across API/runtime/observability/tooling packages
- full `./test/integration` package coverage and targeted failover smoke coverage (`TestF1`, `TestF3`, `TestF7`)

Full path (`make verify-full`):
- all quick checks
- replay regression report with full fixture set
- full `go test ./...` coverage

MVP path (`make verify-mvp`):
- all full checks
- live-enforced MVP SLO report generation (`rspp-cli slo-gates-mvp-report`)
- fails closed when required live artifacts are missing/invalid or fixed-decision gates are violated

Security path (`make security-baseline-check`):
- focused security baseline package tests via `scripts/security-check.sh`:
  - `./api/eventabi`
  - `./api/observability`
  - `./internal/security/policy`
  - `./internal/runtime/transport`
  - `./internal/observability/replay`
  - `./internal/observability/timeline`

## Validation harness (`DX-02`)

Validates fixture and artifact shapes for:
- `event`
- `control_signal`
- `turn_transition`
- `resolved_turn_plan`
- `decision_outcome`

Validation policy:
- valid fixtures must pass typed and schema validators
- invalid fixtures must fail typed and schema validators

PRD alignment note:
- PRD expects spec-first CI/CD with spec lint/validation plus replay tests before deploy.
- Current `DX-02` scope validates shared contract artifacts only (`event`, `control_signal`, `turn_transition`, `resolved_turn_plan`, `decision_outcome`).
- Dedicated `PipelineSpec` lint/validation gate coverage is not implemented yet in this folder.

## Replay regression policy (`DX-03`)

Fail conditions:
- unexplained `PLAN_DIVERGENCE` or `OUTCOME_DIVERGENCE`
- any `AUTHORITY_DIVERGENCE`
- unapproved `ORDERING_DIVERGENCE`
- invocation-latency timing-scope breaches

Replay fixture metadata file:
- `test/replay/fixtures/metadata.json`

Metadata fields used by tooling:
- `gate`
- `timing_tolerance_ms`
- `expected_divergences`
- `final_attempt_latency_threshold_ms`
- `total_invocation_latency_threshold_ms`
- `invocation_latency_scopes`

## Ops SLO defaults (`DX-05`)

Default thresholds:
- turn-open p95 `<= 120ms`
- first-output p95 `<= 1500ms`
- cancel-fence p95 `<= 150ms`
- end-to-end p50 `<= 1500ms`
- end-to-end p95 `<= 2000ms`
- baseline completeness ratio `== 1.0`
- stale accepted outputs `== 0`
- terminal lifecycle correctness ratio `== 1.0`

MVP-mode live enforcement:
1. `rspp-cli slo-gates-mvp-report` combines baseline SLO gates with live chain evidence and fixed-decision checks.
2. Live end-to-end threshold failures may be waived only after multiple attempts, and the waiver is recorded in the report.
3. Fixed-decision checks fail on missing parity markers/invalid parity, unsupported provider policy, non-simple mode behavior, or missing LiveKit readiness evidence.

## PRD MVP quality-gate alignment status

| PRD MVP gate | Target | Status in tooling |
| --- | --- | --- |
| Turn-open decision latency | p95 `<= 120ms` | implemented in `DX-05` |
| First assistant output latency | p95 `<= 1500ms` | implemented in `DX-05` |
| Cancellation fence latency | p95 `<= 150ms` | implemented in `DX-05` |
| Authority safety | stale accepted outputs `== 0` | implemented in `DX-05` |
| Replay baseline completeness | OR-02 completeness `== 1.0` | implemented in `DX-05` |
| Terminal lifecycle correctness | exactly one terminal (`commit` or `abort`) then `close` | implemented in `DX-05` |
| End-to-end latency | `P50 <= 1.5s`, `P95 <= 2s` | implemented in `DX-05` and enforced in MVP live mode |

## Live end-to-end latency diagnostics (current status + planned expansion)

Current implementation status in tooling:
- `rspp-cli live-latency-compare-report` evaluates paired streaming/non-streaming artifacts and emits per-combo plus aggregate first-audio/completion deltas.
- `rspp-cli slo-gates-mvp-report` consumes live provider-chain evidence and fixed-decision checks, including mode-proof validation.
- Blocking live checks continue to enforce completion thresholds (`p50 <= 1500ms`, `p95 <= 2000ms`) plus fixed-decision checks.

Currently emitted/consumed live metric:

| Metric | Current gate usage | Artifact field |
| --- | --- | --- |
| End-to-end first assistant audio latency | compare artifact + diagnostic summary | `first_assistant_audio_e2e_latency_ms` |
| End-to-end turn completion latency | blocking in MVP live mode (`p50`, `p95`) | `turn_completion_e2e_latency_ms` |

Planned per-stage streaming diagnostics (not yet emitted/consumed by tooling artifacts):

| Planned metric | Planned target | Planned scope |
| --- | --- | --- |
| STT first partial latency | `<= 450ms` | first user audio frame -> first STT stream chunk |
| STT partial to LLM start handoff latency | `<= 120ms` | eligible STT partial accepted -> LLM invoke start |
| LLM first partial latency | `<= 700ms` | LLM invoke start -> first LLM stream chunk |
| LLM partial to TTS start handoff latency | `<= 120ms` | eligible LLM partial accepted -> TTS invoke start |
| TTS first audio latency | `<= 650ms` | TTS invoke start -> first audio chunk |
| End-to-end first assistant audio latency | `<= 1800ms` | first user audio frame -> first assistant audio chunk |

Gate rollout status:
1. Current blocking gate scope uses end-to-end turn completion latency plus fixed-decision checks.
2. Per-stage streaming metrics are planned and remain non-blocking until artifact schema and gate ingestion support are implemented.
3. Baseline SLO gates remain active in parallel with live-enforced MVP mode.
4. `verify-mvp` now requires `live-latency-compare-report` generation to succeed before MVP SLO gating.

Latest implementation progress snapshot (2026-02-13):
1. Mode-correctness and compare-report pipeline are fully wired and enforced in `verify-mvp`.
2. Live smoke workflow requires provider enable toggles (`RSPP_*_ENABLE=1`) in addition to provider API-key env vars to avoid skip-only artifacts.
3. Sampled paired live evidence (`stt-assemblyai|llm-anthropic|tts-elevenlabs`, combo cap 1, AssemblyAI poll interval override `250ms`) currently shows:
   - first-audio delta (streaming - non-streaming): `+3969ms`
   - completion delta (streaming - non-streaming): `+4925ms`
4. Stage-level evidence in that sample points to STT latency as the dominant regression contributor.

## Live E2E latency improvement tracking (planned phases)

The following execution phases track the five prioritized improvements:
1. Phase A: handoff trigger correctness
   - remove duplicate downstream starts from fallback/supersede overlap paths
   - require handoff action lineage in artifacts
2. Phase B: text-only partial trigger gating
   - ensure metadata-only chunks do not open partial handoff windows
   - require trigger-cause evidence in chain reports
3. Phase C: STT latency tuning controls
   - add bounded provider cadence tuning knobs (for example AssemblyAI poll interval control)
   - require effective config echo in artifacts
4. Phase D: native STT streaming rollout (all STT providers)
   - upgrade STT adapters to provider-native incremental streaming paths
   - require streaming mode confirmation per attempt
5. Phase E: fair streaming/non-streaming parity gate
   - add semantic parity markers and comparison validity checks
   - only promote A/B latency deltas to gate decisions when parity is true

Promotion readiness criteria:
1. Stable pass trend across provider matrix runs with parity-valid comparisons.
2. No unexplained divergence in replay/timeline evidence for handoff paths.
3. CI live artifact completeness for required comparison markers.

## MVP fixed-decision alignment (PRD 5.2)

Current tooling alignment to PRD fixed MVP decisions:
1. LiveKit-only transport path: enforced in MVP live mode through required passing LiveKit smoke report.
2. Single-region authority with lease/epoch checks: represented by stale-epoch accepted output safety gate (`== 0`) in `DX-05`.
3. Simple mode only: enforced by normalizer/registry-based validation in MVP live mode.
4. OR-02 baseline replay evidence mandatory: enforced by completeness gate (`== 1.0`) in `DX-05`.
5. Streaming/non-streaming parity: required parity markers and valid parity comparison in MVP live mode.

## Spec-first CI/CD and release alignment (PRD 3.2)

1. PRD target: lint/validate pipeline specs and run replay tests before deploy.
2. Implemented in this folder:
   - replay smoke/regression gating (`DX-03`)
   - release readiness artifact checks and deterministic release manifest generation (`DX-04`)
3. Not implemented yet in this folder:
   - dedicated `PipelineSpec` lint/validation gate under `DX-02`/`DX-04` (current readiness checks consume gate artifacts and rollout config, but do not parse/validate pipeline specs directly)

## Release readiness (`DX-04`)

Release publish fails closed when required artifacts are missing, stale, or failing:
- contracts report
- replay regression report
- SLO gates report

## Authoring scaffolds (`DX-06`)

`internal/tooling/authoring` reserves scaffolds/tests for future node authoring workflows.
Current rule: no runtime contract bypasses; any generated scaffolds must validate through `DX-02` and replay gates.

## Test and gate touchpoints

- `go test ./internal/tooling/regression ./internal/tooling/ops ./internal/tooling/release` (via `make verify-quick`)
- `go run ./cmd/rspp-cli ...` command surface (via `make verify-quick` and `make verify-full`)
- `make verify-quick`
- `make verify-full`
- `make verify-mvp`

## Change checklist

1. Keep artifact schemas stable unless intentionally versioned.
2. Update tests for every policy branch change.
3. Keep replay fail-policy behavior explicit and deterministic.
4. Re-run CLI and tooling tests for command-surface changes.

## Related repo-level docs

- `docs/repository_docs_index.md`
- `docs/CIValidationGates.md`
- `docs/ContractArtifacts.schema.json`
- `docs/SecurityDataHandlingBaseline.md`
