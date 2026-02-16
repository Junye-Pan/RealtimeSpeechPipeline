# RSPP MVP Executable Backlog (Canonical-Guide Map)

_Generated: 2026-02-16 08:42Z_

## Inputs And Scope

- Authoritative sources read: `docs/rspp_SystemDesign.md`, `docs/RSPP_features_framework.json`, and all 15 canonical guides from system-design section 6.
- MVP filter applied: `release_phase == "mvp"`.
- Inventory totals: `214` total, `189` MVP, `25` post-MVP.
- Non-negotiable invariants used in risk/dependency tagging: Turn Plan Freeze, Lane Priority, Cancellation-first, Authority/Epoch, OR-02 replay evidence, simple/v1-only profile, stateless runtime compute.

## A) MVP Backlog (Grouped By Canonical Area)

Each item includes feature IDs/WP IDs, target path prefixes, dependencies, PR boundary, done criteria, and risk flags.

### Command entrypoints

- Canonical guide: `cmd/command_entrypoints_guide.md`
- Area path prefixes: `cmd/`
- Work package anchors: `NF-009, NF-010, NF-011, NF-012, NF-013, NF-015, NF-024, NF-025, NF-026, NF-027, NF-028, NF-029`

- **BL-001 | Group:** `Operability`
  - Feature IDs: `NF-016`
  - Target module path prefix(es): `internal/tooling/, cmd/`
  - Dependency edges: `requires tooling/runtime/controlplane command surfaces`
  - PR boundary suggestion: `single feature-group PR constrained to internal/tooling/, cmd/ (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in make verify-quick.`
  - Risk flags: `contract compatibility risk`

- **BL-002 | Group:** `Tooling: Local Runner, CI/CD & Testing`
  - Feature IDs: `F-125..F-130`
  - Target module path prefix(es): `internal/tooling/, cmd/`
  - Dependency edges: `requires tooling/runtime/controlplane command surfaces`
  - PR boundary suggestion: `single feature-group PR constrained to internal/tooling/, cmd/ (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in make verify-quick.`
  - Risk flags: `scope drift risk if PR boundary expands`

- **BL-003 | Group:** `Usability`
  - Feature IDs: `NF-015`
  - Target module path prefix(es): `internal/tooling/, cmd/`
  - Dependency edges: `requires tooling/runtime/controlplane command surfaces`
  - PR boundary suggestion: `single feature-group PR constrained to internal/tooling/, cmd/ (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in make verify-quick.`
  - Risk flags: `contract compatibility risk`

### Control-plane internals

- Canonical guide: `internal/controlplane/controlplane_module_guide.md`
- Area path prefixes: `internal/controlplane/`
- Work package anchors: `CP-01, CP-02, CP-03, CP-04, CP-05, CP-06, CP-07, CP-08, CP-09, CP-10, CP-11, CP-12`

- **BL-004 | Group:** `Correctness`
  - Feature IDs: `NF-018`
  - Target module path prefix(es): `internal/runtime/state, internal/runtime/identity, internal/controlplane/lease, internal/controlplane/routingview`
  - Dependency edges: `requires control-plane API contracts; consumes Event ABI authority semantics`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/state, internal/runtime/identity, internal/controlplane/lease, internal/controlplane/routingview (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in controlplane unit tests, test/contract, test/failover, make verify-quick.`
  - Risk flags: `stale-authority/epoch safety risk`

- **BL-005 | Group:** `Reliability & Resilience`
  - Feature IDs: `NF-004..NF-006, NF-030..NF-031`
  - Target module path prefix(es): `internal/runtime/state, internal/runtime/identity, internal/controlplane/lease, internal/controlplane/routingview`
  - Dependency edges: `requires control-plane API contracts; consumes Event ABI authority semantics; secondary touchpoints: Test and conformance suites`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/state, internal/runtime/identity, internal/controlplane/lease, internal/controlplane/routingview (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in controlplane unit tests, test/contract, test/failover, make verify-quick.`
  - Risk flags: `contract compatibility risk, stale-authority/epoch safety risk`
  - Mapping assumption: `Multiple module candidates; primary chosen by canonical-area precedence and module-name tie-breaker.`

- **BL-006 | Group:** `Security`
  - Feature IDs: `NF-011..NF-013`
  - Target module path prefix(es): `internal/security/, internal/controlplane/security, internal/observability/replay`
  - Dependency edges: `requires control-plane API contracts; consumes Event ABI authority semantics`
  - PR boundary suggestion: `single feature-group PR constrained to internal/security/, internal/controlplane/security, internal/observability/replay (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in controlplane unit tests, test/contract, test/failover, make verify-quick.`
  - Risk flags: `data exposure / tenancy boundary risk, stale-authority/epoch safety risk`

- **BL-007 | Group:** `Security, Tenancy & Compliance`
  - Feature IDs: `F-114..F-120`
  - Target module path prefix(es): `internal/security/, internal/controlplane/security, internal/observability/replay`
  - Dependency edges: `requires control-plane API contracts; consumes Event ABI authority semantics`
  - PR boundary suggestion: `single feature-group PR constrained to internal/security/, internal/controlplane/security, internal/observability/replay (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in controlplane unit tests, test/contract, test/failover, make verify-quick.`
  - Risk flags: `data exposure / tenancy boundary risk, stale-authority/epoch safety risk`

- **BL-008 | Group:** `State, Authority & Migration`
  - Feature IDs: `F-155..F-160`
  - Target module path prefix(es): `internal/controlplane/`
  - Dependency edges: `requires control-plane API contracts; consumes Event ABI authority semantics; secondary touchpoints: Control-plane internals, Runtime kernel internals, Transport adapters`
  - PR boundary suggestion: `single feature-group PR constrained to internal/controlplane/ (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in controlplane unit tests, test/contract, test/failover, make verify-quick.`
  - Risk flags: `stale-authority/epoch safety risk`
  - Mapping assumption: `Multiple module candidates; primary chosen by canonical-area precedence and module-name tie-breaker.`

### Runtime kernel internals

- Canonical guide: `internal/runtime/runtime_kernel_guide.md`
- Area path prefixes: `internal/runtime/`
- Work package anchors: `NF-004, NF-008, NF-009, NF-018, NF-019, NF-021, NF-024, NF-027, NF-028, NF-029, NF-038, OR-02`

- **BL-009 | Group:** `Backpressure, Budgets & Degradation`
  - Feature IDs: `F-043..F-054`
  - Target module path prefix(es): `internal/runtime/eventabi, internal/runtime/lanes, internal/runtime/executor, internal/runtime/nodehost, internal/runtime/buffering, internal/runtime/flowcontrol, internal/runtime/budget, internal/runtime/cancellation, internal/runtime/timebase, internal/runtime/sync, internal/runtime/executionpool`
  - Dependency edges: `requires Event ABI + control-plane API contracts and control-plane authority context`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/eventabi, internal/runtime/lanes, internal/runtime/executor, internal/runtime/nodehost, internal/runtime/buffering, internal/runtime/flowcontrol, internal/runtime/budget, internal/runtime/cancellation, internal/runtime/timebase, internal/runtime/sync, internal/runtime/executionpool (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in runtime unit tests, test/integration, test/replay, make verify-quick.`
  - Risk flags: `tail-latency and memory pressure risk`

- **BL-010 | Group:** `Boundaries (Non-goals)`
  - Feature IDs: `NF-019..NF-023`
  - Target module path prefix(es): `internal/runtime/`
  - Dependency edges: `requires Event ABI + control-plane API contracts and control-plane authority context`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/ (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in runtime unit tests, test/integration, test/replay, make verify-quick.`
  - Risk flags: `scope drift risk if PR boundary expands`
  - Mapping assumption: `No direct match in section 4 target-module table; fallback to Runtime kernel internals.`

- **BL-011 | Group:** `Cancellation & Turn Interrupts`
  - Feature IDs: `F-033..F-042`
  - Target module path prefix(es): `internal/runtime/session, internal/runtime/prelude, internal/runtime/turnarbiter, internal/runtime/planresolver, internal/runtime/guard, internal/runtime/localadmission`
  - Dependency edges: `requires Event ABI + control-plane API contracts and control-plane authority context; secondary touchpoints: Runtime kernel internals, Transport adapters`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/session, internal/runtime/prelude, internal/runtime/turnarbiter, internal/runtime/planresolver, internal/runtime/guard, internal/runtime/localadmission (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in runtime unit tests, test/integration, test/replay, make verify-quick.`
  - Risk flags: `cancel-path regression risk`
  - Mapping assumption: `Multiple module candidates; primary chosen by canonical-area precedence and module-name tie-breaker.`

- **BL-012 | Group:** `Compatibility & Evolution`
  - Feature IDs: `NF-010, NF-032`
  - Target module path prefix(es): `internal/runtime/`
  - Dependency edges: `requires Event ABI + control-plane API contracts and control-plane authority context`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/ (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in runtime unit tests, test/integration, test/replay, make verify-quick.`
  - Risk flags: `contract compatibility risk`
  - Mapping assumption: `No direct match in section 4 target-module table; fallback to Runtime kernel internals.`

- **BL-013 | Group:** `Cost`
  - Feature IDs: `NF-017`
  - Target module path prefix(es): `internal/runtime/`
  - Dependency edges: `requires Event ABI + control-plane API contracts and control-plane authority context`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/ (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in runtime unit tests, test/integration, test/replay, make verify-quick.`
  - Risk flags: `scope drift risk if PR boundary expands`
  - Mapping assumption: `No direct match in section 4 target-module table; fallback to Runtime kernel internals.`

- **BL-014 | Group:** `Edge Buffering, Lanes & Flow Control`
  - Feature IDs: `F-142..F-148`
  - Target module path prefix(es): `internal/runtime/eventabi, internal/runtime/lanes, internal/runtime/executor, internal/runtime/nodehost, internal/runtime/buffering, internal/runtime/flowcontrol, internal/runtime/budget, internal/runtime/cancellation, internal/runtime/timebase, internal/runtime/sync, internal/runtime/executionpool`
  - Dependency edges: `requires Event ABI + control-plane API contracts and control-plane authority context`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/eventabi, internal/runtime/lanes, internal/runtime/executor, internal/runtime/nodehost, internal/runtime/buffering, internal/runtime/flowcontrol, internal/runtime/budget, internal/runtime/cancellation, internal/runtime/timebase, internal/runtime/sync, internal/runtime/executionpool (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in runtime unit tests, test/integration, test/replay, make verify-quick.`
  - Risk flags: `tail-latency and memory pressure risk`

- **BL-015 | Group:** `Extensibility: Custom/External Nodes`
  - Feature IDs: `F-121..F-124`
  - Target module path prefix(es): `internal/runtime/`
  - Dependency edges: `requires Event ABI + control-plane API contracts and control-plane authority context`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/ (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in runtime unit tests, test/integration, test/replay, make verify-quick.`
  - Risk flags: `scope drift risk if PR boundary expands`
  - Mapping assumption: `No direct match in section 4 target-module table; fallback to Runtime kernel internals.`

- **BL-016 | Group:** `Kubernetes Runtime, Scaling & Scheduling`
  - Feature IDs: `F-103..F-109`
  - Target module path prefix(es): `internal/runtime/eventabi, internal/runtime/lanes, internal/runtime/executor, internal/runtime/nodehost, internal/runtime/buffering, internal/runtime/flowcontrol, internal/runtime/budget, internal/runtime/cancellation, internal/runtime/timebase, internal/runtime/sync, internal/runtime/executionpool`
  - Dependency edges: `requires Event ABI + control-plane API contracts and control-plane authority context`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/eventabi, internal/runtime/lanes, internal/runtime/executor, internal/runtime/nodehost, internal/runtime/buffering, internal/runtime/flowcontrol, internal/runtime/budget, internal/runtime/cancellation, internal/runtime/timebase, internal/runtime/sync, internal/runtime/executionpool (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in runtime unit tests, test/integration, test/replay, make verify-quick.`
  - Risk flags: `scope drift risk if PR boundary expands`

- **BL-017 | Group:** `PipelineSpec & Graph Execution`
  - Feature IDs: `F-001..F-019`
  - Target module path prefix(es): `internal/runtime/eventabi, internal/runtime/lanes, internal/runtime/executor, internal/runtime/nodehost, internal/runtime/buffering, internal/runtime/flowcontrol, internal/runtime/budget, internal/runtime/cancellation, internal/runtime/timebase, internal/runtime/sync, internal/runtime/executionpool`
  - Dependency edges: `requires Event ABI + control-plane API contracts and control-plane authority context`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/eventabi, internal/runtime/lanes, internal/runtime/executor, internal/runtime/nodehost, internal/runtime/buffering, internal/runtime/flowcontrol, internal/runtime/budget, internal/runtime/cancellation, internal/runtime/timebase, internal/runtime/sync, internal/runtime/executionpool (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in runtime unit tests, test/integration, test/replay, make verify-quick.`
  - Risk flags: `scope drift risk if PR boundary expands`

- **BL-018 | Group:** `Runtime Semantics & Lifecycle`
  - Feature IDs: `F-138..F-141, F-175`
  - Target module path prefix(es): `internal/runtime/session, internal/runtime/prelude, internal/runtime/turnarbiter, internal/runtime/planresolver, internal/runtime/guard, internal/runtime/localadmission`
  - Dependency edges: `requires Event ABI + control-plane API contracts and control-plane authority context`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/session, internal/runtime/prelude, internal/runtime/turnarbiter, internal/runtime/planresolver, internal/runtime/guard, internal/runtime/localadmission (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in runtime unit tests, test/integration, test/replay, make verify-quick.`
  - Risk flags: `scope drift risk if PR boundary expands`

- **BL-019 | Group:** `Security & Privacy`
  - Feature IDs: `NF-014`
  - Target module path prefix(es): `internal/runtime/`
  - Dependency edges: `requires Event ABI + control-plane API contracts and control-plane authority context`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/ (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in runtime unit tests, test/integration, test/replay, make verify-quick.`
  - Risk flags: `data exposure / tenancy boundary risk`
  - Mapping assumption: `No direct match in section 4 target-module table; fallback to Runtime kernel internals.`

- **BL-020 | Group:** `Simple Mode & Configuration`
  - Feature IDs: `F-078..F-082`
  - Target module path prefix(es): `internal/runtime/session, internal/runtime/prelude, internal/runtime/turnarbiter, internal/runtime/planresolver, internal/runtime/guard, internal/runtime/localadmission`
  - Dependency edges: `requires Event ABI + control-plane API contracts and control-plane authority context`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/session, internal/runtime/prelude, internal/runtime/turnarbiter, internal/runtime/planresolver, internal/runtime/guard, internal/runtime/localadmission (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in runtime unit tests, test/integration, test/replay, make verify-quick.`
  - Risk flags: `scope drift risk if PR boundary expands`

- **BL-021 | Group:** `Transport Adapters (LiveKit/WebSocket/Telephony)`
  - Feature IDs: `F-083, F-086..F-091`
  - Target module path prefix(es): `internal/runtime/transport, transports/livekit, transports/websocket, transports/telephony`
  - Dependency edges: `requires Event ABI + control-plane API contracts and control-plane authority context`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/transport, transports/livekit, transports/websocket, transports/telephony (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in runtime unit tests, test/integration, test/replay, make verify-quick.`
  - Risk flags: `external dependency flakiness risk`

### Observability and replay internals

- Canonical guide: `internal/observability/observability_module_guide.md`
- Area path prefixes: `internal/observability/`
- Work package anchors: `NF-002, NF-008, NF-009, NF-011, NF-014, NF-018, NF-024, NF-025, NF-026, NF-027, NF-028, NF-029`

- **BL-022 | Group:** `MVP Quality Gates`
  - Feature IDs: `NF-024..NF-029, NF-038`
  - Target module path prefix(es): `internal/observability/telemetry, internal/observability/timeline, internal/observability/replay`
  - Dependency edges: `requires observability API contracts and runtime lifecycle markers; secondary touchpoints: Command entrypoints, Test and conformance suites, Tooling and release gates internals`
  - PR boundary suggestion: `single feature-group PR constrained to internal/observability/telemetry, internal/observability/timeline, internal/observability/replay (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in observability unit tests, test/replay, make verify-quick.`
  - Risk flags: `contract compatibility risk, replay evidence completeness risk`
  - Mapping assumption: `Multiple module candidates; primary chosen by canonical-area precedence and module-name tie-breaker.`

- **BL-023 | Group:** `Observability`
  - Feature IDs: `NF-008`
  - Target module path prefix(es): `internal/observability/telemetry, internal/observability/timeline, internal/observability/replay`
  - Dependency edges: `requires observability API contracts and runtime lifecycle markers`
  - PR boundary suggestion: `single feature-group PR constrained to internal/observability/telemetry, internal/observability/timeline, internal/observability/replay (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in observability unit tests, test/replay, make verify-quick.`
  - Risk flags: `contract compatibility risk, replay evidence completeness risk`

- **BL-024 | Group:** `Observability, Recording & Replay`
  - Feature IDs: `F-067..F-077`
  - Target module path prefix(es): `internal/observability/telemetry, internal/observability/timeline, internal/observability/replay`
  - Dependency edges: `requires observability API contracts and runtime lifecycle markers`
  - PR boundary suggestion: `single feature-group PR constrained to internal/observability/telemetry, internal/observability/timeline, internal/observability/replay (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in observability unit tests, test/replay, make verify-quick.`
  - Risk flags: `contract compatibility risk, replay evidence completeness risk`

### Tooling and release gates internals

- Canonical guide: `internal/tooling/tooling_and_gates_guide.md`
- Area path prefixes: `internal/tooling/`
- Work package anchors: `NF-009, NF-010, NF-015, NF-016, NF-024, NF-025, NF-026, NF-027, NF-028, NF-029, NF-032, NF-038`

- **BL-025 | Guide Alignment:** `Tooling and release gates internals MVP backlog closure`
  - Feature IDs / WP IDs: `F-001, F-011, F-028, F-078, F-093, F-095, F-100, F-121..F-128, F-130, F-149..F-154, F-162..F-165, F-176, NF-009..NF-010, NF-015..NF-016, NF-024..NF-029, NF-032, NF-038`
  - Target module path prefix(es): `internal/tooling/`
  - Dependency edges: `requires observability artifacts and stable api/* schemas`
  - PR boundary suggestion: `single-area guide-aligned PR constrained to internal/tooling/`
  - Done criteria: `Close outstanding divergence/backlog items in canonical guide; add/update tests in security-baseline-check, verify-full, verify-quick, make verify-quick.`
  - Risk flags: `scope drift risk if PR boundary expands`

### Internal shared helpers

- Canonical guide: `internal/shared/internal_shared_guide.md`
- Area path prefixes: `internal/shared/`
- Work package anchors: `NF-004, NF-008, NF-009, NF-010, NF-011, NF-014, NF-015, NF-018, NF-019, NF-020, NF-024, NF-025`

- **BL-026 | Guide Alignment:** `Internal shared helpers MVP backlog closure`
  - Feature IDs / WP IDs: `F-005, F-008, F-011, F-013, F-020..F-022, F-028, F-048, F-073, F-078..F-081, F-117..F-118, F-121..F-124, F-126, F-129, F-131, F-133, F-136..F-137, F-139, F-141..F-143, F-146, F-149..F-150, F-152..F-156, F-158, F-160..F-162, F-164, F-175..F-176, NF-004, NF-008..NF-011, NF-014..NF-015, NF-018..NF-020, NF-024..NF-029, NF-032`
  - Target module path prefix(es): `internal/shared/`
  - Dependency edges: `depends on upstream internal contracts; no new public behavior`
  - PR boundary suggestion: `single-area guide-aligned PR constrained to internal/shared/`
  - Done criteria: `Close outstanding divergence/backlog items in canonical guide; add/update tests in make verify-quick.`
  - Risk flags: `scope drift risk if PR boundary expands`

### Provider adapters

- Canonical guide: `providers/provider_adapter_guide.md`
- Area path prefixes: `providers/, internal/runtime/provider/`
- Work package anchors: `NF-001, NF-002, NF-003, NF-006, NF-013, NF-019, NF-023, OR-02`

- **BL-027 | Group:** `Error Handling & Recovery`
  - Feature IDs: `F-131..F-137`
  - Target module path prefix(es): `internal/runtime/provider/, providers/`
  - Dependency edges: `requires Event ABI contracts and runtime provider interfaces`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/provider/, providers/ (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in live-provider smoke (optional), provider adapter unit tests, runtime unit tests, test/integration, make verify-quick.`
  - Risk flags: `external dependency flakiness risk`

- **BL-028 | Group:** `Latency & Performance`
  - Feature IDs: `NF-001..NF-003`
  - Target module path prefix(es): `internal/runtime/provider/, providers/`
  - Dependency edges: `requires Event ABI contracts and runtime provider interfaces`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/provider/, providers/ (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in live-provider smoke (optional), provider adapter unit tests, runtime unit tests, test/integration, make verify-quick.`
  - Risk flags: `external dependency flakiness risk, tail-latency and memory pressure risk`

- **BL-029 | Group:** `Provider Adapters & Policies`
  - Feature IDs: `F-055..F-066`
  - Target module path prefix(es): `internal/runtime/provider/, providers/`
  - Dependency edges: `requires Event ABI contracts and runtime provider interfaces`
  - PR boundary suggestion: `single feature-group PR constrained to internal/runtime/provider/, providers/ (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in live-provider smoke (optional), provider adapter unit tests, runtime unit tests, test/integration, make verify-quick.`
  - Risk flags: `external dependency flakiness risk`

### Transport adapters

- Canonical guide: `transports/transport_adapter_guide.md`
- Area path prefixes: `transports/, internal/runtime/transport/`
- Work package anchors: `LK-001, LK-002, LK-003, NF-004, NF-005, NF-006, NF-008, NF-030, NF-031, OR-02`

- **BL-030 | Guide Alignment:** `Transport adapters MVP backlog closure`
  - Feature IDs / WP IDs: `F-011, F-055, F-062, F-065, F-067, F-083..F-091, F-168..F-170, NF-004..NF-006, NF-008, NF-030..NF-031`
  - Target module path prefix(es): `transports/, internal/runtime/transport/`
  - Dependency edges: `requires Event ABI contracts and runtime cancellation/output-fencing behavior`
  - PR boundary suggestion: `single-area guide-aligned PR constrained to transports/, internal/runtime/transport/`
  - Done criteria: `Close outstanding divergence/backlog items in canonical guide; add/update tests in livekit smoke (optional), runtime unit tests, test/integration, test/replay, make verify-quick.`
  - Risk flags: `external dependency flakiness risk`

### Test and conformance suites

- Canonical guide: `test/test_suite_guide.md`
- Area path prefixes: `test/`
- Work package anchors: `NF-001, NF-003, NF-004, NF-006, NF-008, NF-009, NF-017, NF-019, NF-024, NF-025, NF-026, NF-027`

- **BL-031 | Group:** `Testability`
  - Feature IDs: `NF-009`
  - Target module path prefix(es): `test/contract, test/integration, test/replay, test/failover, test/load`
  - Dependency edges: `depends on target implementation streams and stable contracts`
  - PR boundary suggestion: `single feature-group PR constrained to test/contract, test/integration, test/replay, test/failover, test/load (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in go test ./test/..., verify-full, verify-quick, make verify-quick.`
  - Risk flags: `contract compatibility risk`

### Pipeline artifact scaffolds

- Canonical guide: `pipelines/pipeline_scaffolds_guide.md`
- Area path prefixes: `pipelines/`
- Work package anchors: `NF-004, NF-010, NF-024, NF-025, NF-026, NF-027, NF-028, NF-029, NF-032, NF-038, OR-02`

- **BL-032 | Guide Alignment:** `Pipeline artifact scaffolds MVP backlog closure`
  - Feature IDs / WP IDs: `F-001..F-003, F-005..F-006, F-009..F-013, F-015..F-016, F-018..F-021, F-025, F-028..F-029, F-032..F-034, F-036, F-043..F-044, F-048, F-056..F-057, F-062, F-066..F-067, F-071..F-072, F-078..F-079, F-081..F-083, F-086, F-092..F-093, F-095, F-098, F-100, F-102, F-110, F-113, F-121..F-130, F-139, F-141..F-144, F-148..F-151, F-153..F-156, F-159, F-161..F-165, F-168, F-171..F-172, F-175..F-176, NF-004, NF-010, NF-024..NF-029, NF-032, NF-038`
  - Target module path prefix(es): `pipelines/`
  - Dependency edges: `requires simple/v1 profile enforcement and control/event contracts`
  - PR boundary suggestion: `single-area guide-aligned PR constrained to pipelines/`
  - Done criteria: `Close outstanding divergence/backlog items in canonical guide; add/update tests in make verify-quick.`
  - Risk flags: `scope drift risk if PR boundary expands`

### Deployment scaffolds

- Canonical guide: `deploy/deployment_scaffolds_guide.md`
- Area path prefixes: `deploy/`
- Work package anchors: `NF-004, NF-005, NF-006, NF-007, NF-008, NF-010, NF-011, NF-012, NF-014, NF-016, NF-024, NF-027`

- **BL-033 | Group:** `Portability`
  - Feature IDs: `NF-007`
  - Target module path prefix(es): `pipelines/, deploy/, pkg/contracts`
  - Dependency edges: `requires command/tooling/gate contracts`
  - PR boundary suggestion: `single feature-group PR constrained to pipelines/, deploy/, pkg/contracts (no cross-area refactor)`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in make verify-quick.`
  - Risk flags: `contract compatibility risk`

### Public package contract scaffolds

- Canonical guide: `pkg/public_package_scaffold_guide.md`
- Area path prefixes: `pkg/`
- Work package anchors: `NF-010, NF-024, NF-027, NF-028, NF-029, NF-032, NF-038, OR-02`

- **BL-034 | Guide Alignment:** `Public package contract scaffolds MVP backlog closure`
  - Feature IDs / WP IDs: `F-001, F-003..F-007, F-011..F-013, F-015..F-016, F-018..F-022, F-025..F-026, F-028, F-032..F-034, F-036, F-040..F-041, F-043, F-046, F-054..F-055, F-057, F-059, F-063..F-067, F-071..F-073, F-077..F-086, F-089..F-092, F-095, F-098, F-100, F-102, F-110, F-113..F-114, F-117, F-119..F-121, F-124..F-130, F-138..F-145, F-147..F-150, F-153..F-156, F-158..F-161, F-163..F-165, F-175..F-176, NF-010, NF-024, NF-027..NF-029, NF-032, NF-038`
  - Target module path prefix(es): `pkg/`
  - Dependency edges: `requires stable api/* and replay/control contract surfaces`
  - PR boundary suggestion: `single-area guide-aligned PR constrained to pkg/`
  - Done criteria: `Close outstanding divergence/backlog items in canonical guide; add/update tests in make verify-quick.`
  - Risk flags: `contract compatibility risk`

### Control-plane API contracts

- Canonical guide: `api/controlplane/controlplane_api_guide.md`
- Area path prefixes: `api/controlplane/`
- Work package anchors: `CPAPI-01, CPAPI-02, CPAPI-03, CPAPI-04, CPAPI-05, CPAPI-06, CPAPI-07, CPAPI-08, CPAPI-09, CPAPI-10, CPAPI-11, CPAPI-12`

- **BL-035 | Group:** `Control Plane (Registry, Routing, Rollouts)`
  - Feature IDs: `F-092..F-102`
  - Target module path prefix(es): `api/controlplane, api/eventabi, api/observability`
  - Dependency edges: `none; foundation for control-plane and runtime streams; secondary touchpoints: Control-plane internals`
  - PR boundary suggestion: `contract-only PR in api/controlplane, api/eventabi, api/observability plus contract tests; implementation follows in dependent streams`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in api/* package tests, test/contract, make verify-quick.`
  - Risk flags: `contract compatibility risk, stale-authority/epoch safety risk`
  - Mapping assumption: `Multiple module candidates; primary chosen by canonical-area precedence and module-name tie-breaker.`

- **BL-036 | Group:** `Determinism, Replay & Provenance`
  - Feature IDs: `F-149..F-154, F-176`
  - Target module path prefix(es): `api/controlplane, api/eventabi, api/observability`
  - Dependency edges: `none; foundation for control-plane and runtime streams; secondary touchpoints: Observability and replay internals`
  - PR boundary suggestion: `contract-only PR in api/controlplane, api/eventabi, api/observability plus contract tests; implementation follows in dependent streams`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in api/* package tests, test/contract, make verify-quick.`
  - Risk flags: `contract compatibility risk, replay evidence completeness risk, stale-authority/epoch safety risk`
  - Mapping assumption: `Multiple module candidates; primary chosen by canonical-area precedence and module-name tie-breaker.`

- **BL-037 | Group:** `Unified Event ABI & Schema`
  - Feature IDs: `F-020..F-032`
  - Target module path prefix(es): `api/controlplane, api/eventabi, api/observability`
  - Dependency edges: `none; foundation for control-plane and runtime streams`
  - PR boundary suggestion: `contract-only PR in api/controlplane, api/eventabi, api/observability plus contract tests; implementation follows in dependent streams`
  - Done criteria: `Implement acceptance steps from docs/RSPP_features_framework.json for listed IDs; add/update tests in api/* package tests, test/contract, make verify-quick.`
  - Risk flags: `contract compatibility risk, stale-authority/epoch safety risk`

### Event ABI API contracts

- Canonical guide: `api/eventabi/eventabi_api_guide.md`
- Area path prefixes: `api/eventabi/`
- Work package anchors: `NF-008, NF-009, NF-010, NF-011, NF-013, NF-014, NF-018, NF-028, NF-029, NF-032, RK-02`

- **BL-038 | Guide Alignment:** `Event ABI API contracts MVP backlog closure`
  - Feature IDs / WP IDs: `F-008, F-011, F-013, F-020..F-035, F-038..F-039, F-083..F-086, F-090, F-117..F-118, F-121..F-124, F-139, F-141..F-151, F-153..F-154, F-156, F-158..F-164, F-168..F-171, F-175..F-176, NF-008..NF-011, NF-013..NF-014, NF-018, NF-028..NF-029, NF-032`
  - Target module path prefix(es): `api/eventabi/`
  - Dependency edges: `none; foundation for runtime, provider, transport, and replay streams`
  - PR boundary suggestion: `single-area guide-aligned PR constrained to api/eventabi/`
  - Done criteria: `Close outstanding divergence/backlog items in canonical guide; add/update tests in api/* package tests, test/contract, make verify-quick.`
  - Risk flags: `contract compatibility risk`

### Observability API contracts

- Canonical guide: `api/observability/observability_api_guide.md`
- Area path prefixes: `api/observability/`
- Work package anchors: `NF-002, NF-008, NF-009, NF-024, NF-026, NF-027, NF-028, NF-029, NF-038, OR-02`

- **BL-039 | Guide Alignment:** `Observability API contracts MVP backlog closure`
  - Feature IDs / WP IDs: `F-026, F-067, F-069..F-073, F-077, F-116, F-121, F-124, F-127, F-138..F-139, F-141..F-142, F-149..F-154, F-156, F-160, F-164, F-175..F-176, NF-002, NF-008..NF-009, NF-024, NF-026..NF-029, NF-038`
  - Target module path prefix(es): `api/observability/`
  - Dependency edges: `none; foundation for observability and tooling streams`
  - PR boundary suggestion: `single-area guide-aligned PR constrained to api/observability/`
  - Done criteria: `Close outstanding divergence/backlog items in canonical guide; add/update tests in api/* package tests, test/contract, make verify-quick.`
  - Risk flags: `contract compatibility risk, replay evidence completeness risk`

Backlog coverage check: mapped MVP IDs `189` / expected `189`.

## B) Parallel Workstreams (>=3) With Strict Allowed Path Prefixes

- `WS-1 Contracts`: allowed `api/controlplane/`, `api/eventabi/`, `api/observability/`, `test/contract/`; blocks all downstream streams.
- `WS-2 Control-plane Authority`: allowed `internal/controlplane/`, `internal/runtime/planresolver/`, `internal/runtime/turnarbiter/`, `internal/runtime/guard/`; depends on WS-1.
- `WS-3 Runtime Core`: allowed `internal/runtime/session/`, `internal/runtime/prelude/`, `internal/runtime/executor/`, `internal/runtime/lanes/`, `internal/runtime/cancellation/`, `internal/runtime/buffering/`, `internal/runtime/flowcontrol/`, `internal/runtime/budget/`; depends on WS-1 and WS-2.
- `WS-4 Transport + Provider`: allowed `transports/`, `internal/runtime/transport/`, `providers/`, `internal/runtime/provider/`; depends on WS-1 and WS-3.
- `WS-5 Observability + Tooling + Tests`: allowed `internal/observability/`, `internal/tooling/`, `test/integration/`, `test/replay/`, `test/failover/`, `cmd/`; depends on WS-1..WS-4.
- `WS-6 Delivery Scaffolds`: allowed `pipelines/`, `deploy/`, `pkg/`; depends on WS-1 and WS-5 gate outputs.

Single-writer hotspots:
- `api/*` contracts: exactly one writer at a time (WS-1 only).
- `docs/RSPP_features_framework.json`: progress steward only.
- `docs/rspp_SystemDesign.md`: doc steward only.

## C) Contract PR Candidates

- `CPR-1` Turn-start authority + immutable plan-freeze contracts in `api/controlplane/` and `api/eventabi/`; unblocks WS-2 and WS-3; validate with `go test ./api/controlplane ./api/eventabi && make verify-quick`.
- `CPR-2` Cancellation/output-fencing semantics in `api/eventabi/` (and control-plane references in `api/controlplane/`); unblocks WS-3 and WS-4; validate with `go test ./api/eventabi ./internal/runtime/transport ./transports/livekit && make verify-quick`.
- `CPR-3` OR-02 replay evidence minimum schema in `api/observability/` and `api/eventabi/`; unblocks WS-5; validate with `go test ./api/observability ./internal/observability/replay ./test/replay && make verify-quick`.
- `CPR-4` MVP `simple/v1` profile compatibility contract in `api/controlplane/`, `api/eventabi/`, `cmd/`; unblocks WS-3 and WS-6; validate with `go test ./api/controlplane ./internal/runtime/planresolver ./cmd/rspp-cli && make verify-quick`.

## D) Current Repo Health Snapshot

- Command executed: `make verify-quick`
- Exit code: `0`
- Passing package lines captured: `19`
- Failing package lines captured: `0`

Failures and owners: no failures detected in latest quick gate run.

Evidence snippet:

    contract fixtures: total=28 failed=0
    ...
    ok   github.com/tiger/realtime-speech-pipeline/test/failover  (cached)
    VERIFY_QUICK_EXIT=0

## E) Assumptions + Open Questions

Assumptions:
- Primary feature-group-to-area assignment uses system-design module table with deterministic canonical-area precedence when multi-mapped.
- Backlog granularity is one item per MVP feature group, plus guide-alignment items for canonical areas without direct primary group assignment.
- Work package IDs from guides (`CP-*`, `CPAPI-*`, `LK-*`, `OR-02`, `NF-*`) are treated as executable anchors and may be split into multiple PRs by path boundary.
- `make verify-quick` is the required baseline health snapshot for this backlog generation run.

Open questions (non-blocking):
- Should contract PRs (`CPR-1..CPR-4`) be mandated as a hard merge prerequisite before any runtime/provider/transport implementation PR?
- Should large runtime groups such as `PipelineSpec & Graph Execution` be subdivided into smaller PR slices before execution starts?
- Should scaffold streams (`pipelines/`, `deploy/`, `pkg/`) wait until `verify-full` is green?

## Traceability Appendix

- Sections/keywords extracted from `docs/rspp_SystemDesign.md`: repository guarantees, non-goals, target-module breakdown, canonical guide map, ownership and change-control rules.
- Fields extracted from `docs/RSPP_features_framework.json`: `id`, `group`, `description`, `steps`, `release_phase` (MVP filtered).
- Canonical guide recon coverage: 15/15 files from section 6.
- Backlog item count: `39`.
- MVP ID coverage: `189` / `189`.
