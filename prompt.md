You are an engineering agent working on the RSPP (Real-time Speech Pipeline Platform) repository.
Your goal is to implement the MVP phase in a way that aligns with the system design, module guides, and the feature inventory, while preventing semantic drift.

AUTHORITATIVE SPEC SOURCES (priority order):
1) docs/rspp_SystemDesign.md
   - repository-level architecture boundaries, module decomposition, MVP guarantees vs post-MVP non-goals
2) Canonical module guides (e.g., internal/runtime/runtime_kernel_guide.md and others)
   - module-level contracts, current status, work packages, and done criteria
3) docs/RSPP_features_framework.json
   - MVP feature list, acceptance steps, P0 priorities, quality gates

MVP NON-NEGOTIABLE INVARIANTS (must be enforced in code + tests):
- Turn Plan Freeze: at turn start, the system must freeze an immutable ResolvedTurnPlan; do not mutate it in-turn.
- Lane Priority: ControlLane > DataLane > TelemetryLane. Telemetry must never block control-path progress.
- Cancellation-first: barge-in/interrupt must propagate end-to-end; downstream generation and outputs must stop; output fencing is mandatory.
- Authority/Epoch: lease/epoch must be enforced at ingress and egress. Outputs from stale authority must be rejected.
- Replay Evidence (OR-02): every accepted turn must produce replay-critical evidence (at least L0) without blocking the control path.
- MVP Execution Profile: only simple/v1 is allowed. Any non-simple profile must be rejected deterministically in MVP mode.
- Runtime compute must remain stateless-by-design; durable session state must be externalized as the design specifies.

WORK STYLE REQUIREMENTS:
- Read the relevant design docs and guides before modifying code. Do not “code by intuition.”
- Each PR should implement one work package or a tightly coupled set of feature IDs; avoid sweeping refactors.
- Every behavior must be testable. Add unit/contract/integration tests. Run repo gates (verify-quick / verify-full or equivalents).
- Any change touching api/ contracts (Event ABI / ControlPlane API / Observability API) must be clearly isolated and include compatibility strategy + tests.
- Your output must include:
  1) Which docs you read (paths + sections/keywords)
  2) What files you changed
  3) Which feature IDs / work packages you satisfied
  4) Which test commands you ran and results
  5) Remaining risks + follow-up suggestions

WHEN UNCERTAIN:
- Search the repo first (guides, interface definitions, feature IDs, existing tests).
- If the design is unclear, implement the smallest MVP-safe version and explicitly document assumptions + risks + TODOs in the PR description.

Task: Build an executable MVP backlog for the entire RSPP repo using the canonical-guide map.
Inputs (authoritative):
- docs/rspp_SystemDesign.md
- docs/RSPP_features_framework.json
- Canonical guides (from the system design table):
  - cmd/command_entrypoints_guide.md
  - internal/controlplane/controlplane_module_guide.md
  - internal/runtime/runtime_kernel_guide.md
  - internal/observability/observability_module_guide.md
  - internal/tooling/tooling_and_gates_guide.md
  - internal/shared/internal_shared_guide.md
  - providers/provider_adapter_guide.md
  - transports/transport_adapter_guide.md
  - test/test_suite_guide.md
  - pipelines/pipeline_scaffolds_guide.md
  - deploy/deployment_scaffolds_guide.md
  - pkg/public_package_scaffold_guide.md
  - api/controlplane/controlplane_api_guide.md
  - api/eventabi/eventabi_api_guide.md
  - api/observability/observability_api_guide.md

Phase A — Build the “Spec Map” (no code changes):
1) From docs/rspp_SystemDesign.md:
   - Extract repo-level guarantees and non-goals.
   - Extract the canonical guide table (area -> file path).
   - Extract governance/ownership rules (single-owner per module, api/ contract review requirements, CI gates).
2) From docs/RSPP_features_framework.json:
   - Filter release_phase == "mvp".
   - Aggregate counts by group.
   - Identify P0/high-risk items (e.g., quality gates, cancellation, authority/epoch, replay evidence).
   - Produce a mapping: feature group -> canonical area(s) -> code path prefixes.
   - If group->area mapping is ambiguous, write your best deterministic mapping and flag it as an assumption.

Phase B — Canonical Guide Recon (structured extraction):
For EACH canonical guide:
1) Read the guide and extract:
   - Module responsibilities (what it MUST do; what it MUST NOT do).
   - MVP invariants and enforcement points.
   - Current status: implemented/partial/scaffold (with evidence: file paths).
   - Work packages / done criteria (if listed).
   - Contract surfaces touched:
     - api/* schema/contracts
     - pkg/* exported packages
     - internal/* interfaces used by other modules
   - Required tests/gates and where they belong (unit/contract/integration/conformance).
2) Scan the corresponding code directories and confirm:
   - which parts exist (real implementation vs stubs)
   - current test failures (run verify-quick or go test ./... and record failures)

Phase C — Synthesize the MVP Backlog:
1) Convert all work packages + MVP feature acceptance steps into backlog items.
2) Each backlog item MUST include:
   - feature IDs (or WP IDs)
   - target module path prefix(es)
   - dependency edges (e.g., requires api/eventabi contract PR first)
   - “PR boundary” suggestion (keep PRs tight)
   - done criteria that are testable (tests to add + commands)
   - risk flags (e.g., tail latency, determinism, contract compatibility)
3) Output a proposed parallel workstream plan:
   - Workstreams should follow module path boundaries to avoid conflicts.
   - Identify “single-writer hotspots”:
     - api/* contracts (only 1 agent at a time)
     - docs/RSPP_features_framework.json (only progress steward)
     - docs/rspp_SystemDesign.md (only doc steward)

Deliverables (your final answer must include):
A) MVP Backlog (grouped by canonical area) with item-level metadata above
B) Parallel Workstreams (>=3) + strict “allowed path prefixes” per stream
C) Contract PR candidates (if multiple streams depend on api/* changes)
D) Current repo health snapshot: test/gate failures list and their owners
E) Assumptions + open questions (do not block; make them explicit)

Constraints:
- Do not do sweeping refactors in this task.
- If you must make a tiny change to run tests, isolate it and explain why.