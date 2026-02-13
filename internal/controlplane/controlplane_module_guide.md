# Internal Control Plane Guide

This folder owns turn-start control-plane resolution used by runtime turn arbitration. It resolves immutable snapshots, normalizes profile inputs, and emits deterministic admission and authority decisions before a turn is accepted.

## Ownership and review

| Module | Path | Primary owner | Cross-review |
| --- | --- | --- | --- |
| `CP-01` Pipeline Registry | `internal/controlplane/registry` | `CP-Team` | Runtime-Team |
| `CP-02` Spec Normalizer/Validator | `internal/controlplane/normalizer` | `CP-Team` | Runtime-Team |
| `CP-03` GraphDefinition Compiler | `internal/controlplane/graphcompiler` | `CP-Team` | Runtime-Team |
| `CP-04` Policy Evaluation | `internal/controlplane/policy` | `CP-Team` | Security-Team |
| `CP-05` Resource/Admission | `internal/controlplane/admission` | `CP-Team` | Runtime-Team |
| `CP-06` Security/Tenant Isolation | `internal/controlplane/security` | `Security-Team` | CP-Team |
| `CP-07` Placement Lease Authority | `internal/controlplane/lease` | `CP-Team` | Runtime-Team |
| `CP-08` Routing View Publisher | `internal/controlplane/routingview` | `CP-Team` | Runtime-Team |
| `CP-09` Rollout/Version Resolver | `internal/controlplane/rollout` | `CP-Team` | Runtime-Team |
| `CP-10` Provider Health Aggregator | `internal/controlplane/providerhealth` | `CP-Team` | Provider-Team |
| Distribution adapters | `internal/controlplane/distribution` | `CP-Team` | Security-Team |

Security-sensitive paths:
- `internal/controlplane/security`
- `internal/controlplane/distribution/http_adapter.go` (auth headers and remote snapshot retrieval)

## Turn-start bundle composition order

`internal/runtime/turnarbiter/controlplane_bundle.go` composes control-plane services in this order:

1. `registry`
2. `normalizer`
3. `rollout`
4. `graphcompiler`
5. `routingview`
6. `providerhealth`
7. `policy`
8. `lease`
9. `admission`

## Deterministic guarantees from this folder

1. File and HTTP distribution backends return equivalent snapshot semantics for equal inputs.
2. HTTP distribution performs deterministic endpoint failover, bounded retries, and bounded stale-serving behavior.
3. Unsupported execution profile inputs are classified deterministically during pre-turn normalization.
4. Snapshot provenance needed by `ResolvedTurnPlan` is preserved for replay and audit.
5. Stale/missing/invalid snapshot outcomes are deterministic so runtime pre-turn handling remains replay-stable.

## Configuration knobs used here

File distribution:
- `RSPP_CP_DISTRIBUTION_PATH`

HTTP distribution:
- `RSPP_CP_DISTRIBUTION_HTTP_URLS`
- `RSPP_CP_DISTRIBUTION_HTTP_URL`
- timeout/retry/cache/auth options in `internal/controlplane/distribution/http_adapter.go`

## PRD contract expansion plan (coding status: unimplemented)

Planned control-plane contract coverage aligned to `docs/PRD.md`:

1. Multi-region routing/failover contract planning for control-plane-issued placement/routing decisions.
   - Coding status: unimplemented.
2. State contract planning for hot vs durable ownership boundaries used by control-plane resolution and migration decisions.
   - Coding status: unimplemented.
3. Identity/correlation/idempotency contract planning for `session_id`, `turn_id`, `pipeline_version`, `event_id`, `provider_invocation_id`, and lease epoch semantics.
   - Coding status: unimplemented.
4. Resource/admission contract planning for fairness keys, lane precedence, and deterministic scheduling-point enforcement boundaries.
   - Coding status: unimplemented.
5. Security/tenant-isolation contract planning for policy publication and enforcement expectations across runtime/transport/external-node boundaries.
   - Coding status: unimplemented.

Streaming handoff policy (implemented contract, resolver plumbing pending):
- control-plane contract now includes `ResolvedTurnPlan.streaming_handoff` and per-edge policy bounds in `api/controlplane/types.go`
- CP continues to own profile-default and tenant-safe bounds for:
  - `STT->LLM` partial trigger policy
  - `LLM->TTS` partial trigger policy
  - per-edge queue limits and degrade thresholds
- pending integration: `internal/runtime/planresolver` does not yet populate per-turn streaming handoff policy values from control-plane data sources

Planned control-plane policy wiring (documentation scope):
1. Promote control-plane-resolved handoff values to runtime as the default policy source for accepted turns.
2. Keep runtime environment flags as explicit operational override path with clear precedence rules.
3. Include fairness/semantic-parity policy metadata needed for valid streaming vs non-streaming latency comparisons.
4. Ensure emitted policy snapshots remain immutable per accepted turn for deterministic replay.

## Test and gate touchpoints

Primary package tests:
- `go test ./internal/controlplane/...`
- `go test ./internal/runtime/turnarbiter -run CP`

Repository gates:
- `make verify-quick`
- `make verify-full`

## Change checklist

1. Preserve backend parity and deterministic failure classification.
2. Keep snapshot provenance fields complete and stable.
3. Keep `api/controlplane` contract compatibility for emitted outcomes.
4. Re-run control-plane and turnarbiter tests before merge.

## Related repo-level docs

- `docs/repository_docs_index.md`
- `docs/rspp_SystemDesign.md`
- `docs/CIValidationGates.md`
- `docs/SecurityDataHandlingBaseline.md`
- `docs/ContractArtifacts.schema.json`
