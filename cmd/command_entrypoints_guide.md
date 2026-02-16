# Command Entrypoints Target Architecture Guide

This guide defines the target architecture and module breakdown for framework command entrypoints, grounded in:
- `docs/PRD.md` (notably sections `3.2`, `4.1.6`, `5.1`, `5.2`, `5.3`)
- `docs/RSPP_features_framework.json` feature IDs
- `docs/rspp_SystemDesign.md` (repository-level module boundaries)

Feature ID citations in this document use the format `[F-###]` and `[NF-###]`.

## Entrypoint scope

| Entrypoint | Primary scope | Contract focus | Feature evidence |
| --- | --- | --- | --- |
| `rspp-runtime` | Runtime operator actions | provider bootstrap, transport command path, retention and runtime evidence | `[F-055]`, `[F-083]`, `[F-086]`, `[F-116]` |
| `rspp-control-plane` | Control-plane operator/client bootstrap actions | route + authority + token issuance | `[F-092]`, `[F-095]`, `[F-099]`, `[F-119]`, `[F-101]` |
| `rspp-cli` | CI/release and developer tooling | validation, replay, gate reports, release promotion | `[F-126]`, `[F-127]`, `[F-072]`, `[F-077]`, `[NF-024]`, `[NF-028]`, `[NF-038]` |
| `rspp-local-runner` | Local single-node developer flow | minimal control-plane stub + transport simulation | `[F-125]`, `[F-130]`, `[F-083]` |

## Target architecture

```text
Entrypoints (cmd/*)
  -> Shared Command Kernel (parse, config, policy, errors, output)
    -> Domain Facades (runtime ops, control-plane ops, tooling/gates, local runner)
      -> Core modules (internal/runtime, internal/controlplane, internal/observability, providers, transports)
        -> Artifact + Evidence pipeline (.codex/* JSON/MD)
```

Design intent:
- keep entrypoints thin and deterministic `[F-008]`, `[F-149]`
- centralize command semantics, error handling, and artifact contracts `[F-126]`, `[NF-015]`, `[F-028]`
- make feature/gate traceability explicit from command invocation to release evidence `[F-127]`, `[NF-028]`

## Code-Verified Snapshot (Current `cmd/*`)

Snapshot basis:
- verified against `cmd/rspp-runtime/main.go`, `cmd/rspp-cli/main.go`, `cmd/rspp-local-runner/main.go`, `cmd/rspp-control-plane/main.go`
- validated by existing tests in `cmd/rspp-runtime/main_test.go`, `cmd/rspp-cli/main_test.go`, `cmd/rspp-local-runner/main_test.go`

| Entrypoint | Current status | Implemented command contract (as of code) | Feature evidence |
| --- | --- | --- | --- |
| `rspp-runtime` | Implemented | Default action with no args is `bootstrap-providers`; explicit subcommands are `livekit`, `retention-sweep`, `help`; runtime telemetry setup executes before dispatch and can fail command startup | `[F-055]`, `[F-083]`, `[F-086]`, `[F-116]`, `[NF-015]` |
| `rspp-cli` | Implemented | Subcommands: `validate-contracts`, `validate-contracts-report`, `replay-smoke-report`, `replay-regression-report`, `generate-runtime-baseline`, `slo-gates-report`, `slo-gates-mvp-report`, `live-latency-compare-report`, `publish-release`; unknown command exits non-zero with usage output | `[F-126]`, `[F-127]`, `[F-072]`, `[F-077]`, `[NF-024]`, `[NF-028]`, `[NF-038]` |
| `rspp-local-runner` | Implemented | Thin pass-through to LiveKit CLI; default args enforce deterministic dry-run report path when no args are provided; supports `help` and optional `livekit` prefix | `[F-125]`, `[F-130]`, `[F-083]` |
| `rspp-control-plane` | Scaffold only | Prints scaffold banner only; no subcommands, no structured JSON outputs, no token/route flows yet | `[F-092]`, `[F-095]`, `[F-099]`, `[F-119]` |

### Current output/default contracts

| Command | Key default outputs/paths in code | Fail-closed behavior |
| --- | --- | --- |
| `rspp-runtime retention-sweep` | report default: `.codex/replay/retention-sweep-report.json`; mutates and rewrites `-store` artifact | rejects missing `-store`/`-tenants`, invalid `runs/interval`, deterministic policy errors with explicit codes |
| `rspp-runtime livekit` | report path is provided through LiveKit adapter flags; usage documents supported flags | invalid runtime/probe config propagates adapter error |
| `rspp-cli validate-contracts-report` | `.codex/ops/contracts-report.json` + Markdown sibling | fails when contract fixtures/schema validation fails |
| `rspp-cli replay-regression-report` | `.codex/replay/regression-report.json`; per-fixture artifacts under sibling `fixtures/` dir | fails when forbidden divergences are present or gate arg is invalid |
| `rspp-cli generate-runtime-baseline` | `.codex/replay/runtime-baseline.json` | fails if baseline generation path cannot complete |
| `rspp-cli slo-gates-report` | `.codex/ops/slo-gates-report.json` + Markdown sibling | fails when any MVP gate in baseline report fails |
| `rspp-cli slo-gates-mvp-report` | `.codex/ops/slo-gates-mvp-report.json` + Markdown sibling | fails on baseline/live/fixed-decision violations (with bounded waiver path for live E2E retries) |
| `rspp-cli live-latency-compare-report` | `.codex/providers/live-latency-compare.json` + Markdown sibling | fails on execution-mode mismatch, missing comparison identity, missing overlap evidence, or no combo intersection |
| `rspp-cli publish-release` | default output: `internal/tooling/release` manifest path + Markdown sibling | fails when readiness artifacts are missing/stale/failing |
| `rspp-local-runner` | default report: `.codex/transports/livekit-local-runner-report.json` | propagates LiveKit adapter error |

## Target-vs-Code Divergence (Implementation Gaps)

| Gap | Current code reality | Implementation impact | Required action |
| --- | --- | --- | --- |
| Shared command kernel is not yet present | Dispatch/parsing/output handling are duplicated across `cmd/rspp-runtime/main.go`, `cmd/rspp-cli/main.go`, and `cmd/rspp-local-runner/main.go` | inconsistent UX, error semantics, and extension integration surface | introduce `internal/tooling/commandcore/kernel` and migrate entrypoints incrementally `[F-126]`, `[NF-015]` |
| Control-plane command surface is unimplemented | `cmd/rspp-control-plane/main.go` is a scaffold banner only | cannot satisfy token/route bootstrap command contracts in CLI workflows | implement `issue-session-token` and `resolve-session-route` first with JSON contract tests `[F-099]`, `[F-119]`, `[F-095]` |
| Artifact writing is not centralized | each command path manually performs `MkdirAll + Marshal + WriteFile` | schema/version drift risk and repeated error handling logic | create shared artifact writer and normalize report metadata stamping `[F-028]`, `[F-149]`, `[NF-028]` |
| Extension hooks are architectural only | no command plugin registration path exists in `cmd/*` | extension model in this guide is not currently executable | add explicit extension host package and compatibility checks before loading plugins `[F-121]`, `[F-124]`, `[F-164]` |
| Exit-code contract is not unified | `rspp-cli` uses mixed `1/2`; other binaries mostly return `1`; usage behavior differs by binary | automation wrappers must encode binary-specific behavior | define shared exit-code policy in kernel and align binaries to it `[NF-015]`, `[F-126]` |
| Structured output contracts differ by command | some outputs embed rich gate metadata; others provide only summaries; control-plane has none | harder to compose universal command pipelines for operations tooling | define command descriptor schema and minimum output envelope fields across all entrypoints `[F-126]`, `[F-149]`, `[F-097]` |

## Implementation-Ready Plan (Cmd Scope)

1. `WP-1` Build control-plane MVP command surface.
Scope: `cmd/rspp-control-plane/main.go`, new tests in `cmd/rspp-control-plane/main_test.go`, service wiring in `internal/tooling/entrypoints/controlplaneops`.
Deliverables: `issue-session-token` and `resolve-session-route` commands with stable JSON outputs containing session identity, pipeline version, lease epoch, expiry, and runtime endpoint.
Acceptance: command-level tests cover success, validation failure, stale authority rejection, and output schema stability.
Feature trace: `[F-099]`, `[F-119]`, `[F-095]`, `[F-101]`.

2. `WP-2` Introduce shared command kernel and migrate one binary at a time.
Scope: new `internal/tooling/commandcore/kernel` and binary migrations starting with `rspp-cli`, then `rspp-runtime`, then `rspp-local-runner`.
Deliverables: shared command descriptors, unified help renderer, normalized exit-code handling, shared arg-validation contract.
Acceptance: existing command tests remain green; usage/error behavior is deterministic across binaries.
Feature trace: `[F-126]`, `[NF-015]`.

3. `WP-3` Centralize artifact/evidence writing.
Scope: new `internal/tooling/commandcore/artifacts` and `internal/tooling/commandcore/evidence`; remove duplicated report-write logic from `cmd/rspp-runtime/main.go` and `cmd/rspp-cli/main.go`.
Deliverables: shared `WriteJSON`/`WriteMarkdownSummary` helpers with stable metadata envelope and OR-02 completeness gate hooks.
Acceptance: report outputs remain backward compatible at current paths; replay/gate release commands still fail closed.
Feature trace: `[F-028]`, `[F-149]`, `[NF-028]`, `[NF-024]`, `[NF-038]`.

4. `WP-4` Add extension host foundation without breaking core commands.
Scope: `internal/tooling/commandcore/extensions` and command registration hooks in kernel.
Deliverables: command plugin registration contract, safety validation, and disabled-by-default loading policy.
Acceptance: built-in command behavior unchanged when no extensions are loaded; invalid extension descriptors are rejected deterministically.
Feature trace: `[F-121]`, `[F-122]`, `[F-123]`, `[F-124]`, `[F-164]`.

5. `WP-5` Lock implementation conformance in `cmd` tests.
Scope: expand command tests for usage/output/exit behaviors; add golden checks for report path defaults where appropriate.
Deliverables: explicit regression tests for command surface stability and fail-closed behavior.
Acceptance: `go test ./cmd/...` passes and prevents accidental command contract drift.
Feature trace: `[F-126]`, `[F-127]`, `[NF-015]`, `[NF-028]`.

## 1) Module list with responsibilities

| Target module | Responsibilities | Entrypoints | Evidence |
| --- | --- | --- | --- |
| `internal/tooling/commandcore/kernel` | Command registration, dispatch, exit-code policy, shared help rendering | all | `[F-126]`, `[NF-015]` |
| `internal/tooling/commandcore/config` | Config layering and resolved effective config view (defaults -> env -> tenant/session overrides) | runtime, control-plane, local-runner, cli | `[F-080]`, `[F-081]`, `[F-079]` |
| `internal/tooling/commandcore/errors` | Typed, actionable validation/runtime errors with stable error codes and field paths | all | `[F-001]`, `[NF-013]`, `[NF-015]` |
| `internal/tooling/commandcore/artifacts` | Deterministic artifact writing, schema/version stamping, manifest indexing | runtime, cli, local-runner, control-plane | `[F-028]`, `[F-149]`, `[F-153]` |
| `internal/tooling/commandcore/evidence` | OR-02 and gate-evidence completeness checks before promotion operations | cli, runtime | `[NF-028]`, `[NF-024]`, `[NF-025]`, `[NF-026]`, `[NF-027]`, `[NF-029]`, `[NF-038]` |
| `internal/tooling/entrypoints/runtimeops` | Runtime commands: provider bootstrap, transport command bridge, retention sweep orchestration | `rspp-runtime` | `[F-055]`, `[F-083]`, `[F-086]`, `[F-116]` |
| `internal/tooling/entrypoints/controlplaneops` | Session token issuance, route resolution, rollout/lease-aware operator commands | `rspp-control-plane` | `[F-092]`, `[F-095]`, `[F-099]`, `[F-119]`, `[F-101]` |
| `internal/tooling/entrypoints/toolingops` | Contract lint/validate, replay smoke/regression, SLO reports, latency comparison, release publish | `rspp-cli` | `[F-126]`, `[F-127]`, `[F-072]`, `[F-077]`, `[F-074]`, `[NF-024]`, `[NF-025]`, `[NF-026]`, `[NF-027]`, `[NF-028]`, `[NF-029]`, `[NF-038]` |
| `internal/tooling/entrypoints/localrunnerops` | Single-machine runtime + CP stub lifecycle, deterministic dry-run defaults, optional live probe mode | `rspp-local-runner` | `[F-125]`, `[F-130]`, `[F-079]`, `[F-083]` |
| `internal/tooling/commandcore/security` | Redaction, token handling, tenant isolation markers in command outputs | all | `[F-114]`, `[F-117]`, `[F-119]`, `[NF-011]`, `[NF-012]` |
| `internal/tooling/commandcore/extensions` | Registration and lifecycle for command plugins, custom node/runtime extension descriptors | runtime, cli, local-runner | `[F-011]`, `[F-121]`, `[F-122]`, `[F-123]`, `[F-124]`, `[F-164]` |

## 2) Key interfaces (pseudocode)

```go
type Command interface {
    Descriptor() CommandDescriptor
    Execute(ctx context.Context, req CommandRequest) (CommandResult, error)
}

type CommandDescriptor struct {
    Binary          string            // rspp-runtime | rspp-control-plane | rspp-cli | rspp-local-runner
    Name            string            // subcommand name
    Summary         string
    InputSchemaRef  string
    OutputArtifacts []ArtifactSpec
    FeatureRefs     []string          // e.g. F-126, NF-028
}

type CommandKernel interface {
    Register(cmd Command) error
    Run(ctx context.Context, argv []string) ExitStatus
}

type ArtifactWriter interface {
    WriteJSON(path string, schemaVersion string, payload any) (ArtifactRef, error)
    WriteMarkdownSummary(path string, payload any) (ArtifactRef, error)
}

type GateEvaluator interface {
    EvaluateMVP(ctx context.Context, evidence EvidenceBundle) ([]GateResult, error)
    ValidateReplayBaseline(ctx context.Context, evidence EvidenceBundle) error // OR-02 completeness
}

type ControlPlaneCommandService interface {
    IssueSessionToken(ctx context.Context, req SessionTokenRequest) (SessionTokenBundle, error)
    ResolveSessionRoute(ctx context.Context, req RouteResolveRequest) (SessionRouteBundle, error)
}

type ExtensionHost interface {
    RegisterCommandPlugin(plugin CommandPlugin) error
    RegisterNodeFactory(factory NodeFactory) error
    RegisterRuntimeAdapter(adapter RuntimeAdapter) error
}
```

Interface trace map:

| Interface | Why it exists | Feature evidence |
| --- | --- | --- |
| `Command` / `CommandDescriptor` / `CommandKernel` | Stable command contract, discoverability, and consistent CLI ergonomics | `[F-126]`, `[NF-015]` |
| `ArtifactWriter` | Deterministic JSON/summary artifacts for gates, replay, and audits | `[F-028]`, `[F-149]`, `[F-153]` |
| `GateEvaluator` | Enforce MVP gates and replay-baseline completeness before promotion | `[NF-024]`, `[NF-025]`, `[NF-026]`, `[NF-027]`, `[NF-028]`, `[NF-029]`, `[NF-038]` |
| `ControlPlaneCommandService` | Session token + route issuance with authority context | `[F-095]`, `[F-099]`, `[F-119]` |
| `ExtensionHost` | Controlled plugin/custom-node/custom-runtime registration | `[F-121]`, `[F-122]`, `[F-123]`, `[F-124]`, `[F-164]` |

## 3) Data model: core entities and relations

| Entity | Purpose | Key relations | Feature evidence |
| --- | --- | --- | --- |
| `CommandSurface` | Top-level binary identity and versioned command surface | `CommandSurface` has many `CommandSpec` | `[F-126]`, `[F-125]` |
| `CommandSpec` | Subcommand contract (inputs, outputs, feature refs, stability) | `CommandSpec` has many `CommandInvocation` | `[F-126]`, `[NF-015]` |
| `CommandInvocation` | One command execution with resolved config and timestamps | `CommandInvocation` emits many `ArtifactRecord` and `ControlEvidence` | `[F-149]`, `[F-074]` |
| `ArtifactRecord` | Versioned artifact metadata (`path`, hash, schema version, created_at) | grouped into `EvidenceBundle` | `[F-028]`, `[F-153]` |
| `EvidenceBundle` | Cohesive gate/release evidence package for one pipeline/version/run | evaluated by many `GateResult` | `[NF-028]`, `[F-149]` |
| `GateResult` | Pass/fail + measured values for a gate and threshold set | consumed by `ReleaseManifest` | `[NF-024]`, `[NF-025]`, `[NF-026]`, `[NF-027]`, `[NF-029]`, `[NF-038]` |
| `ReleaseManifest` | Promotion decision record with required artifact refs | references one `EvidenceBundle` and all required `GateResult` | `[F-127]`, `[NF-028]` |
| `SessionTokenBundle` | Signed short-lived token + tenant/session metadata | produced by control-plane commands, consumed by transports | `[F-119]`, `[F-088]` |
| `SessionRouteBundle` | Runtime endpoint + routing snapshot + lease epoch | produced by control-plane commands, consumed by runtime/local-runner paths | `[F-099]`, `[F-095]`, `[F-159]` |
| `ExtensionDescriptor` | Metadata for command/node/runtime extension packages | validated by extension host before activation | `[F-121]`, `[F-164]`, `[NF-036]` |

Core flow relation:
`CommandSurface -> CommandSpec -> CommandInvocation -> ArtifactRecord -> EvidenceBundle -> GateResult -> ReleaseManifest` (`[F-126]`, `[F-127]`, `[NF-028]`)

## 4) Extension points: plugins, custom nodes, custom runtimes

| Extension point | Contract | MVP posture |
| --- | --- | --- |
| Command plugins | Add subcommands through `ExtensionHost.RegisterCommandPlugin`; must declare output artifacts + feature refs for traceability | Optional and non-gate-critical by default; gate-critical commands remain built-in `[F-126]`, `[F-164]` |
| Custom nodes | Register node factories and package/version independently from PipelineSpec | First-class `[F-011]`, `[F-121]`; external boundary with limits `[F-122]`, `[F-123]` |
| Untrusted node execution | Sandbox boundary and stricter execution policy for untrusted node packages | MVP boundary present, stronger WASM posture evolves later `[F-124]` |
| Custom runtimes | Register runtime adapters and execution profiles; require conformance profile and artifact compatibility checks | Simple profile baseline `[F-078]`, post-MVP conformance hardening `[F-164]` |
| Custom transports | Shared transport adapter contract for session/audio/control mapping | LiveKit MVP `[F-083]`; WebSocket/telephony follow same contract `[F-084]`, `[F-085]`, `[F-086]` |

## 5) Design invariants

1. Entrypoints are orchestration surfaces only; runtime semantics remain in domain modules, not `cmd/*` glue code. `[F-002]`, `[F-140]`
2. All gate-facing outputs are deterministic artifacts with stable schemas and explicit versions. `[F-028]`, `[F-149]`, `[F-153]`
3. Gate and release commands fail closed on missing, stale, or failing evidence. `[NF-024]`, `[NF-025]`, `[NF-026]`, `[NF-027]`, `[NF-028]`, `[NF-029]`, `[NF-038]`
4. OR-02 baseline replay evidence is mandatory for every accepted turn in promotion paths. `[NF-028]`
5. Control-plane route outputs must include authority context (lease/epoch) and stale authority must be rejected. `[F-095]`, `[F-156]`, `[NF-027]`
6. Session tokens are short-lived, signed, and never logged in raw form. `[F-119]`, `[F-117]`, `[NF-011]`
7. Validation commands return actionable failure details including field-path context. `[F-001]`, `[NF-013]`, `[NF-015]`
8. Cancellation fence semantics are preserved end-to-end; post-cancel output acceptance is invalid. `[F-175]`, `[NF-026]`
9. Replay tooling must support deterministic regression comparison with explicit divergence artifacts. `[F-072]`, `[F-077]`, `[NF-009]`
10. Local runner remains reproducible on a single machine with minimal control-plane dependencies. `[F-125]`, `[F-130]`
11. Transport command paths share one adapter contract across LiveKit/WebSocket/telephony variants. `[F-086]`, `[F-083]`, `[F-084]`, `[F-085]`
12. Simple mode is the default command behavior when advanced settings are absent; resolved config must be inspectable. `[F-079]`, `[F-081]`
13. Tenant isolation and RBAC context are preserved in command-side control-plane operations. `[F-114]`, `[F-115]`, `[NF-012]`
14. Extension activation requires explicit descriptor validation and bounded resource/time policies. `[F-122]`, `[F-123]`, `[F-124]`
15. Backward compatibility for command artifacts and schemas is governed, versioned, and testable across supported skew windows. `[NF-010]`, `[NF-032]`

## 6) Tradeoffs: major decisions and alternatives

| Decision | Alternative | Why this architecture chooses it |
| --- | --- | --- |
| Shared command kernel across all binaries | Fully separate parser/dispatcher logic per binary | Reduces drift in validation, errors, and output contracts; keeps feature traceability uniform. `[F-126]`, `[NF-015]` |
| JSON-first artifact contracts plus optional Markdown summaries | Human-readable stdout-first with ad hoc logs | CI/gates/replay require machine-verifiable artifacts; summaries remain for operator readability. `[F-127]`, `[F-077]`, `[NF-028]` |
| Fail-closed release/gate flow | Fail-open with warnings for missing artifacts | Promotion safety and PRD gate guarantees require hard failure behavior. `[NF-024]`, `[NF-025]`, `[NF-026]`, `[NF-027]`, `[NF-028]`, `[NF-029]`, `[NF-038]` |
| Built-in core commands, optional plugin commands | Fully dynamic plugin-only command surface | Core path stays deterministic and supportable; plugins remain for non-core/custom workflows. `[F-121]`, `[F-164]` |
| Local runner defaults to deterministic dry-run/simulated path | Live-only local runner | Dry-run ensures reproducibility and fast offline loops; live probes remain opt-in. `[F-125]`, `[F-130]` |
| Single `rspp-cli` for validation/replay/gates/release | Split each concern into separate binaries | One CLI improves discoverability and evidence-chain continuity; internal modules keep code boundaries explicit. `[F-126]`, `[F-127]` |
| Control-plane command path can operate with snapshot artifacts where possible | Always-online control-plane dependency for every command | Snapshot-driven flows improve deterministic CI and local workflows while preserving authoritative CP contracts at runtime boundaries. `[F-092]`, `[F-099]`, `[F-149]` |

## Implementation notes for current repository state

- Current command surfaces already align with most target responsibilities:
  - `rspp-runtime`: `bootstrap-providers`, `livekit`, `retention-sweep` `[F-055]`, `[F-083]`, `[F-116]`
  - `rspp-cli`: validation, replay, SLO/gate reports, release publish, latency compare `[F-126]`, `[F-127]`, `[F-072]`, `[F-077]`, `[NF-024]`, `[NF-028]`
  - `rspp-local-runner`: thin wrapper over LiveKit command path with deterministic default dry-run `[F-125]`, `[F-130]`, `[F-083]`
- `rspp-control-plane` is currently scaffold-only and should converge on `IssueSessionToken` and `ResolveSessionRoute` contracts first, because those are the minimum command surfaces needed for transport bootstrap and authority-safe routing. `[F-099]`, `[F-119]`, `[F-095]`

## Change checklist

1. Keep command outputs deterministic and artifact-path stable unless intentionally versioned.
2. Preserve feature-ID traceability for every new command/subcommand contract.
3. Update tests with command-surface changes:
   - `cmd/rspp-runtime/main_test.go`
   - `cmd/rspp-cli/main_test.go`
   - `cmd/rspp-local-runner/main_test.go`
   - `cmd/rspp-control-plane/*_test.go` (as command surface is implemented)
4. Re-run:
   - `go test ./cmd/...`
   - `make verify-quick`

## Related docs

- `docs/PRD.md`
- `docs/RSPP_features_framework.json`
- `docs/rspp_SystemDesign.md`
- `docs/CIValidationGates.md`
- `docs/SecurityDataHandlingBaseline.md`
