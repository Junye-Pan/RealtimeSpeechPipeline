# Repository Scaffold Lock and Module Ownership (MVP)

## 1. Purpose

Lock a non-normative but required starting repository scaffold for MVP implementation, based on `docs/ModularDesign.md` section 15, and assign an explicit owner for each initial module.

This lock is for execution discipline, not long-term architecture rigidity.

## 2. Lock scope

Applies to MVP implementation slice only:
- Go runtime/control-plane implementation
- LiveKit transport path
- one STT/LLM/TTS provider set
- OR-02 baseline replay evidence requirements

## 3. Locked scaffold (starting point)

```text
cmd/
  rspp-control-plane/
  rspp-runtime/
  rspp-local-runner/
  rspp-cli/
api/
  eventabi/
  controlplane/
  observability/
internal/
  controlplane/
    registry/
    normalizer/
    graphcompiler/
    policy/
    admission/
    security/
    lease/
    routingview/
    rollout/
    providerhealth/
  runtime/
    prelude/
    turnarbiter/
    planresolver/
    eventabi/
    lanes/
    executor/
    nodehost/
    provider/contracts/
    provider/registry/
    provider/invocation/
    buffering/
    flowcontrol/
    cancellation/
    budget/
    determinism/
    state/
    identity/
    transport/
    guard/
    localadmission/
    executionpool/
  observability/
    telemetry/
    timeline/
    replay/
  tooling/
    validation/
    regression/
    release/
    ops/
providers/
  stt/
  llm/
  tts/
transports/
  livekit/
test/
  contract/
  integration/
  replay/
  failover/
```

## 4. Owner groups

Use these owner groups in CODEOWNERS/team mapping:

- `CP-Team`: control-plane contracts and services
- `Runtime-Team`: runtime semantics and execution
- `ObsReplay-Team`: telemetry, timeline, replay
- `Transport-Team`: transport adapters/boundaries
- `Provider-Team`: provider adapters/invocation behavior
- `Security-Team`: security policy, redaction, replay access
- `DevEx-Team`: tooling, CI gates, release controls

## 5. Initial module-to-owner map

## 5.1 Control plane

| Module | Path | Owner |
| --- | --- | --- |
| CP-01 Pipeline Registry | `internal/controlplane/registry` | `CP-Team` |
| CP-02 Spec Normalizer/Validator | `internal/controlplane/normalizer` | `CP-Team` |
| CP-03 GraphDefinition Compiler | `internal/controlplane/graphcompiler` | `CP-Team` |
| CP-04 Policy Evaluation | `internal/controlplane/policy` | `CP-Team` |
| CP-05 Resource/Admission | `internal/controlplane/admission` | `CP-Team` |
| CP-06 Security/Tenant Isolation | `internal/controlplane/security` | `Security-Team` |
| CP-07 Placement Lease Authority | `internal/controlplane/lease` | `CP-Team` |
| CP-08 Routing View Publisher | `internal/controlplane/routingview` | `CP-Team` |
| CP-09 Rollout/Version Resolver | `internal/controlplane/rollout` | `CP-Team` |
| CP-10 Provider Health Aggregator | `internal/controlplane/providerhealth` | `CP-Team` |

## 5.2 Runtime

| Module | Path | Owner |
| --- | --- | --- |
| RK-02 Session Prelude | `internal/runtime/prelude` | `Runtime-Team` |
| RK-03 Turn Arbiter | `internal/runtime/turnarbiter` | `Runtime-Team` |
| RK-04 ResolvedTurnPlan Resolver | `internal/runtime/planresolver` | `Runtime-Team` |
| RK-05 Event ABI Gateway | `internal/runtime/eventabi` | `Runtime-Team` |
| RK-06 Lane Router | `internal/runtime/lanes` | `Runtime-Team` |
| RK-07 Graph Runtime Executor | `internal/runtime/executor` | `Runtime-Team` |
| RK-08 Node Runtime Host | `internal/runtime/nodehost` | `Runtime-Team` |
| RK-10 Provider Adapter Manager | `internal/runtime/provider/registry` | `Provider-Team` |
| RK-11 Provider Invocation Controller | `internal/runtime/provider/invocation` | `Provider-Team` |
| RK-12/13 Buffering + Watermarks | `internal/runtime/buffering` | `Runtime-Team` |
| RK-14 Flow Control | `internal/runtime/flowcontrol` | `Runtime-Team` |
| RK-16 Cancellation | `internal/runtime/cancellation` | `Runtime-Team` |
| RK-17 Budget | `internal/runtime/budget` | `Runtime-Team` |
| RK-19 Determinism | `internal/runtime/determinism` | `Runtime-Team` |
| RK-20 State Access | `internal/runtime/state` | `Runtime-Team` |
| RK-21 Identity/Correlation | `internal/runtime/identity` | `Runtime-Team` |
| RK-22/23 Transport boundary | `internal/runtime/transport` + `transports/livekit` | `Transport-Team` |
| RK-24 Runtime Contract Guard | `internal/runtime/guard` | `Runtime-Team` |
| RK-25 Local Admission Enforcer | `internal/runtime/localadmission` | `Runtime-Team` |
| RK-26 Execution Pool Manager | `internal/runtime/executionpool` | `Runtime-Team` |

## 5.3 Observability, replay, tooling

| Module | Path | Owner |
| --- | --- | --- |
| OR-01 Telemetry | `internal/observability/telemetry` | `ObsReplay-Team` |
| OR-02 Timeline Recorder | `internal/observability/timeline` | `ObsReplay-Team` |
| OR-03 Replay Engine | `internal/observability/replay` | `ObsReplay-Team` |
| DX-02 Validation Harness | `internal/tooling/validation` | `DevEx-Team` |
| DX-03 Replay Regression | `internal/tooling/regression` | `DevEx-Team` |
| DX-04 Release/Rollout CLI | `internal/tooling/release` + `cmd/rspp-cli` | `DevEx-Team` |
| DX-05 Ops SLO Pack | `internal/tooling/ops` | `DevEx-Team` |

## 6. Ownership operating rules

1. One primary owner per module path.
2. Cross-team changes require review from both touched module owners.
3. Contract files under `api/` require both `CP-Team` and `Runtime-Team` approval.
4. Security-sensitive paths require `Security-Team` review:
   - `internal/controlplane/security`
   - `internal/observability/replay`
   - any redaction/replay-access policy files

## 7. Change-control for scaffold lock

A scaffold change is allowed only if:
1. proposal references impacted section-4 contracts,
2. ownership remap is explicit,
3. CI gates in `docs/CIValidationGates.md` remain satisfiable,
4. module boundaries for non-goals are preserved.

Until then, this scaffold is the default implementation layout.
