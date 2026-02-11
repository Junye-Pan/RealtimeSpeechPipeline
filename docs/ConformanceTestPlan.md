# Conformance Test Plan (Implemented Baseline)

## 1. Purpose and status

Define and track the implemented conformance baseline for contract correctness, deterministic replay behavior, cancellation fencing, authority safety, and merge/drop lineage guarantees.

Status snapshot:
- Baseline reflects repository behavior as of `2026-02-11`.
- This document is synchronized with current gate execution in `Makefile`, `scripts/verify.sh`, and `cmd/rspp-cli`.
- This document is synchronized with the current MVP section-10 closure state in `docs/MVP_ImplementationSlice.md` (`10.1.24` closed; `10.2` currently has no open items).
- CP promotion-to-implemented scope for `CP-01/02/03/04/05/07/08/09/10` is closed at MVP scope: module behavior + file/env/http parity + deterministic backend-failure fallback/stale handling + conformance evidence synchronization are now covered in baseline tests/docs.
- CP bundle provenance integration coverage includes partial-backend fallback and stale-snapshot deterministic pre-turn handling scenarios, CP-02 simple-mode normalization enforcement with unsupported-profile deterministic pre-turn handling, CP-03 graph compile output propagation, CP-05 pre-turn decision shaping (`CP-05` emitter outcomes), CP-07 lease-authority gating paths, and rollout/policy/provider-health fallback determinism under backend outages.
- OR-01 telemetry pipeline baseline coverage is implemented for non-blocking telemetry behavior, deterministic sampling, runtime env wiring, and runtime instrumentation paths (`turnarbiter`, `scheduler`, `provider invocation`, `transport fence`).
- DX-04 release/readiness baseline is implemented with artifact-backed release gate checks (`contracts-report`, replay regression, SLO gates) and deterministic publish manifest workflow coverage.
- OR-03 replay access path now includes distributed HTTP replay-audit backend coverage with deterministic JSONL fallback behavior, plus backend-resolver failure-mode assertions; retention-sweep coverage now includes CP distribution snapshot-first policy sourcing with deterministic outage fallback and recovery behavior.
- LiveKit transport-path closure evidence is implemented via `transports/livekit` deterministic adapter coverage, runtime/local-runner command wiring, and non-blocking real LiveKit smoke artifacts (`docs/LiveKitTransportClosure.md`).

## 2. Suite structure and current coverage

| Suite ID | Suite name | Current implementation status | Primary evidence |
| --- | --- | --- | --- |
| `CT` | Contract tests | implemented | `test/contract/*`, `api/controlplane/types_test.go`, `api/eventabi/types_test.go`, `internal/runtime/planresolver/resolver_test.go` |
| `RD` | Replay determinism tests | implemented | `test/replay/*`, `cmd/rspp-cli replay-smoke-report`, `cmd/rspp-cli replay-regression-report`, `test/replay/fixtures/metadata.json` |
| `CF` | Cancellation fencing tests | implemented | `test/integration/quick_conformance_test.go`, `test/integration/cf_full_conformance_test.go`, `internal/runtime/transport/fence_test.go` |
| `AE` | Authority epoch tests | implemented | `test/integration/quick_conformance_test.go`, `test/integration/ae004_enrichment_test.go`, `test/failover/failure_smoke_test.go`, `test/failover/failure_full_test.go` |
| `ML` | Merge/drop lineage tests | implemented | `internal/runtime/buffering/drop_notice_test.go`, `test/integration/ml_conformance_test.go`, replay fixture metadata |
| `LK` | LiveKit transport closure tests | implemented | `transports/livekit/*_test.go`, `test/integration/livekit_transport_integration_test.go`, `cmd/rspp-runtime/main_test.go`, `cmd/rspp-local-runner/main_test.go` |

## 3. Implemented test matrix

## 3.1 Contract tests (`CT`)

| Test ID | Current evidence | Gate participation | Pass criteria |
| --- | --- | --- | --- |
| `CT-001` | `test/contract/schema_validation_test.go`, `internal/tooling/validation/contracts.go`, `test/contract/fixtures/event/*` | quick + full | schema pass for valid fixtures and schema rejection for invalid fixtures |
| `CT-002` | `api/eventabi/types_test.go`, `test/contract/fixtures/control_signal/*` | quick + full | signal ownership mapping is enforced and unknown signals are rejected |
| `CT-003` | `test/contract/fixtures/turn_transition/*`, `internal/runtime/turnarbiter/arbiter_test.go`, `test/integration/runtime_chain_test.go` | quick + full | legal transitions accepted, illegal/pre-turn failure paths remain pre-turn with no invalid lifecycle emissions |
| `CT-004` | `internal/runtime/planresolver/resolver_test.go`, `test/contract/fixtures/resolved_turn_plan/*` | quick + full | frozen fields/provenance are present and deterministic for identical inputs |
| `CT-005` | `api/controlplane/types_test.go`, `test/contract/fixtures/decision_outcome/*`, `test/integration/runtime_chain_test.go` | quick + full | emitter/phase/scope constraints for decision outcomes are enforced |
| `CT-006` | `internal/controlplane/normalizer/normalizer_test.go`, `internal/runtime/turnarbiter/controlplane_bundle_test.go`, `internal/runtime/turnarbiter/controlplane_backends_test.go`, `internal/runtime/turnarbiter/arbiter_test.go`, `internal/controlplane/distribution/file_adapter_test.go`, `internal/controlplane/distribution/http_adapter_test.go`, `test/integration/runtime_chain_test.go` (`TestCPBackendRolloutPolicyProviderHealthFailuresFallBackDeterministically`, `TestCPBackendUnsupportedExecutionProfileTriggersDeterministicPreTurnHandling`) | quick + full | CP turn-start backend path preserves deterministic behavior under backend parity (`file`/`env`/`http`), partial-backend fallback, stale-snapshot handling, and CP-02 simple-mode normalization enforcement for promoted modules `CP-01/02/03/04/05/07/08/09/10`; includes deterministic unsupported-profile pre-turn handling, CP-03 graph compile propagation, CP-05 pre-turn reject/defer shaping, CP-07 lease-authority gating, and rollout/policy/provider-health fallback defaults under backend outages. |

## 3.2 Replay determinism tests (`RD`)

| Test ID | Current evidence | Gate participation | Pass criteria |
| --- | --- | --- | --- |
| `RD-001` | `test/replay/rd001_replay_smoke_test.go`, `cmd/rspp-cli replay-smoke-report` | quick + full tests, quick replay-smoke artifact | replay-decisions no-divergence baseline; smoke replay gate must have zero failing divergences |
| `RD-002` | `test/replay/rd002_rd003_rd004_test.go` | quick + full | recompute path timing divergence is tolerated only within fixture tolerance |
| `RD-003` | `test/replay/rd002_rd003_rd004_test.go`, `cmd/rspp-cli slo-gates-report` | quick + full | OR-02 completeness for accepted turns is enforced |
| `RD-004` | `test/replay/rd002_rd003_rd004_test.go`, replay metadata entry `rd-004-snapshot-provenance-plan` | quick + full tests, full replay-regression artifact | snapshot provenance mismatch yields `PLAN_DIVERGENCE` and must be explicitly expected in metadata |
| `RD-005` | `cmd/rspp-cli/main.go`, `cmd/rspp-cli/main_test.go`, `internal/tooling/regression/divergence_test.go`, replay metadata fields `final_attempt_latency_threshold_ms` + `total_invocation_latency_threshold_ms` + `invocation_latency_scopes` | full | invocation latency threshold breaches fail replay regression with deterministic timing-divergence scopes derived from runtime baseline artifact invocation outcomes (metadata scopes take precedence over fixture-id scope derivation) |

## 3.3 Cancellation fencing tests (`CF`)

| Test ID | Current evidence | Gate participation | Pass criteria |
| --- | --- | --- | --- |
| `CF-001` | `test/integration/quick_conformance_test.go` | quick + full | cancel during active generation yields deterministic fenced terminalization |
| `CF-002` | `test/integration/cf_full_conformance_test.go`, `internal/runtime/transport/fence_test.go` | quick + full | late provider/output after cancel is deterministically fenced/dropped |
| `CF-003` | `test/integration/cf_full_conformance_test.go`, `internal/runtime/turnarbiter/arbiter_test.go` | quick + full | cancel path emits exactly one terminal then `close` |
| `CF-004` | `test/integration/cf_full_conformance_test.go` | quick + full | OR-02 evidence includes cancel markers (`cancel_sent_at`, `cancel_accepted_at`, fence marker) |

## 3.4 Authority epoch tests (`AE`)

| Test ID | Current evidence | Gate participation | Pass criteria |
| --- | --- | --- | --- |
| `AE-001` | `test/integration/quick_conformance_test.go` | quick + full | pre-turn stale epoch emits `stale_epoch_reject` without `turn_open`/`abort`/`close` |
| `AE-002` | `test/failover/failure_smoke_test.go`, `internal/runtime/turnarbiter/arbiter_test.go` | quick + full | in-turn authority revoke emits deterministic `deauthorized_drain -> abort(authority_loss) -> close` and wins same-point cancel ties |
| `AE-003` | `test/failover/failure_full_test.go` | full | stale old-placement output is rejected without split-brain acceptance |
| `AE-004` | `test/integration/ae004_enrichment_test.go` | quick + full | missing transport authority metadata follows deterministic enrichment path |
| `AE-005` | `test/integration/quick_conformance_test.go` | quick + full | pre-turn deauthorization emits `deauthorized_drain` and prevents open/terminal sequence |

## 3.5 Merge/drop lineage tests (`ML`)

| Test ID | Current evidence | Gate participation | Pass criteria |
| --- | --- | --- | --- |
| `ML-001` | `internal/runtime/buffering/drop_notice_test.go` | quick + full | deterministic `drop_notice` contains valid range and reason |
| `ML-002` | `test/integration/ml_conformance_test.go` | quick + full | merge/coalesce lineage is deterministic and complete |
| `ML-003` | `test/integration/ml_conformance_test.go`, replay metadata entry `ml-003-replay-absence-classification` | quick + full tests, full replay-regression artifact | replay distinguishes deterministic drop from unexplained upstream absence |
| `ML-004` | `test/integration/ml_conformance_test.go` | quick + full | sync-domain discontinuity and reset markers are deterministic |

## 3.6 LiveKit transport closure tests (`LK`)

| Test ID | Current evidence | Gate participation | Pass criteria |
| --- | --- | --- | --- |
| `LK-001` | `transports/livekit/adapter_test.go`, `test/integration/livekit_transport_integration_test.go` | quick + full | RK-22 ingress classification + RK-22 output fencing + RK-23 connection lifecycle normalization are preserved through adapter flow. |
| `LK-002` | `cmd/rspp-runtime/main_test.go`, `cmd/rspp-local-runner/main_test.go`, `transports/livekit/command_test.go` | quick + full | operator-facing command surfaces (`rspp-runtime livekit`, `rspp-local-runner`) produce deterministic report artifacts and fail closed on invalid runtime/probe config. |
| `LK-003` | `test/integration/livekit_live_smoke_test.go` (`-tags=livekitproviders`), `.github/workflows/verify.yml` job `livekit-smoke` | non-blocking | real LiveKit RoomService probe + deterministic adapter flow execute with credentials and emit smoke log/report artifacts. |

## 4. Gate execution map (implemented)

| Gate | Implemented command owner | Conformance scope | Failure matrix scope | Replay report mode |
| --- | --- | --- | --- | --- |
| quick | `make verify-quick` via `scripts/verify.sh quick` | `go test` over `test/contract`, `test/integration`, `test/replay` and supporting runtime/api packages | smoke subset `F1`, `F3`, `F7` via `go test ./test/failover -run 'TestF[137]'` | `replay-smoke-report` (fixture `rd-001-smoke`) |
| full | `make verify-full` via `scripts/verify.sh full` | `go test ./...` | full matrix `F1`-`F8` through complete package test run | `replay-regression-report` (fixtures enabled for gate `full`) |

Notes:
1. Quick gate currently executes all tests in `test/contract`, `test/integration`, and `test/replay`, which is broader than a minimal subset.
2. Full gate includes both smoke and full failover tests because it runs `go test ./...`.

## 5. Replay fixture metadata policy

Metadata file:
- `test/replay/fixtures/metadata.json`

Per-fixture fields in use:
1. `gate`: `quick`, `full`, or `both` (default behavior when omitted is `full`).
2. `timing_tolerance_ms`: per-fixture timing tolerance used by divergence evaluation.
3. `final_attempt_latency_threshold_ms`: optional max for OR-02 invocation `final_attempt_latency_ms`.
4. `total_invocation_latency_threshold_ms`: optional max for OR-02 invocation `total_invocation_latency_ms`.
5. `expected_divergences`: expected class/scope entries; `ORDERING_DIVERGENCE` requires `approved: true`.

Policy invariants:
1. `AUTHORITY_DIVERGENCE` is always failing.
2. Missing expected divergences are failing.
3. `PLAN_DIVERGENCE` and `OUTCOME_DIVERGENCE` are failing unless explicitly expected.
4. `TIMING_DIVERGENCE` is failing when `diff_ms` is missing or exceeds tolerance.
5. Invocation latency timing scopes (`invocation_latency_final:*`, `invocation_latency_total:*`) emitted by threshold checks are always failing regardless of timing tolerance.

## 6. Conformance exit criteria for baseline

1. `make verify-quick` passes.
2. `make verify-full` passes.
3. Replay and SLO report artifacts are produced at expected `.codex/` paths.
4. No unresolved contract ambiguity remains for MVP baseline artifacts.

## 7. Expansion backlog (post-MVP)

1. Extend CP distribution hardening beyond current baseline (authn/authz hardening, endpoint discovery/rotation, push invalidation, and live-provider/failover chain joins).
2. Extend artifact-derived invocation-latency extraction beyond the current expanded fixture coverage set to additional replay chains as they are added.

## 8. Consistency references

1. `docs/CIValidationGates.md`
2. `docs/MVP_ImplementationSlice.md` section `10` (`10.1.24` closure + no current open `10.2` items)
3. `docs/LiveKitTransportClosure.md`
