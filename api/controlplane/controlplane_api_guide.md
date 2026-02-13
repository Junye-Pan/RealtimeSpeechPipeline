# Control Plane API Guide

This package defines shared control-plane contract types consumed by runtime and tooling.

## Ownership

- Primary: CP-Team
- Required cross-review when changing contract semantics: Runtime-Team

## What this package owns

- `DecisionOutcome`: deterministic admission/authority outcomes
- `TurnTransition`: legal lifecycle transitions
- `ResolvedTurnPlan`: immutable turn-frozen execution artifact
- Supporting contract types for budgets, buffering, flow control, snapshot provenance, recording policy, and determinism context
- Replay-mode and decision-handling mode enums through `recording_policy.allowed_replay_modes`
- Turn-start snapshot references for routing/admission/policy/provider-health provenance

## Key invariants enforced here

- Outcome kind, phase, scope, and emitter combinations are strict.
- `scope=turn` requires `turn_id`.
- Authority outcomes require `authority_epoch` and RK-24 emitter.
- `shed` is scheduling-point only and emitted by RK-25.
- Turn transitions are restricted to legal deterministic edges.
- `ResolvedTurnPlan` requires complete snapshot provenance and bounded non-zero execution policies.
- `recording_policy.allowed_replay_modes` requires at least one valid mode and rejects duplicates.

## PRD control-plane contract coverage (current status)

Placement lease / authority:
1. `authority_epoch` is first-class in `DecisionOutcome` and `ResolvedTurnPlan` for deterministic single-writer authority enforcement.
2. Explicit lease-token fields are not yet modeled as first-class shared API types in this package.

Routing view / registry handle:
1. `snapshot_provenance.routing_view_snapshot` captures the turn-start routing view reference consumed by runtime.
2. Control-plane snapshot references for admission/policy/provider health are also mandatory and turn-frozen.

Policy + admission deterministic boundaries:
1. `DecisionOutcome` phase/scope constraints encode scheduling-point boundaries (`edge_enqueue`, `edge_dequeue`, `node_dispatch`) and pre-turn vs active-turn semantics.
2. Turn-start plan validation requires bounded buffer/flow-control/budget policies and complete snapshot provenance so runtime can enforce deterministic admission/overload outcomes.
3. Fairness keys and overload posture are carried through edge/flow policy fields and policy/admission snapshot references in the resolved plan.

Replay-mode and decision-handling distinctions:
1. `recording_policy.allowed_replay_modes` supports:
   - `re_simulate_nodes`
   - `playback_recorded_provider_outputs`
   - `replay_decisions`
   - `recompute_decisions`
2. This package enforces allowed mode values and deduplicated declarations so replay/tooling can interpret mode intent deterministically.

MVP fixed-scope alignment (PRD section 5.2):
1. Simple-mode-only MVP is represented through required `resolved_turn_plan.execution_profile`.
2. Single-region authority model alignment is represented through required `authority_epoch` and authority-outcome invariants.
3. Replay baseline (`L0`) MVP requirement is represented through `recording_policy.recording_level`.
4. LiveKit-only MVP transport path is an integration/runtime boundary and is intentionally out of scope for this API package.

## Streaming handoff policy extensions (partial implementation)

`ResolvedTurnPlan` now supports optional deterministic handoff policy under `streaming_handoff`:
1. top-level `enabled` switch
2. per-edge policy objects:
   - `stt_to_llm`
   - `llm_to_tts`
3. edge policy fields:
   - `enabled`
   - `min_partial_chars`
   - `trigger_on_punctuation`
   - `max_partial_age_ms`
   - `max_pending_revisions`
   - `max_pending_segments`
   - `coalesce_latest_only`
   - `emit_flow_control_signals`
   - `degrade_on_threshold_breach`

Policy invariants:
1. Policy snapshot is turn-frozen and immutable after `turn_open`.
2. Identical snapshot + input sequence yields identical handoff decisions.
3. Handoff policy cannot bypass authority and cancellation precedence rules.

Implementation status:
1. Implemented:
   - `ResolvedTurnPlan.streaming_handoff` contract types and validation in `api/controlplane/types.go`.
2. Not implemented yet:
   - `docs/ContractArtifacts.schema.json` does not yet model `ResolvedTurnPlan.streaming_handoff`.
   - `test/contract/fixtures/resolved_turn_plan` does not yet include streaming handoff valid/invalid fixture coverage.
   - Runtime scheduling still uses env-driven handoff policy (`internal/runtime/executor/streaming_handoff.go`); plumb-through from resolved-turn-plan policy to runtime scheduler remains in progress.

Planned control-plane wiring milestone:
1. Resolved turn plans should become the canonical source for runtime handoff policy values during turn execution.
2. Environment-driven overrides should remain optional operator controls, not the primary policy source, once plumb-through is complete.
3. Latency-critical handoff knobs and parity requirements should be represented in turn-frozen plan snapshots for replay-stable decisions.

## Where this package is used

- Turn-open gating and lifecycle control in `internal/runtime/turnarbiter`
- Turn plan freeze in `internal/runtime/planresolver`
- Contract fixture validation in `internal/tooling/validation`
- Integration/replay/conformance assertions under `test/`

## Change checklist

1. Keep `api/controlplane/types.go` aligned with `docs/ContractArtifacts.schema.json`.
2. Update fixture coverage for valid and invalid cases:
   - `test/contract/fixtures/decision_outcome`
   - `test/contract/fixtures/turn_transition`
   - `test/contract/fixtures/resolved_turn_plan`
3. Re-run:
   - `go test ./api/controlplane ./test/contract`
   - `make verify-quick`

## Related docs

- `docs/repository_docs_index.md`
- `docs/ContractArtifacts.schema.json`
- `docs/CIValidationGates.md`
- `docs/rspp_SystemDesign.md`
