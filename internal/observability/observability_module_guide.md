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
2. Deterministic sampling/drop decisions for telemetry channels.
3. Runtime instrumentation entrypoints remain safe for hot paths.

## PRD MVP quality-gate measurement contract

Observability ownership for PRD production targets:
1. Turn-open decision latency (`p95 <= 120 ms`): measured from `turn_open_proposed` acceptance marker to `turn_open`.
2. First assistant output latency (`p95 <= 1500 ms`): measured from `turn_open` to first DataLane output chunk marker.
3. Cancellation fence latency (`p95 <= 150 ms`): measured from cancel-accept marker to egress fence marker.
4. Authority safety (`0` accepted stale-epoch outputs): enforced by stale-epoch outcome evidence and failover/lease-rotation test artifacts.
5. Replay baseline completeness (`100%` accepted-turn coverage): verified from OR-02 required baseline evidence fields.
6. Terminal lifecycle correctness (`100%`): verified from OR-02 lifecycle sequence evidence (`terminal -> close`).
7. End-to-end latency (`P50 <= 1.5s`, `P95 <= 2s`): measured from turn-open to terminal completion markers.

## OR-02 timeline baseline requirements

Accepted-turn evidence must preserve:
- lifecycle markers (open/terminal/close ordering)
- authority epoch and authority outcomes
- determinism markers
- snapshot provenance references
- replay-correlation metadata

Failure-path evidence requirements:
- `F1`: admission/scheduling-point outcomes and policy snapshot refs
- `F3`: provider invocation outcome class and retry/switch provenance
- `F4`/`F5`: buffer pressure, drop/discontinuity lineage markers
- `F7`/`F8`: authority epoch mismatch/revoke/migration markers

Deterministic downgrade rule:
- recording fidelity may downgrade under pressure, but replay-critical baseline evidence cannot be dropped.

Replay fidelity and mode policy (PRD-aligned):
1. `L0` baseline recording is mandatory for MVP and must preserve replay-critical control evidence.
2. `L1` and `L2` are higher capture tiers that extend payload/detail capture under policy and cost controls.
3. Replay supports deterministic re-simulation and recorded-output playback modes; provider determinism is not assumed unless recorded outputs are used.
4. Decision-handling mode distinctions are explicit:
   - `replay-decisions`: use recorded runtime decisions from OR-02 evidence.
   - `recompute-decisions`: recompute only when required deterministic inputs are fully captured and valid.
5. Replay artifacts must declare both replay mode and decision-handling mode so comparison validity is auditable.
6. If `recompute-decisions` prerequisites are not met, replay tooling must fall back to `replay-decisions` semantics or mark comparison validity failure.
7. Any pressure-triggered downgrade must preserve `L0` baseline evidence invariants.

Streaming-handoff evidence requirements (implemented baseline + pending expansion):
1. Per-attempt provider evidence is persisted in OR-02 with raw payload and timing fields:
   - `input_payload`, `output_payload`, `output_status_code`, `payload_truncated`
   - `attempt_latency_ms`, `first_chunk_latency_ms`, `chunk_count`, `bytes_out`, `streaming_used`
2. Cross-stage handoff timing is persisted via `HandoffEdgeEvidence`:
   - `stt_to_llm` and `llm_to_tts` edges
   - `handoff_id`, `upstream_revision`, `action`, queue/watermark snapshot fields
3. Live chain smoke publishes operator artifacts:
   - `.codex/providers/live-provider-chain-report.json`
   - `.codex/providers/live-provider-chain-report.md`
   - artifacts include per-step input snapshot, raw I/O samples, and latency/handoff rollups
4. Raw payload capture remains policy-controlled by:
   - `RSPP_PROVIDER_IO_CAPTURE_MODE`
   - `RSPP_PROVIDER_IO_CAPTURE_MAX_BYTES`
5. Pending expansion:
   - replay divergence policy for missing handoff markers/chunk-order markers will be promoted once API-level handoff schema extensions are finalized.

## Live latency diagnosis requirements (planned additions)

For streaming/non-streaming performance investigations, live-chain artifacts should include the following explicit markers:
1. Execution identity:
   - `execution_mode`: `streaming` or `non_streaming`
   - `semantic_parity`: whether completion semantics are equivalent for compared runs
2. Per-step timing and I/O:
   - raw input/output capture fields already emitted at attempt level
   - per-attempt `attempt_latency_ms`, `first_chunk_latency_ms`, `chunk_count`, `bytes_out`, `streaming_used`
3. Handoff decision lineage:
   - explicit handoff actions (`forward`, `coalesce`, `supersede`, `final_fallback`)
   - queue/watermark context when action selection occurs
4. Comparison validity fields:
   - provider combination identity for both runs
   - prompt/input snapshot identity for both runs
   - invalid-comparison reason when parity constraints are not met

Replay and gate policy target:
1. Missing required compare markers should fail comparison validity.
2. Comparison output should distinguish true latency regressions from semantic mismatch.

## OR-03 replay and access guarantees

1. Divergence classes remain stable (`PLAN`, `OUTCOME`, `ORDERING`, `TIMING`, `AUTHORITY`).
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
