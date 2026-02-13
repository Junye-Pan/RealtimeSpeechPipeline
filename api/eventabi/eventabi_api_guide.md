# Event ABI API Guide

This package defines the shared event and control-signal contract vocabulary used across runtime, transport, provider invocation, and tests.

PRD alignment scope:
- `docs/PRD.md` section 4.1.3 (Event ABI schema/envelope contract)
- `docs/PRD.md` section 4.2 (lane signal model and control preemption semantics)

## Ownership

- Primary: Runtime-Team
- Required cross-review for contract changes: CP-Team

## What this package owns

- Lane model: `DataLane`, `ControlLane`, `TelemetryLane`
- Scope model:
  - envelope `event_scope`: `session`, `turn`
  - control-signal `scope` field values used by current contracts/fixtures: `session`, `turn`, `node`, `provider_invocation`
- Payload classes: `audio_raw`, `text_raw`, `PII`, `PHI`, `derived_summary`, `metadata`
- Redaction actions: `allow`, `mask`, `hash`, `drop`, `tokenize`
- Artifact structs:
  - `EventRecord`
  - `ControlSignal`
  - `MediaTime`
  - `SeqRange`
  - `RedactionDecision`

## Current representation model (implemented)

1. Runtime contracts are currently represented as two typed artifacts:
   - `EventRecord` for non-control lanes (`DataLane`, `TelemetryLane`)
   - `ControlSignal` for `ControlLane`
2. Both artifacts share core envelope identity/timing fields.
3. `ControlSignal` carries signal-specific control metadata (for example `edge_id`, `target_lane`, `reason`, `seq_range`).

PRD model alignment note:
- PRD describes a universal Event envelope across all lanes.
- Current package uses split artifact structs with shared envelope subsets.
- This split model is the implemented baseline for current schema/tests.

## PRD Event ABI envelope alignment status

| PRD envelope requirement | Current status in this package |
| --- | --- |
| `session_id`, `turn_id`, `pipeline_version`, `event_id`, lane, transport/runtime sequence, runtime/wall-clock timestamps | implemented |
| turn-scope authority context | implemented (`authority_epoch` required for turn-scoped `EventRecord`; required field on `ControlSignal`) |
| `edge_id` | implemented on `ControlSignal` |
| `sync_domain`, `discontinuity_id` | implemented on `ControlSignal` |
| `node_id` | not implemented yet |
| `causal_parent_id` | not implemented yet |
| `idempotency_key` | not implemented yet |
| `sync_id` | not implemented yet |
| `merge_group_id`, `merged_from_event_ids` | not implemented yet |
| `late_after_cancel` | not implemented yet |

## High-value invariants

- `EventRecord` cannot use `ControlLane`.
- Turn-scope events require `turn_id` and authority context.
- `audio_raw` payloads require media-time coordinates.
- `ControlSignal` requires `ControlLane` and metadata payload class.
- Signal-specific emitter/field constraints are strict for lifecycle, flow-control, provider, authority, and transport signals.
- For `budget_warning` / `budget_exhausted` / `degrade` / `fallback`, strict emitter constraints are currently schema-level; Go `ControlSignal.Validate()` currently enforces enum membership but not dedicated emitter-specific checks for those four signals.

## Control signal vocabulary vs PRD lane model

Implemented in current contract vocabulary:
- turn lifecycle: `turn_open_proposed`, `turn_open`, `commit`, `abort`, `close`
- interruption/cancel: `barge_in`, `stop`, `cancel`
- pressure/flow control: `watermark`, `budget_warning`, `budget_exhausted`, `degrade`, `fallback`, `flow_xoff`, `flow_xon`, `credit_grant`, `drop_notice`, `discontinuity`
- provider/routing/admission/authority/connection/output signals:
  - `provider_error`, `circuit_event`, `provider_switch`
  - `lease_issued`, `lease_rotated`, `migration_start`, `migration_finish`, `session_handoff`
  - `admit`, `reject`, `defer`, `shed`
  - `stale_epoch_reject`, `deauthorized_drain`
  - `connected`, `reconnecting`, `disconnected`, `ended`, `silence`, `stall`
  - `output_accepted`, `playback_started`, `playback_completed`, `playback_cancelled`

PRD replay-critical marker alignment:
- partial: `recording_level_downgraded` is present.
- not implemented yet as explicit Event ABI signal contracts:
  - plan-hash marker signal
  - determinism-seed marker signal
  - ordering-marker signal

## Compatibility, ordering, and dedupe expectations

Implemented in this package:
- strict `schema_version` format validation (`v<major>.<minor>[.<patch>]`)
- strict signal enum and emitter constraints
- required sequence/timestamp non-negative constraints

Not implemented yet in this package:
- explicit version negotiation/deprecation policy rules
- explicit idempotency-key contract and dedupe semantics
- explicit per-lane/per-edge ordering reconciliation metadata beyond current sequence fields

## Streaming handoff contract extensions (planned, not implemented yet)

Current implementation status:
- not present in current `EventRecord` / `ControlSignal` schema fields
- not enforced by current runtime Event ABI normalization/validation

Target event metadata for orchestration-level handoff (not yet implemented):
1. `stream_kind`: `start|delta|final|error`.
2. `handoff_id`: deterministic identifier for one upstream->downstream handoff window.
3. `upstream_revision`: monotonic revision for partial-to-final reconciliation.
4. `chunk_sequence`: monotonic sequence within one provider invocation stream.
5. `handoff_edge`: `stt_to_llm` or `llm_to_tts` where applicable.

Planned contract rule targets (not yet enforced by Event ABI):
1. A `final`/`error` chunk must close a stream for a given `provider_invocation_id`.
2. Chunks from stale authority epochs are invalid and must be rejected deterministically.
3. Cross-stage handoff events must carry both source and destination correlation identifiers.

## Why this package matters

- Runtime event gateway validates/normalizes against these contracts.
- Contract fixture suites in `test/contract` use this package as typed truth.
- Security and replay behavior depend on payload class and redaction action consistency.

## Change checklist

1. Update `api/eventabi/types.go` and `docs/ContractArtifacts.schema.json` together.
2. Keep PRD alignment status current when adding/removing envelope fields or control signals.
3. Expand valid/invalid fixtures under `test/contract/fixtures/event` and `control_signal`.
4. Re-run:
   - `go test ./api/eventabi ./test/contract`
   - `make verify-quick`

## Related docs

- `docs/repository_docs_index.md`
- `docs/ContractArtifacts.schema.json`
- `docs/SecurityDataHandlingBaseline.md`
- `docs/rspp_SystemDesign.md`
- `docs/PRD.md`
