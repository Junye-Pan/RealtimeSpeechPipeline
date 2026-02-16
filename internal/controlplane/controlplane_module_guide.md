# Internal Control Plane Guide (Implementation-Ready)

This document is the implementation-ready control-plane architecture guide for RSPP.

Code-verification timestamp: **February 16, 2026**.

## Source of truth used

- `docs/PRD.md`
- `docs/RSPP_features_framework.json`
- `docs/rspp_SystemDesign.md`
- `internal/controlplane/*`
- `internal/runtime/turnarbiter/controlplane_bundle.go`
- `internal/runtime/turnarbiter/controlplane_backends.go`
- `internal/runtime/planresolver/resolver.go`
- `cmd/rspp-control-plane/main.go`

## 1) Module list with responsibilities (current progress + divergence)

| Module | Path | Current implementation status | Divergence from target | Feature refs |
| --- | --- | --- | --- | --- |
| `CP-01` Pipeline Registry | `internal/controlplane/registry` | Implemented baseline resolver with defaults and backend override (`ResolvePipelineRecord`). | No publish/mutate API yet; immutability lifecycle is assumed, not enforced by control-plane write path. | `F-015`, `F-092` |
| `CP-02` Spec Normalizer | `internal/controlplane/normalizer` | Implemented baseline normalization and strict MVP profile gate (`simple` only). | No execution-profile version surface (`simple/v1`) or layered override resolution yet. | `F-078`, `F-079`, `F-080`, `F-081` |
| `CP-03` Graph Compiler | `internal/controlplane/graphcompiler` | Implemented deterministic compile output (`graph_ref`, snapshot, fingerprint). | Compiler is reference/fingerprint normalization only; no full graph semantic compilation/validation pipeline yet. | `F-002`, `F-005`, `F-008` |
| `CP-04` Rollout Resolver | `internal/controlplane/rollout` | Implemented effective version resolution from requested/registry/backend snapshot. | No first-class canary/percentage/allowlist rollout rule engine in code path yet. | `F-093`, `F-100`, `F-102` |
| `CP-05` Routing View Publisher | `internal/controlplane/routingview` | Implemented snapshot reference resolver with required routing/admission/ABI refs. | Snapshot does not currently carry concrete runtime endpoint selection metadata or client route payload fields. | `F-094`, `F-099`, `F-159` |
| `CP-06` Provider Health Snapshot | `internal/controlplane/providerhealth` | Implemented snapshot reference resolver (`ProviderHealthSnapshot`). | No in-module health aggregation, SLO windows, or incident model; currently a snapshot pointer seam. | `F-060`, `F-112` |
| `CP-07` Policy Evaluation | `internal/controlplane/policy` | Implemented deterministic allowed-action normalization and snapshot reference output. | No built-in provider selection/cost/circuit policy engine; behavior primarily delegated to backends. | `F-056`, `F-057`, `F-058`, `F-066`, `F-102` |
| `CP-08` Lease Authority | `internal/controlplane/lease` | Implemented epoch validity + authorization decision surface with normalized reasons. | No signed lease token/expiry/renewal model yet; authority is represented by epoch + booleans. | `F-095`, `F-156` |
| `CP-09` Admission Decision | `internal/controlplane/admission` | Implemented pre-turn `admit/reject/defer` decision with deterministic defaults and scope normalization. | Fairness/scheduling-point policies are not modeled here yet (currently pre-turn baseline only). | `F-098`, `F-049`, `F-050`, `F-172`, `F-174` |
| `CP-10` Snapshot Distribution | `internal/controlplane/distribution` | Implemented file + HTTP adapters, deterministic error taxonomy, stale classification, HTTP failover/retry/cache/stale-serving bounds, retention snapshot loading. | No broker/API service abstraction above distribution adapters yet. | `F-149`, `F-159`, `NF-028` |
| `CP-11` TurnStart Resolver Facade | Current location: `internal/runtime/turnarbiter/controlplane_bundle.go` | Implemented composition order and immutable turn-start bundle assembly with provenance + CP admission/lease decisions. | Architecturally lives in runtime package; target ownership should move to `internal/controlplane/resolution`. | `F-149`, `F-141`, `F-156` |
| `CP-12` Backend Fallback Wiring | `internal/runtime/turnarbiter/controlplane_backends.go` | Implemented per-service fallback wrappers: non-stale backend errors fall back to defaults; stale snapshot errors propagate. | This resilience choice is implicit in runtime wiring, not declared as explicit control-plane policy contract yet. | `F-159`, `NF-028` |
| `CP-13` Control-plane process surface | `cmd/rspp-control-plane/main.go` | Scaffold only (`fmt.Println`). | No control-plane API server, mutation/audit endpoints, route/token endpoints, or operational status APIs yet. | `F-097`, `F-099`, `F-101`, `F-165` |

## Current turn-start composition order (implemented)

Current order in `internal/runtime/turnarbiter/controlplane_bundle.go`:
1. `registry`
2. `normalizer`
3. `rollout`
4. `graphcompiler`
5. `routingview`
6. `providerhealth`
7. `policy`
8. `lease`
9. `admission`

This order is implemented and validated by tests under `internal/runtime/turnarbiter/controlplane_bundle_test.go` and `internal/runtime/turnarbiter/controlplane_backends_test.go`.

## 2) Key interfaces (implementation-ready pseudocode)

### Current implemented contracts (shape-matched to code)

```go
// Implemented seam (currently under runtime package).
type TurnStartBundleResolver interface {
    ResolveTurnStartBundle(in TurnStartBundleInput) (TurnStartBundle, error)
}

type TurnStartBundleInput struct {
    SessionID                string
    TurnID                   string
    RequestedPipelineVersion string
    AuthorityEpoch           int64
}

type TurnStartBundle struct {
    PipelineVersion        string
    GraphDefinitionRef     string
    ExecutionProfile       string
    GraphFingerprint       string
    AllowedAdaptiveActions []string
    SnapshotProvenance     SnapshotProvenance
    HasCPAdmissionDecision bool
    CPAdmissionOutcomeKind OutcomeKind
    CPAdmissionScope       OutcomeScope
    CPAdmissionReason      string
    HasLeaseDecision       bool
    LeaseAuthorityEpoch    int64
    LeaseAuthorityValid    bool
    LeaseAuthorityGranted  bool
}
```

```go
// Implemented module backends.
type RegistryBackend interface { ResolvePipelineRecord(version string) (PipelineRecord, error) }
type RolloutBackend interface { ResolvePipelineVersion(in ResolveVersionInput) (ResolveVersionOutput, error) }
type RoutingBackend interface { GetSnapshot(in RoutingInput) (RoutingSnapshot, error) }
type PolicyBackend interface { Evaluate(in PolicyInput) (PolicyOutput, error) }
type ProviderHealthBackend interface { GetSnapshot(in ProviderHealthInput) (ProviderHealthOutput, error) }
type GraphCompilerBackend interface { Compile(in GraphCompileInput) (GraphCompileOutput, error) }
type AdmissionBackend interface { Evaluate(in AdmissionInput) (AdmissionOutput, error) }
type LeaseBackend interface { Resolve(in LeaseInput) (LeaseOutput, error) }
```

### Required interface upgrades (next implementation step)

```go
// Control-plane-owned resolver target package: internal/controlplane/resolution
type TurnStartResolver interface {
    ResolveTurnStartBundle(ctx Context, in TurnStartRequest) (TurnStartBundle, error)
}

type TurnStartRequest struct {
    TenantID                 string // NEW: needed for F-114/F-050/F-056 policy scope
    SessionID                string
    TurnID                   string
    RequestedPipelineVersion string
    RequestedAuthorityEpoch  int64
    TransportKind            string // NEW: needed for F-161 capability snapshot
}
```

```go
// First-class session route/token broker for adapters.
type SessionRouteBroker interface {
    ResolveSessionRoute(ctx Context, in SessionRouteRequest) (SessionRoute, error)
    IssueSessionToken(ctx Context, in SessionTokenRequest) (SignedSessionToken, error)
}
```

## 3) Data model: core entities and relations

### Current entities in code

- `PipelineRecord` (`registry`)
- `ResolveVersionOutput` (`rollout`)
- `routingview.Snapshot`
- `policy.Output`
- `providerhealth.Output`
- `admission.Output`
- `lease.Output`
- `controlplane.SnapshotProvenance`
- `TurnStartBundle` (runtime-consumed CP bundle)
- `distribution.fileArtifact` sections:
  - `registry`
  - `rollout`
  - `routing_view`
  - `policy`
  - `provider_health`
  - `graph_compiler`
  - `admission`
  - `lease`
  - `retention`

### Required core entities for target architecture

- `PipelineSpecVersion` (immutable write path)
- `ExecutionProfileVersion` (`simple/v1` and future)
- `RolloutPolicyVersion` (canary/percentage/allowlist)
- `RoutingPlacement` (region/runtime endpoint payload)
- `PlacementLeaseToken` (epoch + token + expiry)
- `PolicyBundleVersion`
- `AdmissionPolicyVersion` + fairness keys
- `CapabilitySnapshot` (transport/provider/runtime)
- `SessionRoute` + `SignedSessionToken`
- `AuditRecord` (mutation provenance)

### Relationship rules to enforce

1. `PipelineSpecVersion` + `ExecutionProfileVersion` resolve to one graph definition per turn (`F-001`, `F-002`, `F-078`).
2. `RolloutPolicyVersion` selects effective pipeline version without mutating prior artifacts (`F-093`, `F-100`).
3. `RoutingPlacement` + `PlacementLeaseToken` jointly define authority for that turn/session (`F-095`, `F-156`).
4. `PolicyBundleVersion` + `ProviderHealthSnapshot` produce deterministic adaptive action set (`F-057`, `F-066`).
5. `SnapshotProvenance` must be complete for every accepted turn (`F-149`, `NF-028`).

## 4) Extension points: plugins, custom nodes, custom runtimes

### Implemented extension points

- Per-module snapshot backends via interfaces in each `internal/controlplane/*` package.
- Distribution source extensibility via file/HTTP adapters in `internal/controlplane/distribution`.
- Runtime fallback wrappers in `internal/runtime/turnarbiter/controlplane_backends.go`.

### Missing extension points to implement

- Node catalog and package governance plugin boundary (`F-121`, `F-171`, `NF-036`).
- External runtime capability plugin surface (`F-161`, `F-164`).
- Policy plugin contract with deterministic downgrade semantics on missing dynamic signals (`F-066`).
- Session route/token broker plugin for transport adapters (`F-099`, `F-119`).

## 5) Design invariants (must preserve)

1. Turn-start bundle must contain complete snapshot provenance and be immutable per turn (`F-149`, `NF-028`).
2. Pre-turn CP decisions (`admit/reject/defer`) must not emit accepted-turn terminal events (`F-141`).
3. Authority validation must reject stale/deauthorized epochs deterministically (`F-156`, `NF-027`).
4. Routing snapshot resolution must fail deterministically on stale snapshots (`F-159`).
5. Backend stale-snapshot errors are propagated; non-stale backend errors may only fall back via explicit policy (`F-159`).
6. Execution profile unsupported in MVP must be rejected deterministically (`F-078`, current `simple`-only gate).
7. Rollout fallback order must be deterministic (backend -> registry -> requested only where allowed).
8. Allowed adaptive actions must be normalized into canonical order and valid action set (`F-057`, `F-066`).
9. Admission scope normalization must preserve tenant/session boundaries (`F-114`, `F-098`).
10. Lease decisions must emit normalized reason taxonomy for stale/deauthorized/authorized outcomes (`F-156`).
11. Distribution HTTP behavior must preserve ordered endpoint failover and bounded stale-serving semantics (`F-159`).
12. Snapshot artifact schema version must be validated before service resolution.
13. Retention snapshot resolution must classify stale vs missing deterministically (`F-116`, `NF-014`).
14. Control-plane mutation surfaces must eventually emit auditable records before becoming authoritative (`F-097`, `F-165`).
15. Compatibility/skew policy must be explicit before multi-version runtime-control-plane rollouts (`NF-032`, `F-162`).
16. Control-plane token material must never leak secrets in logs/events (`F-117`, `NF-011`).

## 6) Major tradeoffs and alternatives

| Tradeoff | Current choice | Alternative | Impact |
| --- | --- | --- | --- |
| Runtime-located CP resolver facade | `CP-11` resolver lives in `internal/runtime/turnarbiter` | Move to `internal/controlplane/resolution` now | Current state is fast to iterate, but blurs ownership boundaries and makes CP API evolution harder. |
| Fallback on non-stale backend errors | `controlplane_backends.go` swallows non-stale backend errors per service and uses defaults | Hard-fail all backend errors | Current improves availability, but risks silent behavior drift unless explicitly audited. |
| Snapshot-distributed decision inputs | File/HTTP snapshot distribution with stale classification | Synchronous CP RPC at turn-open | Snapshot mode preserves runtime locality; RPC mode gives fresher data but adds hot-path dependency. |
| Minimal lease shape (epoch+booleans) | Simple lease decision output | Signed lease token with expiry/renew | Current simplifies MVP, but cannot fully satisfy token-level authority proofs. |
| Rollout resolver simplicity | Version mapping only | Full canary/percent/allowlist engine | Current is deterministic and small, but does not meet all rollout controls (`F-093`). |
| Policy engine seam-first | Normalization + backend delegation | In-process policy DSL evaluator | Current enables backend flexibility, but leaves many policy guarantees externalized. |
| Control-plane binary scaffold | `cmd/rspp-control-plane/main.go` placeholder | Full API/control service | Current keeps repo compile path simple; no production CP process capabilities yet. |

## Progress/divergence checklist (implementation-ready)

### P0: Close architecture ownership gaps

1. Move turn-start resolver from runtime to control-plane ownership.
   - Add: `internal/controlplane/resolution/resolver.go`
   - Keep runtime seam: `internal/runtime/turnarbiter/controlplane_bundle.go` as adapter only.
   - Done when: all resolver composition tests pass from new package and runtime imports only interface adapter.

2. Make fallback policy explicit and configurable.
   - Update: `internal/runtime/turnarbiter/controlplane_backends.go`
   - Add policy knob: strict mode (`fail_on_backend_error=true`) vs availability mode (current behavior).
   - Done when: tests cover stale and non-stale branches for both modes.

3. Introduce tenant-aware turn-start input.
   - Update: turn-start input structs and all call sites.
   - Files: `internal/runtime/turnarbiter/controlplane_bundle.go`, `internal/runtime/turnarbiter/arbiter.go`, related tests.
   - Done when: tenant scope reaches policy/admission consistently and tests assert scope behavior.

### P1: Meet MVP control-plane feature contracts more completely

1. Rollout policy semantics.
   - Expand `internal/controlplane/rollout` to support canary/percentage/allowlist policy snapshots.
   - Done when: deterministic test matrix for percent bucketing + rollback + allowlist overrides is added.

2. Route metadata + signed session tokens.
   - Add package `internal/controlplane/sessionroute`.
   - Link to transport adapter contract for token verification.
   - Done when: route+token outputs are versioned and verified in integration tests.

3. Lease token enrichment.
   - Extend `internal/controlplane/lease` output with token id/expiry metadata.
   - Done when: stale/expired token cases are explicitly test-covered.

4. Control-plane process bootstrap.
   - Replace scaffold in `cmd/rspp-control-plane/main.go` with service bootstrap wiring.
   - Done when: health/readiness endpoints and snapshot source config are live.

### P2: Post-MVP modules called out in target architecture

1. `internal/controlplane/compat` for skew policy and deprecation lifecycle (`F-162`, `F-163`).
2. `internal/controlplane/audit` for mutation provenance (`F-097`, `F-165`).
3. `internal/controlplane/migration` for drain/migrate strategy (`F-113`, `F-157`).
4. `internal/controlplane/capability` for turn-start capability freeze (`F-161`).

## Validation and tests

Targeted tests executed successfully for code-verified status:
- `go test ./internal/controlplane/admission ./internal/controlplane/graphcompiler ./internal/controlplane/lease ./internal/controlplane/normalizer ./internal/controlplane/policy ./internal/controlplane/providerhealth ./internal/controlplane/registry ./internal/controlplane/rollout ./internal/controlplane/routingview`
- `go test ./internal/controlplane/distribution -run 'Test(FileAdapter|LoadRetentionPolicySnapshotFromFile)'`
- `go test ./internal/runtime/turnarbiter -run 'Test(ResolveTurnStartBundle|IsStaleSnapshotResolutionError|NewControlPlaneBundleResolverWithBackends$|NewControlPlaneBundleResolverWithBackendsFallsBackPerService|NewControlPlaneBundleResolverWithBackendsPropagatesStaleErrors)'`

Note: full HTTP-based adapter tests requiring local socket listeners are environment-constrained in this sandbox.

## Related docs

- `docs/PRD.md`
- `docs/RSPP_features_framework.json`
- `docs/rspp_SystemDesign.md`
- `docs/CIValidationGates.md`
- `docs/SecurityDataHandlingBaseline.md`
- `docs/ContractArtifacts.schema.json`
