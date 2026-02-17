MODULE SNAPSHOT
- Area: Control Plane + API
- Canonical guide: `internal/controlplane/controlplane_module_guide.md`, `api/controlplane/controlplane_api_guide.md`
- Code path prefixes: `internal/controlplane`, `api/controlplane`
- Public contract surfaces (api/*, pkg/*): `api/controlplane/types.go` (`DecisionOutcome`, `TurnTransition`, `ResolvedTurnPlan` and nested validators); `api/eventabi` payload class contract consumed by control-plane retention snapshot loading (`internal/controlplane/distribution/retention_snapshot.go`); no implemented exported package contract surface under `pkg/*` (only guide file `pkg/contracts/contracts_package_guide.md`). Internal interfaces consumed/produced in this area: per-module `Backend` interfaces under `internal/controlplane/*`, grouped via `internal/controlplane/distribution.ServiceBackends`, consumed by runtime seam `internal/runtime/turnarbiter.TurnStartBundleResolver`.
- MVP responsibilities (bullets):
1. MUST deterministically resolve turn-start control-plane bundle inputs (pipeline/version/profile, graph ref/fingerprint, policy/admission/lease decisions, snapshot provenance).
2. MUST enforce MVP execution profile gate (`simple`) and reject unsupported profiles.
3. MUST classify stale snapshot conditions deterministically and propagate stale errors.
4. MUST normalize policy/admission/lease outputs to canonical enums/order/reasons.
5. MUST keep shared API contract validation strict and replay-safe (`DecisionOutcome`, `TurnTransition`, `ResolvedTurnPlan`).
6. MUST NOT mutate immutable versioned artifacts in place during resolution.
7. MUST NOT silently accept stale routing/authority contexts for turn-start.
- MVP invariants to enforce (bullets):
1. Accepted turns include complete `SnapshotProvenance`.
2. Pre-turn CP outcomes remain `admit|reject|defer` and do not emit accepted-turn terminal semantics.
3. Authority epoch/authorization checks reject stale or deauthorized authority deterministically.
4. Stale routing/snapshot resolution fails deterministically.
5. Adaptive actions are explicit, deduplicated, and canonicalized.
6. Distribution artifact schema/version is validated before service resolution.
7. Stale snapshot backend errors are propagated; non-stale backend failures follow explicit fallback policy.
- Current implementation status:
  - Implemented:
    - CP baseline service modules are implemented and unit tested: `internal/controlplane/registry/registry.go`, `internal/controlplane/normalizer/normalizer.go`, `internal/controlplane/rollout/rollout.go`, `internal/controlplane/graphcompiler/graphcompiler.go`, `internal/controlplane/routingview/routingview.go`, `internal/controlplane/providerhealth/providerhealth.go`, `internal/controlplane/policy/policy.go`, `internal/controlplane/lease/lease.go`, `internal/controlplane/admission/admission.go`.
    - Distribution adapters are implemented with deterministic error taxonomy/stale classification and retention support: `internal/controlplane/distribution/file_adapter.go`, `internal/controlplane/distribution/http_adapter.go`, `internal/controlplane/distribution/retention_snapshot.go`.
    - Runtime integration seam with fallback wrappers and stale propagation exists and is tested: `internal/runtime/turnarbiter/controlplane_bundle.go`, `internal/runtime/turnarbiter/controlplane_backends.go`, `internal/runtime/turnarbiter/controlplane_bundle_test.go`, `internal/runtime/turnarbiter/controlplane_backends_test.go`.
    - Shared API validators and strict fixture conformance harness exist: `api/controlplane/types.go`, `api/controlplane/types_test.go`, `test/contract/fixtures_test.go`.
    - Recheck validation commands passed: `go test ./...`; `make verify-quick`; `go test ./api/controlplane ./internal/controlplane/... ./test/contract ./test/integration -run 'Test(CP|CT|Contract|ResolveTurnStartBundle|NewHTTPBackends|LoadRetentionPolicySnapshotFromFile)'`.
  - Partial:
    - Rollout logic is deterministic baseline mapping (requested/registry), but no canary/percentage/allowlist policy engine: `internal/controlplane/rollout/rollout.go`.
    - Routing view returns snapshot refs only, not concrete session route endpoint/token payloads: `internal/controlplane/routingview/routingview.go`.
    - Lease output is epoch + booleans + reason; no signed lease token/expiry/renewal model: `internal/controlplane/lease/lease.go`.
    - Policy/provider-health modules are normalization and snapshot-pointer seams, not full in-process policy/health engines: `internal/controlplane/policy/policy.go`, `internal/controlplane/providerhealth/providerhealth.go`.
    - API contract coverage is present but narrow at package-local level (DecisionOutcome-focused unit tests + minimal fixtures): `api/controlplane/types_test.go`, `test/contract/fixtures/resolved_turn_plan/*`, `test/contract/fixtures/turn_transition/*`, `test/contract/fixtures/decision_outcome/*`.
  - Scaffold/Missing:
    - Control-plane binary remains scaffold-only: `cmd/rspp-control-plane/main.go`.
    - Control-plane owned resolver target package is missing; resolver still lives in runtime package: `internal/runtime/turnarbiter/controlplane_bundle.go` (no `internal/controlplane/resolution/*`).
    - First-class shared API route/lease/version/session status types are missing in `api/controlplane/types.go` (recheck: no matches for `SessionRoute`, `PlacementLease`, `LeaseTokenRef`, `SessionStatus`, `PipelineVersionRef`, `PolicyVersionRef`, `RolloutDecision`).
    - Control-plane security module path is scaffold-only: `internal/controlplane/security/.gitkeep`.
  (Each with evidence: file paths)
- MVP backlog candidates (items):
  For each item:
  - Title: Control-plane process bootstrap (artifact publish/resolve/rollback + audit)
  - Feature IDs / WP IDs: `F-092`, `F-097`, `F-100`, `F-101`, `F-102`; `CP-13`, `CPAPI-01`, `CPAPI-02`, `CPAPI-10`
  - Target paths: `cmd/rspp-control-plane/main.go`, `internal/controlplane/registry`, `api/controlplane`
  - Dependencies (contracts/other PRs): Contract-owner PR for version/audit API ref types; runtime/ops review
  - Implementation notes (minimal MVP approach): Add minimal HTTP control-plane service with immutable artifact store, publish/list/get/rollback endpoints, append-only audit entries
  - Tests to add (file/dir + scenario): `test/integration/controlplane_process_test.go` for publish/list/get metadata, rollback behavior, and audit immutability
  - Done criteria (verifiable): New endpoints return immutable version refs with metadata (`created_at`, `author`, hash), rollback changes new-session resolution, audit log is append-only

  - Title: Deterministic rollout policy semantics (canary/percentage/allowlist)
  - Feature IDs / WP IDs: `F-093`, `F-100`; `CP-04` (guide P1)
  - Target paths: `internal/controlplane/rollout/rollout.go`, `internal/controlplane/distribution/file_adapter.go`, `internal/controlplane/distribution/http_adapter.go`
  - Dependencies (contracts/other PRs): Stable session bucketing contract and rollout snapshot schema update
  - Implementation notes (minimal MVP approach): Extend rollout snapshot model with explicit policy rules, deterministic bucketing, allowlist override, rollback pointer
  - Tests to add (file/dir + scenario): `internal/controlplane/rollout/rollout_test.go` (deterministic bucket matrix, allowlist override, rollback), `test/integration/runtime_chain_test.go` multi-session distribution checks
  - Done criteria (verifiable): Under fixed snapshot, same session hashes to same version; rollback deterministically routes new sessions to prior version

  - Title: Session route metadata + token + status shared contract/API
  - Feature IDs / WP IDs: `F-094`, `F-099`, `F-101`; `CPAPI-03`, `CPAPI-04`
  - Target paths: `api/controlplane/types.go`, `internal/controlplane/sessionroute/*`, `cmd/rspp-control-plane/main.go`
  - Dependencies (contracts/other PRs): Contract-owner approval for new shared route/token types; transport adapter integration
  - Implementation notes (minimal MVP approach): Add transport-agnostic `SessionRoute`/token/status types and minimal broker endpoint returning endpoint + short-lived token + resolved version
  - Tests to add (file/dir + scenario): `api/controlplane/*_test.go`, `test/contract/fixtures/session_route/*`, `test/integration/*session_route*`
  - Done criteria (verifiable): Route API issues endpoint+token and adapters can connect/verify binding to intended pipeline version

  - Title: Lease token enrichment and strict authority safety
  - Feature IDs / WP IDs: `F-095`, `F-156`, `F-157`; `CP-08`, `CPAPI-04`
  - Target paths: `internal/controlplane/lease/lease.go`, `api/controlplane/types.go`, `internal/runtime/turnarbiter/controlplane_bundle.go`
  - Dependencies (contracts/other PRs): Contract-owner lease token type additions; migration signal contract alignment
  - Implementation notes (minimal MVP approach): Add lease token reference + expiry to authority output and enforce ingress/egress authority checks consistently
  - Tests to add (file/dir + scenario): `internal/controlplane/lease/lease_test.go` for stale/expired/deauthorized cases; `test/integration/runtime_chain_test.go` for stale-authority rejection and migration continuity
  - Done criteria (verifiable): Stale/expired authority attempts are rejected deterministically; migration handoff preserves `session_id` continuity markers

  - Title: Tenant-aware admission controls (quota/rate-limit)
  - Feature IDs / WP IDs: `F-098`; `CP-09`, `CPAPI-05`
  - Target paths: `internal/controlplane/admission/admission.go`, `internal/controlplane/policy/policy.go`, runtime turn-open inputs
  - Dependencies (contracts/other PRs): Tenant context propagation in turn-start request path
  - Implementation notes (minimal MVP approach): Extend admission inputs with tenant-scoped capacity policy snapshot and deterministic reject/defer behavior
  - Tests to add (file/dir + scenario): `internal/controlplane/admission/admission_test.go` tenant/session scope matrix; `test/integration/runtime_chain_test.go` rate-limit exceed behavior
  - Done criteria (verifiable): Over-limit session starts reject/defer deterministically with tenant-scoped reasons/observability

  - Title: Explicit backend failure mode (strict vs availability fallback)
  - Feature IDs / WP IDs: `F-159` (stale handling), `F-096` (failover posture); `CP-12` (guide P0)
  - Target paths: `internal/runtime/turnarbiter/controlplane_backends.go`, `internal/runtime/turnarbiter/controlplane_backends_test.go`
  - Dependencies (contracts/other PRs): Config plumbing for resolver mode
  - Implementation notes (minimal MVP approach): Add explicit mode flag to either fail on non-stale backend errors or keep current fallback behavior; keep stale propagation invariant
  - Tests to add (file/dir + scenario): Backends test matrix for stale/non-stale x strict/availability; integration assertion of mode effect on turn-open
  - Done criteria (verifiable): Mode switch changes only non-stale backend handling; stale errors always stop resolution

  - Title: API schema parity and validator completeness
  - Feature IDs / WP IDs: `F-141`, `F-149`, `F-150`, `F-151`, `F-153`, `F-043`, `F-044`, `F-081`; `CPAPI` P0/P1 from guide
  - Target paths: `api/controlplane/types.go`, `api/controlplane/types_test.go`, `docs/ContractArtifacts.schema.json`, `test/contract/fixtures/*`
  - Dependencies (contracts/other PRs): Contract-owner decision on schema/type parity deltas
  - Implementation notes (minimal MVP approach): Resolve `timestamp_ms` parity, enforce `sync_drop_policy.group_by` enum parity, align `streaming_handoff` between types/schema/fixtures, split validator tests into focused files
  - Tests to add (file/dir + scenario): `api/controlplane/turn_transition_test.go`, `api/controlplane/resolved_turn_plan_test.go`, `api/controlplane/flow_buffer_policy_test.go`, `api/controlplane/recording_determinism_test.go`; expanded valid/invalid fixtures
  - Done criteria (verifiable): Go validators, strict fixture decoding, and JSON schema accept/reject identical payloads

  - Title: Extensibility and boundary contracts baseline
  - Feature IDs / WP IDs: `F-121`, `F-122`, `F-123`, `F-124`; `CPAPI-12`
  - Target paths: `api/controlplane`, `internal/controlplane/security`
  - Dependencies (contracts/other PRs): Runtime/nodehost and security stream contract collaboration
  - Implementation notes (minimal MVP approach): Introduce typed plugin/external-node boundary descriptors without implementing full sandbox runtime in this step
  - Tests to add (file/dir + scenario): Contract fixtures for descriptor validation and integration stubs for timeout/cancellation/resource-limit propagation
  - Done criteria (verifiable): Typed extension descriptors validate strictly and are consumable by runtime without ad-hoc metadata

  - Title: State contract + idempotency/provider-invocation linkage
  - Feature IDs / WP IDs: `F-155`, `F-158`, `F-160`; state/authority stream
  - Target paths: `api/controlplane`, `internal/runtime/provider/invocation`, `test/failover`, `test/replay`
  - Dependencies (contracts/other PRs): Runtime idempotency lineage semantics and migration/failover policy alignment
  - Implementation notes (minimal MVP approach): Add explicit state class contract fields and provider invocation linkage fields for replay/dedupe correctness
  - Tests to add (file/dir + scenario): failover/replay cases for duplicate ingress, reconnect replay, failover replay, and normalized provider invocation outcomes
  - Done criteria (verifiable): Duplicate ingress is deduped per contract and invocation lineage remains replay-visible with normalized outcomes
- Risks & pitfalls (tail latency, determinism, race, compatibility):
1. HTTP fetch retry/failover can increase turn-open tail latency if timeout/backoff bounds drift.
2. Availability-mode fallback on non-stale backend errors can mask CP outages and introduce silent behavioral drift.
3. Epoch-only lease model increases split-authority risk without signed token + expiry checks.
4. Type/schema drift (`timestamp_ms`, `sync_drop_policy.group_by`, `streaming_handoff`) can break strict fixture compatibility and CI contracts.
5. Missing compatibility/skew contract enforcement can cause runtime-control-plane-schema rollout mismatch.
- Suggested “owner stream” (which workstream should implement it)
1. CP Core stream: rollout semantics, routing, lease, admission, distribution fallback-mode controls.
2. CP API Contracts stream: `api/controlplane` shared types, schema parity, fixture expansion.
3. CP Service Surface stream: `cmd/rspp-control-plane` endpoints and operational APIs.
4. Runtime Authority/State stream: lease enforcement, migration continuity, idempotency/provider-invocation linkage.
5. Security/Extensibility stream: plugin/external boundary contracts and `internal/controlplane/security` implementation.
