# Provider Adapter Guide (Target Architecture)

This guide defines the target architecture and module breakdown for provider adapters in RSPP, aligned to:
- `docs/PRD.md` (especially sections 4.1.6, 4.1.7, 4.3, 5.1, 5.2)
- `docs/RSPP_features_framework.json` (feature IDs cited inline)
- `docs/rspp_SystemDesign.md` (provider subsystem boundary and runtime integration)

Scope boundary:
- In scope: provider contract, invocation lifecycle, retry/switch policy execution, streaming semantics, replay/observability evidence, extension hooks.
- Out of scope: RTC transport-stack internals and model-provider ownership boundaries (`NF-019`, `NF-023`).

## Evidence Baseline

Primary feature anchors for this guide:
- Provider contract and policy control: `F-055` to `F-066`, `F-102`, `F-160`
- Cancellation, budget, degrade behavior: `F-033` to `F-041`, `F-046`, `F-047`
- Error normalization and fallback behavior: `F-131`, `F-133`, `F-136`
- Replay and determinism evidence: `F-149`, `F-150`, `F-151`, `F-153`, `F-154`, `F-176`
- State/authority/idempotency: `F-155`, `F-156`, `F-158`
- Security and tenancy: `F-061`, `F-114`, `F-117`, `NF-013`
- Extensibility and conformance: `F-011`, `F-121`, `F-122`, `F-123`, `F-124`, `F-164`
- Performance targets: `NF-001`, `NF-002`, `NF-003`, `NF-006`

## Target Architecture (Provider Subsystem)

Layering model:

```text
Policy/Plan Layer
  -> provider policy snapshot resolver
  -> capability snapshot freeze (turn-start)

Invocation Layer
  -> deterministic candidate selection
  -> attempt loop (retry/switch/fallback) + cancel/budget enforcement
  -> stream observer and lifecycle guards

Adapter Layer
  -> modality adapters (stt/llm/tts)
  -> shared protocol helpers (http/sse/stream codecs)

Evidence Layer
  -> normalized metrics/logs/traces
  -> OR-02 replay-critical provider invocation evidence

Extension Layer
  -> plugin registration
  -> custom nodes/external nodes boundary
  -> custom runtime host adapter
```

Layer-to-feature trace:
- Policy/Plan Layer: `F-056`, `F-057`, `F-058`, `F-060`, `F-066`, `F-149`, `F-161`
- Invocation Layer: `F-033`, `F-046`, `F-047`, `F-059`, `F-160`
- Adapter Layer: `F-055`, `F-063`, `F-064`, `F-065`
- Evidence Layer: `F-149`, `F-150`, `F-153`, `F-154`, `F-176`
- Extension Layer: `F-011`, `F-121`, `F-122`, `F-123`, `F-124`, `F-164`

## 1) Module List With Responsibilities

| Module | Target path(s) | Responsibility | Evidence |
| --- | --- | --- | --- |
| Provider contracts | `internal/runtime/provider/contracts` | Stable adapter ABI, request/outcome schema, stream chunk validation, normalized outcome taxonomy. | `F-055`, `F-131`, `F-160`, PRD §4.1.7 |
| Provider bootstrap | `internal/runtime/provider/bootstrap` | Canonical provider catalog assembly and minimum modality coverage checks for MVP provider matrix. | PRD §5.2, `F-055` |
| Provider registry | `internal/runtime/provider/registry` | Deterministic provider catalog, preferred-provider ordering, modality-scoped candidate list. | `F-056`, `F-066`, `F-160`, PRD §4.3 |
| Invocation controller | `internal/runtime/provider/invocation` | Attempt loop, retry/backoff/provider-switch/fallback decisions, cancel short-circuit, per-attempt telemetry. | `F-059`, `F-046`, `F-033`, `F-047`, `F-065`, `NF-003` |
| Policy snapshot resolver (target addition) | `internal/runtime/provider/policy` | Convert control-plane policy + tenant/language/region/cost context into deterministic candidate strategy and limits. | `F-056`, `F-057`, `F-058`, `F-060`, `F-066`, `F-102`, PRD §4.1.9 |
| Capability snapshot freeze (target addition) | `internal/runtime/provider/capability` | Freeze provider capability/health snapshot into turn-scoped plan; prevent mid-turn drift. | `F-149`, `F-161`, PRD §4.1.1 and §4.3 |
| Resilience strategy (target addition) | `internal/runtime/provider/strategy` | Shared decision helpers for retry/switch/fallback, budget checks, deterministic backoff shaping. | `F-059`, `F-047`, `F-133`, `NF-006` |
| Evidence writer (target addition) | `internal/runtime/provider/evidence` | Emit OR-02-compliant provider invocation evidence (attempt lineage, outcomes, cancel markers) into non-blocking timeline path. | `F-149`, `F-150`, `F-153`, `F-154`, `F-160`, `F-176`, PRD §4.1.6 |
| Shared protocol adapter helpers | `providers/common/httpadapter`, `providers/common/streamsse` | HTTP/SSE transport adaptation, payload capture controls, status normalization, compatibility stream wrapper. | `F-055`, `F-063`, `F-064`, `F-117`, `NF-013` |
| Modality adapters | `providers/stt/*`, `providers/llm/*`, `providers/tts/*` | Provider-specific request/response mapping, native streaming chunk emission, provider-specific error mapping. | `F-055`, `F-063`, `F-064`, `F-065`, PRD §4.1.7 |
| Plugin host (target addition) | `internal/runtime/provider/plugin` | Discover/register signed adapter plugins and enforce adapter conformance contracts at load time. | `F-055`, `F-164`, `F-121`, `F-124` |
| Contract and conformance suites (target addition) | `test/contract/provider`, `test/integration/provider` | Mandatory provider adapter contract tests, deterministic replay tests, failure-path suites, and compatibility gates. | `F-164`, `F-131`, `F-149`, `F-151`, PRD §4.4 |

## 2) Key Interfaces (Pseudocode)

```go
// Stable runtime-facing provider contract.
type Adapter interface {
    ProviderID() string
    Modality() Modality
    Invoke(req InvocationRequest) (Outcome, error)
}

// Native streaming contract.
type StreamingAdapter interface {
    Adapter
    InvokeStream(req InvocationRequest, observer StreamObserver) (Outcome, error)
}

// Turns control-plane policy snapshot into deterministic invocation plan.
type PolicyResolver interface {
    Resolve(input PolicyInput) (ResolvedProviderPlan, error)
}

type ResolvedProviderPlan struct {
    ProviderInvocationID string
    Modality Modality
    OrderedCandidates []ProviderCandidate // deterministic order
    MaxAttemptsPerProvider int
    AllowedAdaptiveActions []AdaptiveAction // retry, provider_switch, fallback
    Budget BudgetSpec
    CapabilitySnapshotRef string
    PolicySnapshotRef string
}

// Executes attempt loop with retry/switch/fallback and stream hooks.
type InvocationController interface {
    Invoke(plan ResolvedProviderPlan, req InvocationContext) (InvocationResult, error)
}

// Optional adapter plugin boundary.
type AdapterPlugin interface {
    Descriptor() ProviderDescriptor
    NewAdapter(cfg SecretRefConfig) (Adapter, error)
    ContractVersion() string
}

// Extension boundary for custom runtime hosts (in-process, sidecar, remote).
type ProviderRuntimeHost interface {
    Execute(plan ResolvedProviderPlan, req InvocationContext) (InvocationResult, error)
}
```

Interface notes:
- `InvocationRequest` and `StreamChunk` remain strict-validated for deterministic fields (`session_id`, `turn_id`, `provider_invocation_id`, `attempt`, sequence/timestamps), matching current `contracts` behavior and PRD replay requirements (`F-150`, `F-158`, `NF-013`).
- `PolicyResolver` is the main missing seam for separating policy derivation from invocation mechanics while preserving deterministic ordering (`F-056`, `F-066`).
- `ProviderRuntimeHost` is the extension seam for custom runtimes without changing adapter ABI (`F-122`, `F-123`, `F-124`, `F-103` to `F-109`).

## 3) Data Model: Core Entities and Relations

| Entity | Core fields | Relations |
| --- | --- | --- |
| `ProviderDescriptor` | `provider_id`, `modality`, `capabilities`, `contract_version` | Registered in catalog/plugin host; referenced by candidates and attempts (`F-055`, `F-164`). |
| `ResolvedProviderPlan` | `provider_invocation_id`, candidate order, action set, budgets, snapshot refs | Produced once per turn; consumed by invocation controller (`F-149`, `F-161`, `F-056`, `F-066`). |
| `ProviderInvocation` | `provider_invocation_id`, `session_id`, `turn_id`, `event_id`, `authority_epoch`, `status` | Parent of one or more attempts; unit of observability/replay (`F-160`). |
| `ProviderAttempt` | `attempt_no`, `provider_id`, `streaming_used`, `latency_ms`, `first_chunk_latency_ms` | Child of `ProviderInvocation`; yields one normalized outcome (`F-059`, `F-065`). |
| `Outcome` | `class`, `reason`, `retryable`, `backoff_ms`, `circuit_open`, `status_code` | Attached to each attempt and invocation terminal state (`F-131`). |
| `StreamChunk` | `kind`, `sequence`, `text_delta|text_final|audio_bytes`, `error_reason` | Ordered child events under one attempt (`F-063`, `F-064`). |
| `HealthSnapshot` | provider health score, circuit state, freshness timestamp | Read by policy resolver at turn start (`F-057`, `F-060`). |
| `ReplayEvidenceRecord` | plan hash ref, determinism markers, attempt lineage, cancel markers | Written asynchronously for OR-02 completeness (`F-149`, `F-153`, `F-154`). |

Cardinality rules:
- `Turn` (1) -> (`ProviderInvocation`) (N) (`F-160`)
- `ProviderInvocation` (1) -> (`ProviderAttempt`) (1..N) (`F-059`, `F-160`)
- `ProviderAttempt` (1) -> (`Outcome`) (1) (`F-131`)
- `ProviderAttempt` (1) -> (`StreamChunk`) (0..N) (`F-063`, `F-064`)
- `ResolvedTurnPlan` (1) -> (`ResolvedProviderPlan`) (N, one per provider-bearing path) (`F-149`, `F-161`)

## 4) Extension Points: Plugins, Custom Nodes, Custom Runtimes

### A) Adapter plugins

Contract:
- Plugin must implement `Adapter` or `StreamingAdapter` and declare contract version compatibility (`F-055`, `F-164`).
- Plugin load path must enforce conformance tests before admission into runtime catalog (`F-164`).

Design intent:
- Keep provider expansion independent from core runtime release cadence (`F-121`).
- Require deterministic provider ID/modality declaration and standardized outcomes (`F-055`, `F-131`).

Evidence:
- `F-055`, `F-164`, `F-121`, `F-124`, PRD §4.1.7 and §4.4.5.

### B) Custom nodes that invoke providers

Contract:
- Custom nodes use Node API and must emit/consume Event ABI while routing provider work through `ProviderInvocation` (`F-011`, `F-160`).
- External custom nodes remain behind explicit isolation boundary with timeout/resource policies (`F-122`, `F-123`, `F-124`).

Design intent:
- Preserve uniform observability and replay correlation even for custom logic paths (`F-149`, `F-151`, `F-160`).
- Avoid bypasses that would skip provider normalization and control-lane signaling (`F-055`, `F-131`, `F-136`).

Evidence:
- `F-011`, `F-121`, `F-122`, `F-123`, `F-124`, `F-160`, PRD §4.1.7 and §4.1.9.

### C) Custom runtimes (host implementations)

Contract:
- Custom runtime host must preserve invocation semantics: deterministic attempt ordering, cancel fencing, and evidence emission (`F-033`, `F-059`, `F-160`, `F-175`).
- Runtime host can vary execution topology (in-process, sidecar, remote) but not behavior contract (`F-103`, `F-105`, `F-109`).

Design intent:
- Enable deployment portability and specialized execution strategies without forked semantics (`F-122`, `F-123`, `F-124`, `NF-006`).

Evidence:
- `F-103`, `F-105`, `F-109`, `F-122`, `F-123`, PRD §4.3 and §4.4.5.

## 5) Design Invariants (Must Preserve)

1. Provider behavior for a turn is frozen from turn-start snapshots and must not drift mid-turn except through pre-authorized adaptive actions. (`F-149`, `F-161`, PRD §4.1.1, §4.3)
2. Every provider call is represented as exactly one `ProviderInvocation` with stable `provider_invocation_id`. (`F-160`, PRD §4.1.7)
3. Attempt indices are monotonic per invocation and start at `1`; candidate order is deterministic for identical inputs. (`F-059`, `F-066`)
4. Retry/provider-switch/fallback may occur only when allowed by resolved adaptive-action set and remaining budgets. (`F-046`, `F-047`, `F-059`)
5. Outcome classes are constrained to normalized taxonomy; adapter-specific raw errors cannot leak as terminal taxonomy values. (`F-131`, PRD §4.1.7)
6. Streaming lifecycle per attempt is one `start`, ordered chunks, and one terminal (`final` or `error`) with no post-terminal chunks. (`F-063`, `F-064`)
7. After cancel acceptance, no new output acceptance/playback start is valid for the canceled scope. (`F-033`, `F-038`, `F-175`, PRD §4.2.2)
8. Provider invocation latency and cancel latency must be observable with standardized dimensions. (`F-065`, `F-040`, `NF-001`, `NF-003`)
9. Telemetry and timeline recording must be non-blocking relative to control-lane progression and cancellation. (`F-154`, `F-176`, PRD §4.1.6)
10. OR-02 baseline replay evidence for provider outcomes is mandatory on accepted turns. (`F-149`, `F-153`, `F-160`, PRD §4.1.6)
11. Secrets are referenced through managed config and never emitted in logs/debug payloads. (`F-061`, `F-117`)
12. Tenant-scoped policy/provider config isolation is enforced for selection, quotas, and metrics attribution. (`F-114`, `F-056`, `F-058`)
13. Malformed adapter inputs and stream chunks must fail validation safely without crashing runtime. (`NF-013`)
14. Adapter subsystem must remain transport-agnostic and must not absorb transport-stack concerns. (`NF-019`, PRD §2.2)
15. Adapter subsystem integrates with provider APIs; it must not become a model API abstraction that hides determinism and policy controls. (`NF-023`, PRD §2.2)

## 6) Tradeoffs (Major Decisions and Alternatives)

| Decision | Alternative | Why this target architecture chooses it | Cost / risk |
| --- | --- | --- | --- |
| Runtime-owned retry/switch loop in invocation controller | Adapter-owned retry logic | Keeps retry/switch/fallback deterministic, policy-visible, and replayable across providers. | Adapter implementations are thinner but runtime logic is more complex. (`F-059`, `F-133`, `F-149`) |
| Deterministic candidate order frozen per turn | Per-attempt real-time re-ranking | Preserves replay and decision stability; avoids mid-turn nondeterminism from volatile signals. | May miss some short-term optimization opportunities. (`F-066`, `F-149`, `F-150`, `F-161`) |
| Normalized outcome taxonomy | Provider-specific error passthrough | Enables uniform fallback and control signaling across modalities/providers. | Some provider-specific nuance is compressed; requires metadata channel for detail. (`F-131`, `F-136`, PRD §4.1.7) |
| Native streaming path preferred when available | Unary-first with synthetic stream wrappers | Needed for overlap latency and early output guarantees for LLM/TTS. | Streaming adapters are harder to implement and test correctly. (`F-063`, `F-064`, `NF-001`) |
| Bounded/redacted payload capture by default | Full payload capture for every attempt | Protects sensitive data and keeps telemetry overhead bounded. | Debugging deep payload issues may require controlled opt-in capture. (`F-117`, `F-153`, `NF-002`) |
| In-process adapters as default execution path | Out-of-process adapters only | Lower overhead and simpler MVP operations for latency-sensitive path. | Fault isolation is weaker; external-node/plugin host still needed for untrusted code. (`NF-002`, `F-122`, `F-123`, `F-124`) |
| Separate policy resolver module | Embed policy logic directly in invocation controller | Improves testability and keeps controller deterministic/mechanical; isolates policy churn. | Adds one extra module boundary and data model translation step. (`F-056`, `F-057`, `F-058`, `F-102`) |

## 7) Code-Verified Progress and Divergence (2026-02-16)

### Current progress (implemented)

| Capability | Current implementation | Evidence path(s) | Feature alignment |
| --- | --- | --- | --- |
| Stable provider contract + normalized outcomes | `contracts.Adapter`, `contracts.StreamingAdapter`, strict `InvocationRequest`/`StreamChunk` validation, normalized `OutcomeClass` taxonomy are implemented and used. | `internal/runtime/provider/contracts/contracts.go` | `F-055`, `F-131`, `F-160`, `NF-013` |
| Deterministic provider catalog and MVP coverage bounds | Catalog ordering is deterministic; preferred provider is first; modality coverage validation enforces 3-5 providers per modality. | `internal/runtime/provider/registry/registry.go`, `internal/runtime/provider/bootstrap/bootstrap.go` | `F-055`, `F-056`, `F-066` |
| Deterministic retry/switch execution | Invocation controller implements per-provider attempts, retry budget handling, provider switch, fallback decision, and control-signal emission (`provider_error`, `circuit_event`, `provider_switch`). | `internal/runtime/provider/invocation/controller.go` | `F-046`, `F-047`, `F-059`, `F-133`, `F-136` |
| Streaming control and attempt evidence | Streaming is default-on when adapter supports `InvokeStream`; disable flags and modality-specific flags are implemented; per-attempt `streaming_used`, chunk count, bytes, and latency are recorded. | `internal/runtime/provider/invocation/controller.go` | `F-063`, `F-064`, `F-065`, `NF-001`, `NF-003` |
| Shared HTTP normalization + payload safety controls | `httpadapter` performs unary request normalization, status mapping, capture-mode control (`redacted/full/hash`), and compatibility stream wrapper. | `providers/common/httpadapter/httpadapter.go` | `F-055`, `F-117`, `F-131`, `NF-002` |
| MVP provider matrix present in code | 9 adapters are wired in bootstrap (STT: `deepgram`, `google`, `assemblyai`; LLM: `anthropic`, `gemini`, `cohere`; TTS: `elevenlabs`, `google`, `amazon-polly`). | `internal/runtime/provider/bootstrap/bootstrap.go` | `F-055`, PRD §5.2 intent (multi-provider per modality) |

### Streaming maturity by adapter (implemented behavior)

| Provider | Current stream path | Maturity classification | Evidence path |
| --- | --- | --- | --- |
| `llm-anthropic` | SSE parsing from upstream stream | Native upstream token streaming | `providers/llm/anthropic/adapter.go` |
| `llm-cohere` | SSE parsing from upstream stream | Native upstream token streaming | `providers/llm/cohere/adapter.go` |
| `llm-gemini` | SSE parsing from upstream stream endpoint | Native upstream token streaming | `providers/llm/gemini/adapter.go` |
| `tts-elevenlabs` | `/stream` endpoint audio chunk relay | Native upstream audio streaming | `providers/tts/elevenlabs/adapter.go` |
| `tts-amazon-polly` | AWS SDK audio stream relay | Native upstream audio streaming | `providers/tts/polly/adapter.go` |
| `stt-assemblyai` | Async create + poll lifecycle, metadata + final | Poll-based pseudo-stream | `providers/stt/assemblyai/adapter.go` |
| `stt-deepgram` | Unary response parsed then emitted as deltas/final | Compatibility/pseudo-stream | `providers/stt/deepgram/adapter.go` |
| `stt-google` | Unary response parsed then emitted as deltas/final | Compatibility/pseudo-stream | `providers/stt/google/adapter.go` |
| `tts-google` | Unary base64 audio decoded then chunk-emitted | Compatibility/pseudo-stream | `providers/tts/google/adapter.go` |

### Divergence from target architecture

1. `internal/runtime/provider/policy` and `internal/runtime/provider/capability` seams are not implemented yet; policy/candidate logic is still composed in invocation input + registry ordering. (`F-056`, `F-057`, `F-060`, `F-066`, `F-102`, `F-161`)
2. OR-02 provider evidence writer module is not yet explicit; current path emits telemetry and in-memory invocation result but no dedicated provider evidence component in provider subsystem. (`F-149`, `F-153`, `F-154`, `F-160`, `F-176`)
3. Stream lifecycle monotonicity (`start -> chunk* -> final/error` with strict sequence progression) is not centrally state-machine enforced in invocation controller; current runtime observer validates chunk structure per event. (`F-063`, `F-064`, `F-150`)
4. Fallback and provider-switch share the same emitted control signal name (`provider_switch`); `RetryDecision` differentiates `"provider_switch"` vs `"fallback"`. (`F-047`, `F-133`, `F-136`)
5. Provider config currently uses direct environment keys in adapters; secret-reference resolution is not yet represented as a first-class provider config object in this subsystem. (`F-061`, `F-117`)
6. Adapter test coverage is partial: tests exist for `anthropic`, `cohere`, `assemblyai`, `polly`, and `httpadapter`, while `stt-deepgram`, `stt-google`, `llm-gemini`, `tts-elevenlabs`, and `tts-google` currently lack adapter-specific unit suites. (`F-164`, `NF-013`)

## 8) Implementation-Ready Work Packages

Execution order is designed to reduce risk and preserve compatibility.

| Work package | Objective | File scope (expected) | Done criteria |
| --- | --- | --- | --- |
| `WP-1 Policy/Capability Plan Freeze` | Separate policy resolution and capability snapshot freeze from invocation mechanics. | Add `internal/runtime/provider/policy/*`, `internal/runtime/provider/capability/*`; update `internal/runtime/provider/invocation/controller.go`. | Deterministic `ResolvedProviderPlan` produced at turn start, with snapshot refs persisted in invocation result (`F-056`, `F-060`, `F-066`, `F-149`, `F-161`). |
| `WP-2 Provider Evidence Writer` | Add explicit provider evidence emission for OR-02 baseline fields. | Add `internal/runtime/provider/evidence/*`; integrate from invocation controller. | Provider invocation evidence includes invocation ID, attempts, normalized outcomes, cancel markers, and plan refs without blocking control path (`F-149`, `F-153`, `F-154`, `F-160`, `F-176`). |
| `WP-3 Stream Lifecycle Guard` | Enforce monotonic sequence and legal chunk transition ordering centrally in invocation observer layer. | Update `internal/runtime/provider/invocation/controller.go` (observer state machine) + tests. | Invalid sequence/lifecycle causes deterministic failure classification and tests cover violation paths (`F-063`, `F-064`, `F-150`, `NF-013`). |
| `WP-4 Config SecretRef Layer` | Introduce provider config model that accepts secret references, keeping env fallback for local runner. | Add `internal/runtime/provider/config/*`; update adapter constructors/bootstrap wiring. | Runtime accepts secret refs for provider keys/endpoints and preserves redaction guarantees (`F-061`, `F-117`, `F-114`). |
| `WP-5 Adapter Conformance Coverage` | Close missing adapter test gaps and add provider contract suite path. | Add tests under `providers/stt/deepgram`, `providers/stt/google`, `providers/llm/gemini`, `providers/tts/elevenlabs`, `providers/tts/google`; add `test/contract/provider/*`. | Each adapter has success/error/cancel/stream-order tests; contract suite validates common adapter invariants (`F-164`, `F-131`, `F-160`, `NF-013`). |

Validation commands for work packages:
- `go test ./providers/...`
- `go test ./internal/runtime/provider/...`
- `make verify-quick`
