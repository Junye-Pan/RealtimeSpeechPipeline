# Observability API Guide

This package defines shared observability and replay API contracts used for replay access control, immutable replay audit records, divergence classification, and replay evidence metadata.

## Ownership

- Primary: `ObsReplay-Team`
- Cross-review for `api/` contract changes: `Runtime-Team` and `ControlPlane-Team`
- Security-sensitive change review: `Security-Team`

## PRD alignment scope

This guide tracks API-level contracts required by `docs/PRD.md` for:
1. observability and replay by default (metrics/traces/logs/events with correlation)
2. replay and timeline contract surfaces (`EventTimeline`, `ReplayCursor`, `ReplayMode`)
3. recording-level semantics (`L0/L1/L2`) and mandatory replay-critical evidence
4. telemetry lane non-blocking behavior and overload safety constraints
5. MVP replay baseline requirement (`OR-02`) and Simple-mode constraints

## Contract surfaces owned here

Implemented shared API contracts:
- Replay divergence classes:
  - `PLAN_DIVERGENCE`
  - `OUTCOME_DIVERGENCE`
  - `ORDERING_DIVERGENCE`
  - `TIMING_DIVERGENCE`
  - `AUTHORITY_DIVERGENCE`
- Replay access contract types:
  - `ReplayAccessRequest`
  - `ReplayAccessDecision`
  - `ReplayRedactionMarker`
- Immutable replay audit schema:
  - `ReplayAuditEvent`

PRD-required replay/timeline surfaces tracked by this API contract boundary:
- `EventTimeline` contract semantics (ordered event log by session/turn/lane)
- `ReplayCursor` stepping semantics
- `ReplayMode` semantics (re-simulate vs recorded-provider-output playback, with decision handling modes)
- recording-level contract (`L0/L1/L2`) and replay-critical evidence requirements

## Required invariants

Access and audit invariants:
- Replay requests are deny-by-default if required authorization attributes are missing.
- Replay fidelity must be valid (`L0`, `L1`, `L2`).
- Redaction markers must carry valid payload class/action pairs.
- Denied decisions require explicit deny reason.

Replay/timeline invariants (PRD contract):
- All recording levels (`L0/L1/L2`) must preserve replay-critical control evidence:
  - plan hash
  - determinism markers
  - authority markers
  - terminal outcomes
  - turn-start snapshot provenance references
  - admission/policy/authority decision outcomes that gate turn opening/execution
- Replay/timeline recording must separate low-latency local append from asynchronous durable export so timeline persistence cannot block control progression or cancellation propagation.
- If timeline pressure requires fidelity reduction, downgrade behavior must be deterministic and must still preserve replay-critical control evidence.

Telemetry-lane observability invariants (PRD contract):
- Telemetry is non-blocking and best-effort under overload.
- Telemetry dropping/sampling must never block ControlLane delivery or add tail latency to DataLane.
- Contracted telemetry signals include metrics/traces/log/debug categories with session/turn/pipeline correlation markers.

## MVP standards tie-in

- Replay baseline completeness target: 100% of accepted turns include required `OR-02` baseline replay evidence fields.
- Replay scope for MVP is `OR-02` baseline evidence (`L0` baseline contract mandatory).
- MVP mode is `Simple` only; observability/replay defaults must remain compatible with Simple-profile execution semantics.

## Streaming-handoff observability extensions

Replay/telemetry schema target includes structured handoff timing and state markers:
1. per-edge timing (`stt_partial_to_llm_start_ms`, `llm_partial_to_tts_start_ms`)
2. per-stage first chunk timing and chunk counts
3. queue depth/watermark snapshots at handoff decisions
4. handoff action markers (`forward`, `coalesce`, `supersede`, `final_fallback`) and flow-control signals (`flow_xoff`, `flow_xon`)
5. correlation fields (`handoff_id`, `upstream_revision`, `provider_invocation_id`)

Current implementation status:
1. Shared API code currently exposes divergence classes plus replay access/audit contracts as first-class types.
2. First-class shared API types/fields for streaming-handoff markers are not fully implemented in `api/observability`.
3. Streaming-handoff marker validation/classification wiring in replay comparator logic is not fully implemented.

Target divergence behavior:
1. missing required handoff markers is an `OUTCOME_DIVERGENCE`
2. non-monotonic handoff/chunk sequence is an `ORDERING_DIVERGENCE`
3. timing-threshold breach classification remains `TIMING_DIVERGENCE`

## Where this package is used

- Replay divergence evaluation in `internal/tooling/regression`
- Replay authorization/audit in `internal/observability/replay`
- Replay and conformance evidence checks in CI/security baseline suites

## Change checklist

1. Keep API contract semantics aligned with PRD replay/timeline requirements.
2. Preserve compatibility with `L0/L1/L2` recording-level invariants and `OR-02` MVP baseline obligations.
3. Update `api/observability` tests and replay package tests together.
4. Re-run:
   - `go test ./api/observability ./internal/observability/replay`
   - `make verify-quick`
   - `make security-baseline-check`

## Related docs

- `docs/PRD.md`
- `docs/repository_docs_index.md`
- `docs/SecurityDataHandlingBaseline.md`
- `docs/ContractArtifacts.schema.json`
- `docs/CIValidationGates.md`
