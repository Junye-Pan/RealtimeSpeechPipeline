# Internal Observability Guide

This folder implements telemetry, timeline evidence recording, replay divergence/access, audit logging, and retention/deletion enforcement.

## Ownership and module map

| Module | Path | Primary owner | Cross-review |
| --- | --- | --- | --- |
| `OR-01` Telemetry | `internal/observability/telemetry` | `ObsReplay-Team` | Runtime-Team |
| `OR-02` Timeline Recorder | `internal/observability/timeline` | `ObsReplay-Team` | Runtime-Team |
| `OR-03` Replay Engine | `internal/observability/replay` | `ObsReplay-Team` | Security-Team |

Security-sensitive paths:
- `internal/observability/replay`
- redaction and replay-access enforcement paths

## OR-01 telemetry guarantees

1. Best-effort, non-blocking pipeline behavior under pressure.
2. Deterministic sampling decisions for telemetry channels; queue-overflow drops are bounded/non-blocking and explicitly counted.
3. Runtime instrumentation entrypoints remain safe for hot paths.

## PRD MVP quality-gate measurement contract

Observability ownership for PRD production targets:
1. Turn-open decision latency (`p95 <= 120 ms`): measured from `turn_open_proposed` acceptance marker to `turn_open`.
2. First assistant output latency (`p95 <= 1500 ms`): measured from `turn_open` to first DataLane output chunk marker when `first_output_at` evidence is present.
3. Cancellation fence latency (`p95 <= 150 ms`): measured from cancel-accept marker to egress fence marker.
4. Authority safety (`0` accepted stale-epoch outputs): enforced by stale-epoch outcome evidence and failover/lease-rotation test artifacts.
5. Replay baseline completeness (`100%` accepted-turn coverage): verified from OR-02 required baseline evidence fields.
6. Terminal lifecycle correctness (`100%`): verified from OR-02 terminal fields (`terminal_outcome`, `close_emitted`) plus runtime terminal transition evidence (`active -> terminal -> close`).
7. End-to-end latency (`P50 <= 1.5s`, `P95 <= 2s`): measured from turn-open to terminal completion markers.

Current implementation note:
- `turn_open`/terminal/cancel-fence evidence is synthesized in runtime baseline normalization; `first_output_at` is not auto-synthesized there and must be provided by integration paths that emit first-output markers.

## OR-02 timeline baseline requirements

Accepted-turn evidence must preserve:
- lifecycle markers (open/terminal/close ordering)
- authority epoch and authority outcomes
- determinism markers
- snapshot provenance references
- replay-correlation metadata

Failure-path evidence mapping (current baseline + adjacent runtime signals):
- `F1`: admission/scheduling-point outcomes and policy snapshot refs (`decision_outcomes`, `snapshot_provenance`).
- `F3`: provider invocation outcome class and retry/switch provenance (`invocation_outcomes`; optional per-attempt recorder evidence).
- `F4`/`F5`: pressure/discontinuity markers are emitted in runtime control signals and comparator lineage checks; dedicated OR-02 baseline fields for these markers are not yet first-class.
- `F7`/`F8`: authority epoch mismatch/revoke/migration markers (`authority_epoch`, authority decision outcomes, `migration_markers`).

Deterministic downgrade rule:
- Stage-A detail evidence may downgrade/drop under pressure with deterministic `recording_level_downgraded` signaling.
- Baseline append remains capacity-bounded; if baseline append is unavailable on terminalization, runtime converts the terminal path to `abort(recording_evidence_unavailable)`.

Replay fidelity and mode policy (PRD-aligned):
1. `L0` baseline recording is mandatory for MVP and must preserve replay-critical control evidence.
2. `L1` and `L2` fidelity identifiers exist in API/security policy surfaces; current OR-02 baseline artifact and completeness gate remain `L0`-centric.
3. Replay comparator currently provides deterministic divergence comparison across plan/outcome/ordering/authority/timing dimensions and lineage checks.
4. Replay-mode and decision-handling-mode policy (`replay-decisions` vs `recompute-decisions`) remains a contract target; these mode fields are not yet first-class in OR-02 baseline artifacts.
5. Any pressure-triggered downgrade must preserve `L0` baseline evidence invariants for accepted-turn completeness checks.

Streaming-handoff evidence requirements (implemented MVP baseline + pending expansion):
1. Per-attempt provider evidence is captured in OR-02 recorder entries with raw payload and timing fields:
   - `input_payload`, `output_payload`, `output_status_code`, `payload_truncated`
   - `attempt_latency_ms`, `first_chunk_latency_ms`, `chunk_count`, `bytes_out`, `streaming_used`
2. Cross-stage handoff timing is captured via `HandoffEdgeEvidence`:
   - `stt_to_llm` and `llm_to_tts` edges
   - `handoff_id`, `upstream_revision`, `action`, queue/watermark snapshot fields
3. When live provider smoke runs, it publishes operator artifacts:
   - `.codex/providers/live-provider-chain-report.json`
   - `.codex/providers/live-provider-chain-report.md`
   - `.codex/providers/live-provider-chain-report.streaming.json`
   - `.codex/providers/live-provider-chain-report.streaming.md`
   - `.codex/providers/live-provider-chain-report.nonstreaming.json`
   - `.codex/providers/live-provider-chain-report.nonstreaming.md`
   - artifacts include per-step input snapshot, raw I/O samples, and latency/handoff rollups
4. Raw payload capture remains policy-controlled by:
   - `RSPP_PROVIDER_IO_CAPTURE_MODE`
   - `RSPP_PROVIDER_IO_CAPTURE_MAX_BYTES`
5. Pending expansion:
   - replay divergence policy for missing handoff markers/chunk-order markers will be promoted once API-level handoff schema extensions are finalized.
6. Persistence scope note:
   - baseline JSON artifacts persist OR-02 `BaselineEvidence` entries.
   - per-attempt/handoff/invocation-snapshot recorder entries are currently consumed by runtime/integration/test/report paths and are not yet serialized into the baseline artifact schema.

## Live latency diagnosis requirements (implemented MVP baseline)

For streaming/non-streaming performance investigations and MVP live gate checks, live-chain artifacts include the following explicit markers:
1. Execution identity:
   - `execution_mode`: `streaming` or `non_streaming`
   - `effective_handoff_policy`: effective overlap toggles/thresholds used for the run
   - `effective_provider_streaming`: effective provider-streaming enable/disable posture
   - `semantic_parity`: whether completion semantics are equivalent for compared runs
   - `parity_comparison_valid`: whether the comparison is gate-valid
   - `parity_invalid_reason`: reason when parity/validity is false
2. Per-step timing and I/O:
   - raw input/output capture fields already emitted at attempt level
   - per-attempt `attempt_latency_ms`, `first_chunk_latency_ms`, `chunk_count`, `bytes_out`, `streaming_used`
3. Handoff decision lineage:
   - explicit handoff actions (`forward`, `coalesce`, `supersede`, `final_fallback`)
   - queue/watermark context when action selection occurs
4. Comparison validity fields:
   - provider-combination identity for both runs (`comparison_identity`)
   - per-step input snapshots are emitted in report step payloads (not currently enforced as parity-equality markers)
   - invalid-comparison reason when parity constraints are not met

Replay and gate policy:
1. Missing required compare markers (`execution_mode`, `comparison_identity`, parity fields) fail comparison validity.
2. MVP live fixed-decision checks fail when parity markers are missing or invalid.
3. Streaming reports must include overlap proof (`handoffs` and/or non-zero first-audio timing); non-streaming reports must not contain `streaming_used=true`.
4. Comparison output distinguishes true latency regressions from semantic mismatch.

## OR-03 replay and access guarantees

1. Divergence classes remain stable (`PLAN_DIVERGENCE`, `OUTCOME_DIVERGENCE`, `ORDERING_DIVERGENCE`, `TIMING_DIVERGENCE`, `AUTHORITY_DIVERGENCE`).
2. Replay access is deny-by-default when required authorization attributes are missing.
3. Audit append path is immutable and fail-closed.
4. Audit backend supports HTTP primary with deterministic JSONL fallback.
5. Retention/deletion supports hard-delete or cryptographic inaccessibility semantics.

Required replay request attributes:
- `tenant_id`
- `principal_id`
- `role`
- `purpose`
- `requested_scope`
- `requested_fidelity`

PRD MVP fixed-scope alignment:
1. Replay scope for MVP is OR-02 baseline (`L0`) completeness as a required production gate.
2. Observability evidence for runtime MVP gates is produced for the active MVP transport path (LiveKit integration lane).

## Integration touchpoints

- `cmd/rspp-runtime retention-sweep` consumes replay retention APIs.
- `cmd/rspp-cli` consumes timeline and divergence artifacts.
- Runtime modules append OR-02 evidence and emit OR-01 telemetry markers.

## Test and gate touchpoints

- `go test ./internal/observability/...`
- `make security-baseline-check`
- `make verify-quick`
- `make verify-full`

## Change checklist

1. Preserve replay-critical OR-02 evidence on accepted-turn paths.
2. Keep telemetry non-blocking behavior under stress.
3. Keep replay access/audit failure modes fail-closed.
4. Re-run observability and security baseline tests for replay-path changes.

## Related repo-level docs

- `docs/repository_docs_index.md`
- `docs/SecurityDataHandlingBaseline.md`
- `docs/CIValidationGates.md`
- `docs/ContractArtifacts.schema.json`
