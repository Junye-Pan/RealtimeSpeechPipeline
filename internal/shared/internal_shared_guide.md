# Internal Shared Guide

This folder is the cross-domain helper layer for internal modules. It provides deterministic primitives reused by runtime, control-plane, observability, and tooling, while staying non-authoritative for domain policy.

Primary evidence basis:
- PRD core contracts: `docs/PRD.md` sections `4.1.1` to `4.1.9`, `4.3`, `5.1`.
- Repository target architecture and ownership boundaries: `docs/rspp_SystemDesign.md` sections `3`, `4`, `6`.
- Feature catalog traceability: `docs/RSPP_features_framework.json`.

Boundary rule:
- Shared helpers provide reusable primitives only.
- Runtime/control-plane/observability/tooling remain policy owners.
- If logic encodes domain decision policy, it must live in the owning domain package.

## 1) Target modules and responsibilities

| Module (target path) | Responsibilities | Explicit non-ownership | Feature evidence |
| --- | --- | --- | --- |
| `internal/shared/config/profile` | Execution-profile defaults registry, deterministic default injection metadata (`defaulting_source`), profile version checks | Does not pick runtime policy outcomes at execution time | `F-078`, `F-079`, `F-081`, PRD `4.1.1`, `4.3` |
| `internal/shared/config/layering` | Layered config merge engine (`defaults -> env -> tenant -> session`), conflict resolution, validation primitives | Does not own tenant policy authoring | `F-080`, `NF-015`, PRD `4.1.1`, `4.1.9` |
| `internal/shared/config/resolvedview` | Generic resolved-view renderers for effective config/spec snapshots (debug + tooling) | Does not own spec compilation | `F-081`, `F-126`, `F-129` |
| `internal/shared/types/identity` | Canonical internal ID/value objects (`session_id`, `turn_id`, `event_id`, `provider_invocation_id`, `lease_epoch`, idempotency keys) | Does not own authority decisions | `F-013`, `F-155`, `F-156`, `F-158`, PRD `4.1.8` |
| `internal/shared/types/eventmeta` | Lane enums, ordering markers, sync/discontinuity metadata, compatibility-safe envelope fragments used internally | Does not own external API schemas under `api/*` | `F-020` to `F-022`, `F-028`, `F-142`, `F-146`, PRD `4.1.3`, `4.2` |
| `internal/shared/types/determinism` | Replay-critical metadata carriers: determinism markers, plan-hash refs, recording-level markers, downgrade transition records | Does not own replay execution engines | `F-149` to `F-154`, `F-176`, `NF-009`, PRD `4.1.6` |
| `internal/shared/errors/taxonomy` | Stable internal error-code catalog and severity/action-class metadata | Does not own node/provider-specific error policy | `F-131`, `F-136`, `NF-010`, PRD `4.4.4` |
| `internal/shared/errors/normalize` | Cross-domain error normalization adapters (provider/runtime/transport to normalized classes), deterministic default-action mapping helpers | Does not choose rollout/admission behavior | `F-131` to `F-137`, `F-160`, PRD `4.1.7`, `4.4.4` |
| `internal/shared/util/timebase` | Pure helpers for monotonic/wall/media time conversion, canonical metric-anchor calculations, skew-safe comparisons | Does not own transport jitter control loops | `NF-018`, `NF-024` to `NF-026`, PRD `4.1.5`, `4.4.2` |
| `internal/shared/util/canonical` | Deterministic hashing, stable ordering, bounded merge utilities, serialization canonicalization for reproducibility | Does not own lane scheduling or merge policy choices | `F-005`, `F-008`, `F-150`, `F-152`, `NF-009` |
| `internal/shared/util/security` | Redaction, safe-context shaping, secret masking primitives reused by logging/debug/replay paths | Does not own authn/authz policy | `F-117`, `F-118`, `F-073`, `NF-011`, `NF-014` |
| `internal/shared/extensions` | Shared extension descriptors and registry contracts for plugins/custom-node boundaries/custom-runtime adapters | Does not execute plugin policy itself | `F-011`, `F-121` to `F-124`, `F-164`, PRD `4.1.7`, `4.1.8` |

Current scaffold mapping:
- `internal/shared/config`: host `profile`, `layering`, `resolvedview`.
- `internal/shared/errors`: host `taxonomy`, `normalize`.
- `internal/shared/types`: host `identity`, `eventmeta`, `determinism`.
- `internal/shared/util`: host `timebase`, `canonical`, `security`.
- `internal/shared/extensions`: new shared submodule for extension contracts.

### 1.1 Current codebase progress and divergence (audited)

Repository snapshot status:
- `internal/shared` currently contains guides only (no `.go` implementation packages yet).
- Current Go code paths do not import `internal/shared` directly.

| Target module | Current implementation evidence | Divergence to target | Implementation action |
| --- | --- | --- | --- |
| `config/profile` | `internal/controlplane/registry/registry.go`, `internal/controlplane/normalizer/normalizer.go`, `internal/runtime/planresolver/resolver.go` | Profile defaults and enforcement are split across control-plane and runtime; no shared profile registry (`F-078`, `F-081`) | Introduce `internal/shared/config/profile/simple_v1.go`; both CP normalizer and runtime planresolver consume the same default source |
| `config/layering` | Env/default parsing patterns in `internal/controlplane/distribution/http_adapter.go` and provider adapters | No single deterministic layering engine across defaults/env/tenant/session (`F-080`, `NF-015`) | Add `internal/shared/config/layering/resolver.go` and migrate CP/runtime entry config resolution to it |
| `config/resolvedview` | No shared resolved-view renderer | Feature-intended resolved view lacks shared representation (`F-081`, `F-126`) | Add `internal/shared/config/resolvedview/render.go` + integration hooks for CLI/local-runner |
| `types/identity` | `internal/runtime/identity/context.go`; identity fields in `api/eventabi/types.go`, `api/controlplane/types.go` | Runtime identity helper exists, but not shared for cross-domain reuse (`F-013`, `F-156`, `F-158`) | Add `internal/shared/types/identity/bundle.go`; runtime identity service adapts to shared type |
| `types/eventmeta` | `internal/runtime/eventabi/gateway.go` normalization + `api/eventabi/types.go` validation | Event metadata normalization duplicated in runtime helpers; no shared internal envelope helpers (`F-020`, `F-142`) | Add `internal/shared/types/eventmeta/normalize.go` and use from runtime buffering/flowcontrol/nodehost emitters |
| `types/determinism` | `internal/runtime/determinism/service.go`, determinism structs in `api/controlplane/types.go` | Determinism shaping exists but runtime-local; no shared deterministic metadata carrier (`F-149`, `F-150`, `F-176`) | Add `internal/shared/types/determinism/envelope.go`; runtime and observability use shared type |
| `errors/taxonomy` | `internal/controlplane/distribution/file_adapter.go` (`BackendError`/`ErrorCode`), `internal/runtime/provider/contracts/contracts.go` (`OutcomeClass`) | Multiple error taxonomies exist but are not unified (`F-131`, `NF-010`) | Add `internal/shared/errors/taxonomy/codes.go` with mapping adapters per domain |
| `errors/normalize` | `providers/common/httpadapter/httpadapter.go` (`NormalizeStatus`, `NormalizeNetworkError`) | Provider-level normalization exists, but cross-domain normalize/default-action contract is missing (`F-131`, `F-136`, `F-137`) | Add `internal/shared/errors/normalize/mapper.go`; runtime/provider/cp wrappers delegate |
| `util/timebase` | `internal/runtime/timebase/timebase_service_module_guide.md` scaffold only | Required timebase helpers are not implemented (`NF-018`, `NF-024`, `NF-025`, `NF-026`) | Implement `internal/shared/util/timebase/clock.go` + `anchors.go`; consume from runtime metric-anchor emitters |
| `util/canonical` | Hash/normalization helpers spread in `internal/runtime/planresolver/resolver.go`, `internal/runtime/determinism/service.go`, `internal/runtime/identity/context.go`; repeated helper patterns (`nonNegative` appears across runtime modules) | Canonicalization and normalization behavior is duplicated, increasing drift risk (`F-150`, `NF-009`) | Add `internal/shared/util/canonical/hash.go`, `normalize.go`; replace duplicated local helpers incrementally |
| `util/security` | `internal/security/policy/policy.go`, `internal/observability/timeline/redaction.go`, payload capture masking in `providers/common/httpadapter/httpadapter.go` | Security/redaction primitives exist but are split; shared policy-neutral masking helpers are missing (`F-117`, `F-073`, `NF-011`, `NF-014`) | Add `internal/shared/util/security/redact.go`; keep policy matrix in `internal/security/*` |
| `extensions` | `internal/runtime/externalnode/external_node_boundary_guide.md` scaffold only | No shared extension descriptor/registry contract implementation yet (`F-121` to `F-124`, `F-164`) | Add `internal/shared/extensions/descriptor.go` + `registry.go` with ABI/version checks |

Observed duplication signals (migration priority indicators):
- `defaultString(...)` helper is duplicated across provider adapters (9 locations).
- `nonNegative(...)` helper is duplicated across runtime emitters (7 locations).

Interpretation:
- These duplicates should migrate to shared utility primitives only where they are policy-neutral; domain policy logic stays in domain modules (`NF-019`, `NF-020`).

## 2) Key interfaces (pseudocode)

```go
// config/profile
type ProfileDefaults interface {
    Name() string                      // e.g. "simple"
    Version() string                   // e.g. "v1"
    Apply(spec PipelineSpecInput) (ResolvedDefaults, error)
}

type LayeredConfigResolver interface {
    Resolve(base Defaults, env EnvCfg, tenant TenantCfg, session SessionCfg) (EffectiveConfig, ResolveMeta, error)
}

type ResolveMeta struct {
    DerivedFields map[string]DefaultingSource // field path -> source metadata
    Warnings      []ConfigWarning
}
```

Interface rationale: deterministic profile/default handling and layered resolution (`F-078`, `F-080`, `F-081`, `NF-015`).

```go
// errors
type ErrorTaxonomy interface {
    Classify(err error, ctx ErrorContext) NormalizedError
}

type DefaultActionResolver interface {
    ResolveAction(ne NormalizedError, scope ActionScope) DefaultAction
    // Expected actions include reject, retry, fallback, degrade, abort.
}

type SensitiveContextRedactor interface {
    Redact(fields map[string]any) map[string]any
}
```

Interface rationale: normalized error handling with deterministic default actions and safe context (`F-131`, `F-136`, `F-137`, `NF-010`, `NF-011`).

```go
// types/determinism + util/canonical
type DeterminismEnvelope struct {
    PlanHash            string
    DeterminismSeed     string
    MergeRuleID         string
    MergeRuleVersion    string
    OrderingMarkers     []string
    NonDeterministicRef []string
}

type Canonicalizer interface {
    StableHash(v any) (string, error)
    StableSort[T any](items []T, less func(a, b T) bool) []T
}
```

Interface rationale: replay-stable determinism artifacts and ordering (`F-149`, `F-150`, `F-152`, `F-176`, `NF-009`).

```go
// extensions
type ExtensionDescriptor interface {
    Kind() ExtensionKind // plugin, custom_node, runtime_adapter
    Name() string
    Version() string
    ABI() string
    Capabilities() map[string]any
}

type ExtensionRegistry interface {
    Register(desc ExtensionDescriptor) error
    Resolve(kind ExtensionKind, name string, version string) (ExtensionDescriptor, error)
}
```

Interface rationale: extensibility boundaries with versioned compatibility checks (`F-121`, `F-122`, `F-124`, `F-162`, `F-164`, `NF-032`).

## 3) Data model (core shared entities and relations)

| Entity | Purpose | Key relations | Inline feature refs |
| --- | --- | --- | --- |
| `EffectiveConfig` | Resolved runtime/control helper config with layered provenance | Produced by `LayeredConfigResolver`; consumed by runtime/control-plane startup and turn-plan materialization | `F-078`, `F-080`, `F-081`, `NF-015` |
| `DefaultingSource` | Tracks how each derived field was produced | Attached to `EffectiveConfig`, `ResolvedTurnPlan` metadata surfaces | `F-081`, `F-149` |
| `IdentityBundle` | Canonical IDs/epochs/idempotency keys | Embedded in event metadata, error context, replay records | `F-013`, `F-156`, `F-158` |
| `EventMeta` | Lane, ordering, sync/discontinuity markers | Wrapped by runtime/transport events; used by replay and diagnostics | `F-020`, `F-021`, `F-142`, `F-146` |
| `DeterminismEnvelope` | Replay-critical determinism context references | Linked to accepted-turn evidence and replay cursor stepping | `F-149`, `F-150`, `F-152`, `F-153`, `F-176`, `NF-009` |
| `NormalizedError` | Stable internal error shape (`code`, `class`, `scope`, `retryable`, `safe_context`) | Mapped to `DefaultAction`; emitted via control/telemetry surfaces | `F-131`, `F-136`, `NF-010` |
| `DefaultAction` | Deterministic baseline action for a normalized error | Drives domain handlers without encoding domain-specific policy in shared | `F-131`, `F-133`, `F-137` |
| `ExtensionDescriptor` | Declares plugin/custom-node/custom-runtime adapter compatibility | Registered in `ExtensionRegistry`; referenced by runtime/control-plane binders | `F-121`, `F-122`, `F-123`, `F-124`, `F-164` |

Relations (summary):
- `EffectiveConfig` references many `DefaultingSource`.
- `IdentityBundle` and `EventMeta` compose per-event context.
- `DeterminismEnvelope` references `IdentityBundle` and turn-plan hash lineage.
- `NormalizedError` references `IdentityBundle` and optional `EventMeta`.
- `ExtensionDescriptor` is validated against ABI/version constraints before binding.

## 4) Extension points

1. Plugin extension (`internal/shared/extensions`, `Kind=plugin`, `F-121`, `F-124`, `F-164`)
- Purpose: register reusable helper plugins (e.g., config source adapters, redaction packs, canonicalization providers).
- Contract: explicit ABI version + declared capabilities; no implicit global side effects.

2. Custom node boundary extension (`Kind=custom_node`, `F-011`, `F-121`, `F-122`, `F-123`)
- Purpose: shared descriptor for in-process and out-of-process custom nodes.
- Contract: declared input/output event compatibility, lifecycle hook support, timeout/resource hints, cancellation behavior.

3. Custom runtime adapter extension (`Kind=runtime_adapter`, `F-162`, `NF-032`, `NF-010`)
- Purpose: allow alternative runtime hosts/executors to plug into the same shared contracts (identity, error taxonomy, determinism markers, config defaulting metadata).
- Contract: compatibility check against shared ABI and supported version-skew window.

4. Policy-safe extension lifecycle (`F-149`, `F-161`)
- Register -> validate ABI/version/capabilities -> activate at boundary (session start/turn start) -> deactivate with deterministic fallback.
- No mid-turn mutation of extension behavior for accepted turns.

## 5) Design invariants (must hold)

1. Shared helpers MUST remain non-authoritative for runtime/control-plane policy outcomes (`NF-019`, `NF-020`, PRD `4.1.9`).
2. Every derived config value MUST include provenance metadata (`defaulting_source`) when defaulted (`F-078`, `F-081`).
3. Layered config merge order MUST be deterministic: defaults -> environment -> tenant -> session (`F-080`, `NF-015`).
4. Shared identity fields (`session_id`, `turn_id`, `event_id`, `lease_epoch`) MUST be immutable once emitted (`F-013`, `F-156`, `F-158`).
5. Turn-scoped helpers MUST reject use without valid turn context when turn scope is required (`F-139`, `F-141`, PRD `4.1.2`).
6. Error normalization MUST produce stable code/class/action-class for the same input context (`F-131`, `F-136`, `NF-010`).
7. Sensitive fields MUST be redacted before logs/debug/replay export paths (`F-117`, `F-073`, `NF-011`, `NF-014`).
8. Time helpers MUST preserve monotonic-order checks and explicit skew bounds (`NF-018`, `NF-024`, `NF-025`, `NF-026`).
9. Canonical hashing/sorting helpers MUST be deterministic across platforms and process restarts (`F-150`, `F-152`, `NF-009`).
10. Determinism metadata for accepted turns MUST preserve OR-02 replay-critical evidence linkage (`F-149`, `F-153`, `NF-028`).
11. Recording-fidelity downgrade markers MUST be represented with deterministic transition lineage (`F-176`, `F-153`).
12. Control-path helper functions MUST never introduce hidden blocking or unbounded queues (`F-142`, `F-143`, `NF-004`).
13. Extension registration MUST fail closed on ABI/version incompatibility (`F-124`, `F-162`, `NF-032`).
14. Extension activation/deactivation MUST occur at explicit boundaries only (session/turn boundary) (`F-149`, `F-161`, PRD `4.3`).
15. Shared helpers MUST NOT bypass authority epoch context when producing egress/ingress identity metadata (`F-156`, `NF-027`).
16. Unknown extension capabilities MUST degrade to deterministic defaults, not implicit permissive behavior (`F-048`, `F-137`, `F-161`).

## 6) Major tradeoffs and alternatives

| Tradeoff | Chosen direction | Alternative | Why chosen |
| --- | --- | --- | --- |
| Shared core types vs per-domain duplicate structs | Centralize only low-level invariants in `internal/shared/types` | Keep all types local per domain | Needed for consistent identity/correlation/determinism across runtime, control-plane, replay (`F-013`, `F-149`, `NF-008`) |
| Strict typed config resolver vs ad-hoc map merges | Typed layered resolver with provenance metadata | Loose map/JSON merge everywhere | Required for deterministic defaults and resolved-spec explainability (`F-078`, `F-080`, `F-081`, `NF-015`) |
| Central error taxonomy vs domain-local error enums only | Shared normalized taxonomy + per-domain adapters | Fully isolated error vocabularies | Enables deterministic recovery/reporting and CI/gate comparability (`F-131` to `F-137`, `NF-024` to `NF-029`) |
| Deterministic canonicalization helpers vs direct native map iteration | Shared canonical hash/sort utilities | Native structures with implementation-defined ordering | Needed for replay stability and deterministic evidence (`F-150`, `F-152`, `NF-009`) |
| Shared extension registry contract vs hard-coded extension wiring | ABI/versioned `ExtensionRegistry` | Static compile-time wiring only | Supports custom nodes/runtime adapters with explicit compatibility governance (`F-121` to `F-124`, `F-162`, `F-164`) |
| Redaction as shared primitive vs per-sink redaction logic | Shared `util/security` redaction primitives reused by all sinks | Each logger/replay path rolls its own masking | Reduces leakage risk and policy drift (`F-117`, `F-073`, `NF-011`, `NF-014`) |
| Boundary-locked activation vs live hot mutation mid-turn | Activate/deactivate extensions only at session/turn boundaries | Allow arbitrary in-turn hot swapping | Protects plan-freeze and deterministic-turn semantics (`F-149`, `F-161`, PRD `4.3`) |

## 7) Implementation-ready rollout plan

### 7.1 Delivery phases

Phase 1: Shared contracts bootstrap (no behavior changes)
- Create `internal/shared/types/{identity,eventmeta,determinism}` and `internal/shared/errors/taxonomy` with tests.
- Keep adapters/wrappers in existing runtime/controlplane/provider code paths to preserve behavior.
- Exit criteria: package-level unit tests pass; no domain behavior changes (`F-013`, `F-020`, `F-149`, `F-131`).

Phase 2: Deterministic utility extraction
- Add `internal/shared/util/{canonical,security}` and migrate policy-neutral duplicates (`nonNegative`, hashing helpers, payload masking primitives).
- Keep domain policy tables and decision trees in owners (for example `internal/security/policy` remains owner).
- Exit criteria: no replay/order regressions (`NF-009`) and redaction behavior parity tests (`NF-011`, `NF-014`).

Phase 3: Config and error normalization convergence
- Implement `internal/shared/config/{profile,layering,resolvedview}` and `internal/shared/errors/normalize`.
- Rewire CP normalizer, runtime planresolver, and provider HTTP normalization wrappers.
- Exit criteria: deterministic default provenance preserved (`F-078`, `F-080`, `F-081`) and normalized error action parity (`F-131`, `F-136`, `F-137`).

Phase 4: Timebase + extension registry
- Implement `internal/shared/util/timebase` metric-anchor/time mapping helpers.
- Implement `internal/shared/extensions` descriptor/registry with compatibility checks.
- Exit criteria: quality-gate anchor computations are shared and stable (`NF-024` to `NF-026`); extension ABI checks fail closed (`F-124`, `F-162`, `NF-032`).

### 7.2 Initial file scaffold (recommended)

Create these files first:
- `internal/shared/types/identity/bundle.go`
- `internal/shared/types/identity/bundle_test.go`
- `internal/shared/types/eventmeta/normalize.go`
- `internal/shared/types/eventmeta/normalize_test.go`
- `internal/shared/types/determinism/envelope.go`
- `internal/shared/types/determinism/envelope_test.go`
- `internal/shared/errors/taxonomy/codes.go`
- `internal/shared/errors/taxonomy/codes_test.go`
- `internal/shared/util/canonical/hash.go`
- `internal/shared/util/canonical/hash_test.go`
- `internal/shared/util/security/redact.go`
- `internal/shared/util/security/redact_test.go`
- `internal/shared/config/profile/simple_v1.go`
- `internal/shared/config/profile/simple_v1_test.go`
- `internal/shared/config/layering/resolver.go`
- `internal/shared/config/layering/resolver_test.go`
- `internal/shared/config/resolvedview/render.go`
- `internal/shared/config/resolvedview/render_test.go`
- `internal/shared/errors/normalize/mapper.go`
- `internal/shared/errors/normalize/mapper_test.go`
- `internal/shared/util/timebase/anchors.go`
- `internal/shared/util/timebase/anchors_test.go`
- `internal/shared/extensions/descriptor.go`
- `internal/shared/extensions/registry.go`
- `internal/shared/extensions/registry_test.go`

### 7.3 Integration sequence (low-risk order)

1. Migrate runtime identity + determinism wrappers to shared types (`internal/runtime/identity/context.go`, `internal/runtime/determinism/service.go`, `internal/runtime/planresolver/resolver.go`) (`F-149`, `F-150`, `F-158`).
2. Migrate event metadata normalization wrappers (`internal/runtime/eventabi/gateway.go`, buffering/flowcontrol signal builders) (`F-020`, `F-142`, `F-146`).
3. Migrate provider and CP error normalization wrappers (`providers/common/httpadapter/httpadapter.go`, `internal/controlplane/distribution/file_adapter.go`) (`F-131`, `F-136`).
4. Introduce shared profile/layering/resolved-view and wire CP/runtime entrypoints (`F-078`, `F-080`, `F-081`).
5. Introduce shared timebase anchors and extension registry (`NF-024` to `NF-026`, `F-124`, `F-162`).

### 7.4 Verification gates for each integration PR

- `go test ./...` must pass.
- Replay determinism suites remain stable (`NF-009`, `F-149`, `F-152`).
- OR-02 evidence completeness remains 100% for accepted turns (`NF-028`, `F-153`, `F-176`).
- Cancellation/authority gate invariants remain unchanged (`NF-026`, `NF-027`, `F-156`, `F-175`).
- Redaction-leak checks remain green (`NF-011`, `NF-014`, `F-117`).
