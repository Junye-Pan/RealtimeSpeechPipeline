# Transport Adapter Target Architecture

This guide defines the target architecture for framework-level transport adapters that convert transport-native media and control streams into runtime-safe Event ABI signals.

Evidence baseline used in this design:
- PRD requirements: `Req.4`, `Req.8`, `Req.10`, `Req.11`, `Req.15`, `Req.16`, `Req.22`, `Req.23`, `Req.24`, `Req.25`, `Req.27`, `Req.28`, `Req.29`, `Req.30`, `Req.31`, `Req.32`, `Req.35`, `Req.36`
- Feature framework IDs: `F-083` to `F-091`, `F-011`, `F-055`, `F-062`, `F-065`, `F-067`, `F-168`, `F-169`, `F-170`, `NF-004`, `NF-005`, `NF-006`, `NF-008`, `NF-030`, `NF-031`

## 1) Module list with responsibilities

| Module | Target path | Responsibility | Evidence |
| --- | --- | --- | --- |
| Transport contract | `api/transport` | Shared adapter contract for session bootstrap, ingress/egress envelopes, lifecycle, and capability negotiation across all transports. | `Req.31`, `Req.32`, `F-086` |
| Adapter kernel | `internal/runtime/transport/kernel` | Runtime-facing orchestration: starts adapter session, wires ingress to lanes, applies cancel fencing, and drives deterministic shutdown. | `Req.10`, `Req.31`, `F-083`, `F-084`, `F-085` |
| Session lifecycle FSM | `internal/runtime/transport/sessionfsm` | Normalizes connection state transitions (`connected`, `reconnecting`, `disconnected`, `ended`, `silence`, `stall`) with timeout and cleanup policy. | `Req.32`, `F-091` |
| Ingress normalization | `internal/runtime/transport/ingress` | Converts media/control packets into ABI-compliant DataLane or ControlLane events, preserving sequence/time markers. | `Req.8`, `Req.10`, `F-089`, `F-090` |
| Egress broker | `internal/runtime/transport/egress` | Applies output fencing, delivery acks, pacing, and cancellation semantics before transport playback/send. | `Req.10`, `Req.31`, `F-083`, `F-084`, `F-085` |
| Buffer and loss manager | `internal/runtime/transport/buffering` | Jitter buffers, loss handling, drop policy, and bounded memory behavior. | `Req.15`, `Req.16`, `F-087`, `NF-004` |
| Authority and admission guard | `internal/runtime/transport/authority` | Validates control-plane tokens, lease/epoch, tenant identity, and stale authority rejection. | `Req.27`, `Req.30`, `Req.31`, `Req.36`, `F-088` |
| Routing and migration client | `internal/runtime/transport/routing` | Subscribes to routing view updates and performs fast endpoint switch without split-brain egress. | `Req.11`, `Req.28`, `Req.29`, `NF-031` |
| Transport telemetry emitter | `internal/runtime/transport/telemetry` | Emits structured metrics/logs/spans and replay evidence without blocking control path. | `Req.22`, `F-065`, `F-067`, `NF-008`, `NF-030` |
| Extension registry | `internal/runtime/transport/extensions` | Registers transport plugins (signals, codecs, policies) and optional adapter middleware hooks. | `Req.25`, `F-011`, `F-062` |
| LiveKit driver | `transports/livekit` | LiveKit-specific media/session integration over the shared contract. | `F-083`, `F-086` |
| WebSocket driver | `transports/websocket` | Bi-directional event and audio streaming over WebSocket over the shared contract. | `F-084`, `F-086` |
| Telephony driver | `transports/telephony` | PSTN/telephony media/control bridge over the shared contract. | `F-085`, `F-086` |

Target flow:
1. Driver receives transport-native frame/signal (`F-083`, `F-084`, `F-085`).
2. Ingress module normalizes to ABI envelope (`Req.8`, `Req.10`, `F-089`, `F-090`).
3. Authority guard validates lease/epoch/token (`Req.27`, `Req.31`, `F-088`).
4. Kernel forwards to runtime lanes and turn arbiter (`Req.10`, `Req.31`).
5. Runtime egress returns output chunks and control signals (`Req.10`, `F-083`, `F-084`, `F-085`).
6. Egress broker enforces fences and delivery semantics (`Req.31`, `Req.32`).
7. Driver sends transport-native output (`F-083`, `F-084`, `F-085`).
8. Telemetry module emits non-blocking evidence (`Req.22`, `F-065`, `NF-008`).

## 2) Key interfaces (pseudocode)

```go
// Shared cross-transport contract (F-086, Req.31, Req.32).
type TransportAdapter interface {
    Kind() TransportKind
    Capabilities() AdapterCapabilities
    Open(ctx context.Context, bootstrap SessionBootstrap) (TransportSessionHandle, error)
    HandleIngress(ctx context.Context, frame TransportFrame) ([]RuntimeEnvelope, error)
    HandleEgress(ctx context.Context, envelope RuntimeEnvelope) ([]TransportFrame, error)
    OnRoutingUpdate(ctx context.Context, view RoutingView) error
    Close(ctx context.Context, reason CloseReason) error
}

// Lease/epoch and token guard at transport boundary (Req.27, Req.31, F-088).
type AuthorityGuard interface {
    ValidateBootstrap(ctx context.Context, bootstrap SessionBootstrap) (AuthorityContext, error)
    ValidateIngress(ctx context.Context, env RuntimeEnvelope, authority AuthorityContext) error
    ValidateEgress(ctx context.Context, env RuntimeEnvelope, authority AuthorityContext) error
}

// Deterministic lifecycle normalization (Req.32, F-091).
type LifecycleFSM interface {
    Transition(event ConnectionEvent) (ConnectionState, error)
    IsTerminal() bool
    CleanupDeadline() time.Time
}

// Extension point for transport-specific controls/codecs/policies (Req.25, F-011, F-062).
type TransportPlugin interface {
    Name() string
    Bind(adapter TransportAdapter) error
    IngressHook(ctx context.Context, frame TransportFrame) (TransportFrame, error)
    EgressHook(ctx context.Context, frame TransportFrame) (TransportFrame, error)
}

// Optional custom runtime bridge while preserving ABI and lane semantics.
type RuntimeBridge interface {
    Submit(ctx context.Context, env RuntimeEnvelope) error
    SubscribeEgress(ctx context.Context, sessionID string) (<-chan RuntimeEnvelope, error)
}
```

## 3) Data model: core entities and relations

| Entity | Purpose | Key fields |
| --- | --- | --- |
| `TransportSession` | Adapter-scoped session identity and lifecycle root (`Req.32`, `F-086`). | `session_id`, `tenant_id`, `pipeline_id`, `runtime_endpoint`, `state`, `created_at` |
| `AuthorityContext` | Lease/epoch and auth binding for split-brain safety (`Req.27`, `Req.31`, `F-088`). | `lease_id`, `epoch`, `token_id`, `principal`, `expires_at`, `region` |
| `RoutingView` | Current placement and migration target from control plane (`Req.28`, `Req.29`, `NF-031`). | `view_version`, `active_endpoint`, `candidate_endpoint`, `lease_epoch` |
| `IngressEnvelope` | Normalized transport input to runtime ABI (`Req.8`, `Req.10`, `F-089`, `F-090`). | `session_id`, `turn_id`, `lane`, `sequence`, `media_time`, `wall_time`, `payload`, `authority` |
| `EgressEnvelope` | Runtime output before transport-native conversion (`Req.10`, `Req.31`, `F-083`, `F-084`, `F-085`). | `session_id`, `turn_id`, `lane`, `sequence`, `scope`, `payload`, `cancelled` |
| `ConnectionSignal` | Lifecycle/liveness events (`Req.32`, `F-091`). | `kind`, `at`, `reason`, `metadata` |
| `TransportMetric` | Adapter observability record (`F-065`, `F-067`, `NF-008`, `NF-030`). | `name`, `value`, `labels`, `session_id`, `turn_id`, `correlation_id`, `timestamp` |
| `AuditEvidence` | Replay and tamper-evident event trail entry (`Req.22`, `F-170`). | `event_id`, `prev_digest`, `digest`, `correlation_id`, `event_type`, `timestamp` |

Relations:
1. `TransportSession` has one active `AuthorityContext` per lease epoch (`Req.27`, `Req.31`).
2. `TransportSession` consumes many `IngressEnvelope` records and produces many `EgressEnvelope` records (`Req.8`, `Req.10`).
3. Every ingress and egress envelope references the active `AuthorityContext` (`Req.31`, `F-088`).
4. `RoutingView` updates may rotate `AuthorityContext` and runtime endpoint for a session (`Req.28`, `Req.29`, `NF-031`).
5. `ConnectionSignal`, `TransportMetric`, and `AuditEvidence` are all correlated by `session_id` and `turn_id` (`F-067`, `NF-008`).
6. `AuditEvidence` forms a digest chain for tamper evidence (`Req.22`, `F-170`).

## 4) Extension points: plugins, custom nodes, custom runtimes

Plugins:
1. Signal mapper plugins map transport-specific controls (for example DTMF) into normalized ControlLane events (`F-089`, `Req.8`, `Req.10`).
2. Codec plugins handle adapter-local transcode paths before ABI emission (`F-090`, `Req.8`).
3. Admission middleware plugins can enforce tenant/compliance policy before ingress acceptance (`Req.35`, `Req.36`, `F-062`).
4. Telemetry sink plugins export additional metrics/log formats while keeping baseline schema (`F-065`, `NF-008`, `NF-030`).

Custom nodes:
1. Adapter ingress/egress hooks can target custom Event ABI nodes registered through the stable Node API (`F-011`, `Req.8`).
2. Policy-injected nodes can be inserted at turn resolution time (for redaction/safety/compliance) without adapter rewrites (`F-062`, `Req.4`).
3. Node insertion must not bypass authority checks, sequencing markers, or cancel fencing (`Req.10`, `Req.31`).

Custom runtimes:
1. A runtime bridge adapter can target alternative runtime executors if they accept the same Event ABI/lane semantics (`Req.8`, `Req.10`).
2. Custom runtime implementations must preserve turn lifecycle contracts (`turn_open`, cancel-first handling, deterministic close) (`Req.4`, `Req.31`).
3. Transport adapters remain runtime-agnostic above the `RuntimeBridge` interface (`Req.25`, `F-055`).

## 5) Design invariants

1. No ingress event is accepted without valid lease/epoch authority and token binding (`Req.27`, `Req.31`, `F-088`).
2. Pre-turn stale-epoch rejects are terminal for that ingress and do not emit synthetic turn-close events (`Req.31`, `Req.32`).
3. ControlLane signals preempt DataLane and TelemetryLane work for the same session scope (`Req.10`).
4. Cancel acceptance fences egress for that scope before any further playback acknowledgment (`Req.10`, `Req.31`).
5. Ingress and egress sequencing is monotonic per session and lane (`Req.8`, `Req.10`).
6. Timestamp mapping preserves runtime monotonic time plus media-time correlation markers (`Req.8`, `Req.16`).
7. Adapter buffers are bounded and enforce configured drop/backpressure policy (`Req.15`, `Req.16`, `F-087`, `NF-004`).
8. Lifecycle state transitions are schema-valid and deterministic across retries (`Req.32`, `F-091`).
9. Disconnect handling performs deterministic cleanup by configured timeout (`Req.32`, `F-091`).
10. Routing/placement changes must prevent duplicate egress across concurrent epochs (`Req.28`, `Req.29`, `Req.31`).
11. Transport-specific controls are normalized into explicit typed ABI events (`F-089`, `Req.8`).
12. Replay-critical markers (authority, sequencing, cancellation, lifecycle) are always emitted for accepted turns (`Req.22`, `F-170`).
13. Telemetry emission is non-blocking for control and data paths (`Req.22`, `NF-005`, `NF-006`).
14. All logs/metrics/spans include correlation identifiers (`session`, `turn`, `node` where applicable) (`F-067`, `NF-008`).
15. Key rotation and data-residency policy changes are handled without violating active-session safety rules (`Req.36`, `F-168`, `F-169`).

## 6) Tradeoffs and alternatives

| Tradeoff | Chosen direction | Alternative | Why chosen |
| --- | --- | --- | --- |
| Buffer ownership | Adapter-local jitter/loss buffers | Centralized runtime-only buffering | Transport jitter is source-specific; local buffering keeps timing corrections close to source while still honoring runtime limits. (`Req.15`, `Req.16`, `F-087`) |
| Authority validation points | Validate at bootstrap plus ingress and egress | Validate only at session bootstrap | Re-checking at boundaries prevents stale-authority split-brain outputs during migration/rotation. (`Req.27`, `Req.31`, `F-088`) |
| Contract shape | One shared adapter contract with driver specializations | Fully bespoke contracts per transport | Shared contract reduces divergence and makes new adapters conformance-testable. (`Req.31`, `Req.32`, `F-086`) |
| Telemetry reliability mode | Best-effort TelemetryLane with guaranteed replay-critical control evidence | Fully guaranteed telemetry delivery | Guaranteed telemetry can block control path and break latency goals; best-effort keeps system responsive while preserving mandatory evidence. (`Req.22`, `NF-005`, `NF-006`, `NF-030`) |
| Plugin loading model | Registry-based, compile-time safe plugin binding | Dynamic runtime module loading | Compile-time registration keeps deterministic behavior and auditability under strict security constraints. (`Req.25`, `Req.36`, `F-170`) |
| Migration strategy | Fast switch with egress fencing on old epoch | Active-active dual egress during handoff | Dual egress increases duplicate output risk; fencing preserves single-writer semantics for authority. (`Req.28`, `Req.29`, `Req.31`) |

## Current implementation inventory

- `transports/livekit` (implemented baseline)
- `transports/websocket` (scaffold)
- `transports/telephony` (scaffold)

## Current progress and divergence snapshot (2026-02-16)

This section reflects the current codebase state under `transports/` and related command/integration tests.

### Module status vs target architecture

| Target module | Current status | Current code evidence | Divergence from target |
| --- | --- | --- | --- |
| `api/transport` shared contract (`F-086`) | Not started | No shared transport contract package is present. | No cross-adapter `TransportAdapter` contract is implemented yet. |
| `internal/runtime/transport/kernel` | Partial | Turn-oriented orchestration is embedded inside `transports/livekit/adapter.go`. | No standalone shared kernel module; behavior is LiveKit-specific. |
| `internal/runtime/transport/sessionfsm` (`F-091`) | Partial | Lifecycle/control signals are produced in `transports/livekit/adapter.go` via runtime transport helpers. | No dedicated reusable session FSM module. |
| `internal/runtime/transport/ingress` (`F-089`, `F-090`) | Partial | Ingress normalization is implemented in `transports/livekit/adapter.go` (`mapIngressEvent`). | No transport-agnostic ingress module and no codec plugin abstraction yet. |
| `internal/runtime/transport/egress` | Partial | Output fencing behavior exists in `transports/livekit/adapter.go` via runtime output fence logic. | No standalone egress broker with shared pacing/delivery contracts. |
| `internal/runtime/transport/buffering` (`F-087`, `NF-004`) | Not started | No transport-local jitter/loss manager package exists. | Jitter/loss policy module from target architecture is not implemented. |
| `internal/runtime/transport/authority` (`F-088`) | Partial | Epoch/authority fields are propagated through LiveKit events in `transports/livekit/adapter.go`. | No explicit authority guard interface with dedicated ingress/egress validation layer. |
| `internal/runtime/transport/routing` (`NF-031`) | Not started | No routing view update handling API is exposed by transport adapters. | Target routing/migration client is not implemented. |
| `internal/runtime/transport/telemetry` (`F-065`, `NF-008`) | Partial | Deterministic report generation exists in `transports/livekit/adapter.go` and CLI reporting in `transports/livekit/command.go`. | No shared non-blocking telemetry emitter module aligned to target contract. |
| `internal/runtime/transport/extensions` (`F-011`, `F-062`) | Not started | No plugin registry package is present under transports. | Plugin/custom-node extension registry is still pending. |
| `transports/livekit` (`F-083`) | Implemented baseline | `transports/livekit/config.go`, `transports/livekit/adapter.go`, `transports/livekit/command.go`, `transports/livekit/probe.go`. | Baseline exists, but it is currently a deterministic adapter/report pipeline rather than full target modular split. |
| `transports/websocket` (`F-084`) | Scaffold only | `transports/websocket/websocket_transport_guide.md` only. | No runtime adapter implementation yet. |
| `transports/telephony` (`F-085`) | Scaffold only | `transports/telephony/telephony_transport_guide.md` only. | No runtime adapter implementation yet. |

### Test and evidence progress vs guide expectations

Implemented deterministic coverage:
- `transports/livekit/adapter_test.go` covers default, authority-revoked, and transport-disconnect paths.
- `transports/livekit/command_test.go` covers CLI report generation and event fixture loading.
- `transports/livekit/config_test.go` covers configuration validation.
- `transports/livekit/probe_test.go` covers RoomService probe success/failure.
- `test/integration/livekit_transport_integration_test.go` covers baseline integration paths.
- `cmd/rspp-runtime/main_test.go` and `cmd/rspp-local-runner/main_test.go` cover command paths and report generation behavior.

Known divergences:
1. Tests primarily validate LiveKit baseline behavior and do not yet cover target-module concerns such as routing updates (`Req.28`), explicit authority guard boundaries (`Req.27`, `Req.31`), or shared buffer/loss modules (`Req.15`, `Req.16`, `F-087`).
2. Operational appendix references `.codex/transports/livekit-report.json` and `.codex/transports/livekit-local-runner-report.json`; command tests currently validate report generation using test-local temp paths rather than asserting those fixed artifact paths.
3. The probe-enabled runtime CLI path is documented, and low-level probe logic is unit tested, but there is no end-to-end command test that exercises the probe-enabled CLI path directly.

## Operational appendix (current baseline)

Runtime command path:
- `go run ./cmd/rspp-runtime livekit -dry-run=true -probe=false`

Probe-enabled path:
- `go run ./cmd/rspp-runtime livekit -dry-run=false -probe=true -room <room>`

Local runner path:
- `go run ./cmd/rspp-local-runner`

Common env inputs:
- `RSPP_LIVEKIT_URL`
- `RSPP_LIVEKIT_API_KEY`
- `RSPP_LIVEKIT_API_SECRET`
- optional room/session/turn/pipeline/epoch fields

Deterministic artifacts:
- `.codex/transports/livekit-report.json`
- `.codex/transports/livekit-local-runner-report.json`

## Gate evidence map

Blocking deterministic evidence:
- `transports/livekit/*_test.go`
- `test/integration/livekit_transport_integration_test.go`
- `cmd/rspp-runtime/main_test.go`
- `cmd/rspp-local-runner/main_test.go`

Non-blocking live evidence:
- `make livekit-smoke`
- CI job `livekit-smoke`
- artifacts under `.codex/providers/`

PRD MVP standards tie-in:
1. Turn-open decision latency target: p95 <= 120 ms from `turn_open_proposed` acceptance to `turn_open`.
2. First assistant output latency target: p95 <= 1500 ms from `turn_open` to first DataLane output chunk.
3. Cancellation fence latency target: p95 <= 150 ms from cancel acceptance to egress output fencing.
4. Authority safety target: 0 accepted stale-epoch outputs in failover/lease-rotation coverage.
5. MVP mode is `Simple` only: transport integration behavior must remain compatible with Simple-profile runtime defaults.
6. OR-02 baseline replay evidence is mandatory for accepted turns: transport artifacts/signals must preserve replay-critical authority, lifecycle, and delivery markers required by the baseline contract.

## Conformance touchpoints

- `LK-001`: adapter mapping + integration assertions
- `LK-002`: runtime/local-runner command behavior and deterministic artifacts
- `LK-003`: optional live smoke path

## Change checklist

1. Keep signal names/emitter ownership schema-valid.
2. Preserve deterministic report structure and error behavior.
3. Update runtime/local-runner command tests for CLI changes.
4. Re-run LiveKit package and integration tests.

## Related docs

- `docs/repository_docs_index.md`
- `docs/PRD.md`
- `docs/RSPP_features_framework.json`
- `docs/rspp_SystemDesign.md`
- `docs/CIValidationGates.md`
- `docs/SecurityDataHandlingBaseline.md`
