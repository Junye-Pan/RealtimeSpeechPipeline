# Control Plane API Guide

This package defines the target contract architecture for control-plane APIs consumed by runtime, transport adapters, tooling, and operations.

PRD alignment scope:
- `docs/PRD.md` section 3 ownership split and section 4.1.8 contracts 26-36 (control plane, lease/epoch, routing view, state boundaries, policy/admission/security)
- `docs/PRD.md` section 4.3 minimal rule set (plan freeze, authority safety, replay-critical evidence)
- `docs/rspp_SystemDesign.md` sections 3-5 (target architecture, module boundaries, MVP baseline)
- `docs/RSPP_features_framework.json` groups:
  - `Control Plane (Registry, Routing, Rollouts)` (`F-092`..`F-102`)
  - `State, Authority & Migration` (`F-155`..`F-160`)
  - `Governance, Compatibility & Change Management` (`F-161`..`F-165`, post-MVP)
  - `Extensibility: Custom/External Nodes` (`F-121`..`F-124`)

## Ownership

- Primary: `CP-Team`
- Required cross-review when changing contract semantics: `Runtime-Team`
- Security-sensitive contract review required: `Security-Team`

## 1) Module list with responsibilities

Target logical modules under `api/controlplane` (implemented today as a single package, but designed as separable contract surfaces):

| Module | Responsibility | Core contract types | Feature evidence |
| --- | --- | --- | --- |
| `CPAPI-01 Registry & Versioning` | Versioned, immutable artifact identity for pipeline/policy/profile resolution. | `PipelineVersionRef`, `ExecutionProfileRef`, `PolicyVersionRef` | `F-015`, `F-092`, `F-100`, `F-102` |
| `CPAPI-02 Rollout Resolution` | Deterministic rollout selection (canary/percentage/allowlist) and rollback metadata. | `RolloutPolicyRef`, `RolloutDecision`, `RolloutProvenance` | `F-093`, `F-100`, `F-102` |
| `CPAPI-03 Routing & Session Route` | Runtime placement decision references plus client route metadata surface. | `RoutingViewSnapshotRef`, `SessionRoute`, `TransportEndpointRef` | `F-094`, `F-099`, `F-101`, `F-159` |
| `CPAPI-04 Authority & Lease` | Single-authority proof model across ingress/egress and migration boundaries. | `PlacementLease`, `LeaseEpoch`, `LeaseTokenRef`, authority outcomes | `F-095`, `F-156`, `F-157`, `F-111` |
| `CPAPI-05 Admission & Capacity` | Pre-turn and boundary admission outcomes with deterministic scope/phase semantics. | `DecisionOutcome`, `AdmissionPolicySnapshotRef`, `FairnessScope` | `F-098`, `F-049`, `F-050`, `F-172`, `F-174` |
| `CPAPI-06 Policy & Provider Binding` | Policy decision outputs frozen into turn plans (provider binding, adaptive actions, constraints). | `PolicyResolutionSnapshotRef`, `ProviderBindings`, `AllowedAdaptiveActions` | `F-056`, `F-057`, `F-060`, `F-062`, `F-066` |
| `CPAPI-07 Turn Plan & Provenance` | Turn-frozen contract artifact consumed by runtime for deterministic execution/replay. | `ResolvedTurnPlan`, `SnapshotProvenance`, `RecordingPolicy`, `Determinism` | `F-081`, `F-149`, `F-150`, `F-151`, `F-153` |
| `CPAPI-08 Lifecycle & Transition` | Deterministic turn lifecycle transition legality and pre-turn boundary constraints. | `TurnTransition`, lifecycle trigger enums | `F-139`, `F-141`, `F-138` |
| `CPAPI-09 Security & Tenant Context` | Tenant boundary, auth context, token integrity, and redaction-safe contract metadata. | `TenantScope`, `SessionTokenClaims`, `RBACContextRef` | `F-114`, `F-115`, `F-117`, `F-119`, `F-120` |
| `CPAPI-10 Audit & Governance` | Mutation and decision auditability for pipeline/policy/rollout control actions. | `AuditRecord`, `ChangeApprovalRef`, `ChangeReason` | `F-097`, `F-165`, `F-170` |
| `CPAPI-11 Compatibility & Conformance` | Version-skew and profile-compatibility declarations for runtime/control-plane/schema combinations. | `CompatibilityWindow`, `ConformanceProfileRef` | `F-162`, `F-163`, `F-164` |
| `CPAPI-12 Extension Contracts` | Contract seams for plugins, custom/external nodes, and custom runtime capability declarations. | `NodePluginDescriptor`, `ExternalNodeBoundaryRef`, `RuntimeCapabilitySnapshot` | `F-011`, `F-121`, `F-122`, `F-123`, `F-124`, `F-161` |

## 2) Key interfaces (pseudocode)

```go
// Composition root for turn-start control-plane contracts.
type TurnStartContractResolver interface {
    ResolveTurnStart(ctx Context, req TurnStartRequest) (TurnStartBundle, error)
}

type TurnStartRequest struct {
    TenantID                 string
    SessionID                string
    TurnID                   string
    RequestedPipelineVersion string
    RequestedExecutionProfile string
    RequestedPolicyVersion   string
    RequestedAuthorityEpoch  int64
    TransportKind            string
    RegionHint               string
}

type TurnStartBundle struct {
    Plan              ResolvedTurnPlan
    AdmissionOutcome  DecisionOutcome
    Lease             PlacementLease
    SessionRoute      SessionRoute
    AuditRef          string
}
```

```go
// Control-plane API boundary for transport adapters and session bootstrap.
type SessionRoutingAPI interface {
    ResolveSessionRoute(ctx Context, req SessionRouteRequest) (SessionRoute, error) // F-094, F-099
    IssueSessionToken(ctx Context, req SessionTokenRequest) (SignedSessionToken, error) // F-088, F-119
    GetSessionStatus(ctx Context, req SessionStatusRequest) (SessionStatus, error) // F-101
}

type ArtifactControlAPI interface {
    PublishPipelineVersion(ctx Context, req PublishPipelineRequest) (PipelineVersionRef, error) // F-092, F-015
    ResolveRollout(ctx Context, req RolloutResolveRequest) (RolloutDecision, error) // F-093
    RollbackPipeline(ctx Context, req RollbackRequest) (RolloutDecision, error) // F-100
}

type PolicyAdmissionAPI interface {
    ResolvePolicy(ctx Context, req PolicyResolveRequest) (PolicyDecision, error) // F-056, F-066
    EvaluateAdmission(ctx Context, req AdmissionRequest) (DecisionOutcome, error) // F-098, F-050
}

type GovernanceAPI interface {
    AppendAuditRecord(ctx Context, rec AuditRecord) error // F-097
    GetCompatibilityWindow(ctx Context, req CompatibilityRequest) (CompatibilityWindow, error) // F-162
}
```

## 3) Data model: core entities and relations

### Core entities

- `Tenant`: isolation root for policy, quota, and security context (`F-114`, `F-115`)
- `PipelineSpecVersion`: immutable pipeline artifact version (`F-015`, `F-092`)
- `ExecutionProfileVersion`: deterministic default profile (`F-078`)
- `RolloutPolicyVersion`: selection and rollback rules (`F-093`, `F-100`)
- `PolicyBundleVersion`: provider/cost/circuit/degrade policy version (`F-056`, `F-057`, `F-058`, `F-066`)
- `RoutingViewSnapshot`: versioned placement/routing state used by adapters/runtime (`F-094`, `F-159`)
- `PlacementLease`: authority proof (`epoch`, token ref, expiry) for single-writer semantics (`F-095`, `F-156`)
- `AdmissionPolicySnapshot`: quotas/rate-limit/fairness context (`F-098`, `F-050`, `F-174`)
- `SessionRoute`: endpoint + token metadata returned at session bootstrap (`F-099`, `F-088`, `F-119`)
- `ResolvedTurnPlan`: immutable turn-scoped execution contract (`F-149`, `F-081`)
- `DecisionOutcome`: deterministic control outcomes (`admit`, `reject`, `defer`, `shed`, authority outcomes) (`F-141`)
- `AuditRecord`: immutable control-plane change trail (`F-097`, `F-165`)

### Relation model (contract-level)

1. `Tenant` owns version namespaces for `PipelineSpecVersion`, `ExecutionProfileVersion`, and `PolicyBundleVersion` (`F-114`, `F-102`).
2. `RolloutPolicyVersion` selects one effective `PipelineSpecVersion` for a session/turn (`F-093`, `F-100`).
3. `RoutingViewSnapshot` + `PlacementLease` define authoritative runtime placement for a scope (`F-094`, `F-095`, `F-156`, `F-159`).
4. `PolicyBundleVersion` + provider-health snapshot produce `ProviderBindings` and `AllowedAdaptiveActions` in `ResolvedTurnPlan` (`F-056`, `F-060`, `F-066`, `F-149`).
5. `ResolvedTurnPlan` references all required snapshots via `SnapshotProvenance` and is immutable for turn duration (`F-149`, `F-153`).
6. `DecisionOutcome` is emitted at explicit phase/scope boundaries and linked to session/turn identities (`F-141`, `F-139`).
7. Every mutating artifact or control decision path appends an `AuditRecord` with actor, reason, and approval metadata (`F-097`, `F-165`, `F-170`).

## 4) Extension points: plugins, custom nodes, custom runtimes

Contract extension surfaces:

1. Policy plugins:
   - Extend provider selection, fallback, and degrade logic through policy decision outputs, not runtime side channels.
   - Must produce deterministic outcomes under fixed snapshot inputs (`F-056`, `F-057`, `F-066`).

2. Custom/external node onboarding:
   - `NodePluginDescriptor` identifies package/version/capabilities for custom nodes.
   - `ExternalNodeBoundaryRef` declares isolation mode, resource/time limits, and observability injection requirements (`F-121`, `F-122`, `F-123`, `F-124`, `F-171`).

3. Custom runtime implementations:
   - `RuntimeCapabilitySnapshot` declares supported transport/provider/security/conformance capabilities frozen at turn start.
   - Enables capability-aware routing/admission/plan resolution without changing core contracts (`F-161`, `F-164`).

4. Transport integration plugins:
   - `SessionRoute` and token contracts are transport-agnostic; adapters map these into transport-native auth/session flows (`F-086`, `F-088`, `F-099`).

5. Snapshot source plugins:
   - Registry/routing/policy/provider-health data sources can vary (file, API, cached view), but must output the same snapshot reference schema and staleness semantics (`F-149`, `F-159`).

## 5) Design invariants (must preserve)

1. `ResolvedTurnPlan` is immutable after turn open; control-plane/policy changes apply only at declared boundaries (`F-149`, `F-139`).
2. `scope=turn` outcomes/events must include `turn_id` (`F-139`, `F-141`).
3. Pre-turn `admit/reject/defer` outcomes must not emit turn terminal signals (`abort`/`close`) (`F-141`).
4. Authority safety requires lease epoch validation on ingress and egress; stale authority must be rejected deterministically (`F-095`, `F-156`).
5. Routing and authority decisions are coupled: route metadata is invalid without matching authority context (`F-094`, `F-095`, `F-099`).
6. Every accepted turn must carry complete snapshot provenance references (`F-149`, `F-153`).
7. Snapshot references used for turn resolution must be replay-visible (`F-149`, `F-151`).
8. Allowed adaptive actions are explicit, bounded, and deduplicated in the resolved plan (`F-056`, `F-057`, `F-066`).
9. Control-plane decisions are versioned artifacts; in-place mutation of immutable versions is forbidden (`F-015`, `F-092`, `F-102`).
10. Session tokens are short-lived, signed, and verifiable by adapters/runtime (`F-088`, `F-119`).
11. Tenant scope is mandatory for policy, quota, and security resolution (`F-114`, `F-115`, `F-050`).
12. Compatibility/skew checks must reject unsupported runtime-control-plane-schema combinations (`F-162`, `F-163`).
13. Audit records for pipeline/policy/rollout changes are append-only and immutable (`F-097`, `F-165`, `F-170`).
14. Extension plugins cannot bypass authority, admission, or tenant-isolation checks (`F-121`, `F-124`, `F-156`, `F-114`).
15. External/custom node contracts must include explicit isolation and timeout/resource-bound declarations (`F-122`, `F-123`, `F-124`, `F-171`).
16. Control-plane API contracts must remain transport-agnostic and provider-agnostic at the shared type boundary (`F-086`, `F-055`, `F-083`, `F-084`, `F-085`).

## 6) Tradeoffs and alternatives

| Tradeoff | Preferred contract direction | Alternative | Why this direction |
| --- | --- | --- | --- |
| Snapshot-distributed turn-start inputs vs synchronous control-plane RPC | Snapshot references inside `ResolvedTurnPlan` | Direct RPC on each turn-open | Preserves low-latency runtime locality and replayable decision provenance (`F-149`, `F-159`). |
| Epoch+token authority model vs epoch-only | First-class lease token + epoch contract | Epoch-only authority checks | Better split-brain and tamper resistance across transport/runtime boundaries (`F-095`, `F-119`, `F-156`). |
| Explicit `SessionRoute` API vs transport-specific session bootstrap contracts | One transport-agnostic route+token schema | Per-adapter bespoke route contracts | Avoids adapter drift and keeps CP contract stable across LiveKit/WebSocket/telephony (`F-086`, `F-088`, `F-099`). |
| Strict immutable artifact versions vs mutable-in-place control configs | Append-only versioned refs + rollback records | Mutable current-state docs | Improves auditability, rollback safety, and replay correctness (`F-015`, `F-097`, `F-100`, `F-102`). |
| Contract-first extension descriptors vs ad-hoc plugin metadata | Typed plugin/runtime capability descriptors | Unstructured plugin config blobs | Keeps extensibility testable and conformance-enforceable (`F-121`, `F-124`, `F-161`, `F-164`). |
| Centralized admission-only at session start vs shared CP+runtime admission boundaries | CP bootstrap decision + runtime local scheduling enforcement | CP-only hot-path admission checks | Aligns with PRD requirement that hot-path enforcement remains runtime-local while retaining CP authority (`F-098`, `F-049`, `F-050`, `F-172`). |
| Split logical modules in one Go package vs immediate multi-package split | Logical module boundaries in docs/types first | Immediate physical split into many packages | Reduces migration risk while allowing incremental contract hardening and compatibility controls (`F-162`, `F-163`). |

## 7) Code-verified progress and divergence (as of 2026-02-16)

Code audited:
- `api/controlplane/types.go`
- `api/controlplane/types_test.go`
- `test/contract/fixtures_test.go`
- `test/contract/fixtures/{decision_outcome,turn_transition,resolved_turn_plan}`
- `docs/ContractArtifacts.schema.json`

| Contract area | Current code status | Current validation coverage | Divergence from target or schema | Feature evidence |
| --- | --- | --- | --- | --- |
| `DecisionOutcome` | Implemented with strict enum, phase/scope, emitter, authority-epoch, and turn-id rules. | Package unit tests in `api/controlplane/types_test.go` + contract fixtures. | Schema includes optional `timestamp_ms`, but struct does not currently expose `timestamp_ms`. | `F-141`, `F-095`, `F-156`, `F-098` |
| `TurnTransition` | Implemented legal deterministic lifecycle-edge validator. | Contract fixtures cover minimal valid/invalid transitions. | No package-local unit test matrix for all legal/illegal transitions. | `F-139`, `F-138` |
| `ResolvedTurnPlan` core (`Budgets`, `FlowControl`, `SnapshotProvenance`, `RecordingPolicy`, `Determinism`) | Implemented with nested validation and replay-oriented invariants. | Contract fixtures cover minimal valid/invalid plan shape. | Package-local tests do not yet cover broad invalid-path matrix (actions, merge-rule version, lane policy combinations). | `F-149`, `F-150`, `F-151`, `F-153` |
| `SyncDropPolicy` in edge buffering | Implemented (`policy`, `scope`, group requirement, sync-domain requirement). | Indirectly validated via resolved-plan fixture path. | `group_by` enum is stricter in schema (`sync_id|event_id|custom`) than Go validator (currently any non-empty string). | `F-043`, `F-044`, `F-054` |
| `StreamingHandoffPolicy` | Implemented as optional `ResolvedTurnPlan` field with validation. | No package-local tests and no contract fixtures for this field. | `docs/ContractArtifacts.schema.json` currently does not model `resolved_turn_plan.streaming_handoff`. | `F-081`, `F-149`, `F-176` |
| Target control-plane API surface (`LeaseToken`, `SessionRoute`, route/token/session status contracts) | Not implemented as first-class shared types in `api/controlplane`. | N/A in this package. | Still only represented as target architecture in this guide; runtime/control-plane interfaces remain outside this package. | `F-088`, `F-094`, `F-095`, `F-099`, `F-101`, `F-119` |

## 8) Implementation-ready backlog for `api/controlplane`

### P0: close contract/schema parity gaps

1. Resolve `DecisionOutcome.timestamp_ms` parity.
   - Files: `api/controlplane/types.go`, `docs/ContractArtifacts.schema.json`, `test/contract/fixtures/decision_outcome/*`.
   - Decision: either add optional `timestamp_ms` to Go type or remove it from schema.
   - Done when: strict fixture decode and schema validation agree on accepted fields (`F-141`).

2. Enforce `SyncDropPolicy.group_by` enum parity.
   - Files: `api/controlplane/types.go`, `api/controlplane/types_*_test.go`, `test/contract/fixtures/resolved_turn_plan/*`.
   - Done when: Go validator rejects out-of-enum values and fixtures include at least one invalid `group_by` case (`F-043`, `F-044`).

3. Align `streaming_handoff` across type/schema/fixtures.
   - Files: `docs/ContractArtifacts.schema.json`, `test/contract/fixtures/resolved_turn_plan/*`, `api/controlplane/types_test.go` (or split test files).
   - Done when: schema accepts/rejects the same handoff forms as Go validation (`F-081`, `F-149`, `F-176`).

### P1: raise validator test completeness to implementation-ready

1. Add package-local table tests for all major validators.
   - Files to add: `api/controlplane/turn_transition_test.go`, `api/controlplane/resolved_turn_plan_test.go`, `api/controlplane/flow_buffer_policy_test.go`, `api/controlplane/recording_determinism_test.go`.
   - Done when: each validator has valid + invalid cases for boundary conditions and schema-compatibility branches (`F-138`, `F-139`, `F-149`, `F-153`).

2. Keep contract fixture suite as cross-package conformance and expand beyond minimal cases.
   - Files: `test/contract/fixtures/turn_transition/*`, `test/contract/fixtures/resolved_turn_plan/*`, `test/contract/fixtures/decision_outcome/*`.
   - Done when: fixture set includes representative authority, scheduling-point, replay-mode, and flow-control edge cases (`F-141`, `F-149`, `F-150`, `F-151`).

### P2: implement missing target-state shared control-plane API types

1. Introduce first-class shared types for route and authority artifacts.
   - Types to add under `api/controlplane`: `PlacementLease`, `LeaseTokenRef` (or `LeaseTokenClaims`), `SessionRoute`, `TransportEndpointRef`, `SessionStatus`.
   - Done when: transport/control-plane modules can depend on shared types instead of ad-hoc local structs (`F-088`, `F-094`, `F-095`, `F-099`, `F-101`, `F-119`).

2. Introduce minimal version-reference types for artifact APIs.
   - Types to add: `PipelineVersionRef`, `PolicyVersionRef`, `RolloutDecision`.
   - Done when: publish/resolve/rollback contract surfaces are type-stable and replay/audit linkable (`F-092`, `F-093`, `F-100`, `F-102`, `F-097`).

## 9) Change checklist

1. Keep `api/controlplane/types.go` aligned with `docs/ContractArtifacts.schema.json` and call out intentional drifts in this guide.
2. Maintain explicit feature-ID evidence for every newly introduced contract rule or type.
3. Update both package-local unit tests and contract fixtures when changing validation semantics.
4. Re-run:
   - `go test ./api/controlplane ./test/contract`
   - `make verify-quick`

## Related docs

- `docs/PRD.md`
- `docs/RSPP_features_framework.json`
- `docs/rspp_SystemDesign.md`
- `docs/ContractArtifacts.schema.json`
- `internal/controlplane/controlplane_module_guide.md`
