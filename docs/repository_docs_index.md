# Repository Documentation Index

## Documentation policy

`docs/` is the cross-repository layer only:
- architecture boundaries
- repository-wide CI/security governance
- migration and ownership governance
- shared contract schema artifacts

Detailed module implementation and module design are canonical in folder-level guide markdown files.

## Canonical implementation guides

| Area | Canonical file |
| --- | --- |
| Command entrypoints | `cmd/command_entrypoints_guide.md` |
| Control-plane internals | `internal/controlplane/controlplane_module_guide.md` |
| Runtime kernel internals | `internal/runtime/runtime_kernel_guide.md` |
| Observability and replay internals | `internal/observability/observability_module_guide.md` |
| Tooling and release gates internals | `internal/tooling/tooling_and_gates_guide.md` |
| Internal shared helpers | `internal/shared/internal_shared_guide.md` |
| Provider adapters | `providers/provider_adapter_guide.md` |
| Transport adapters | `transports/transport_adapter_guide.md` |
| Test and conformance suites | `test/test_suite_guide.md` |
| Pipeline artifact scaffolds | `pipelines/pipeline_scaffolds_guide.md` |
| Deployment scaffolds | `deploy/deployment_scaffolds_guide.md` |
| Public package contract scaffolds | `pkg/public_package_scaffold_guide.md` |
| Control-plane API contracts | `api/controlplane/controlplane_api_guide.md` |
| Event ABI API contracts | `api/eventabi/eventabi_api_guide.md` |
| Observability API contracts | `api/observability/observability_api_guide.md` |

## Ownership operating rules

1. One primary owner per module path.
2. Cross-domain changes require review from all impacted owners.
3. Contract changes under `api/` require CP and Runtime cross-review.
4. Security-sensitive paths require Security-Team review.

## Scaffold change-control

A scaffold/layout change should declare:
1. impacted contracts and module boundaries
2. explicit ownership remap
3. expected CI gate impact (`verify-quick`, `verify-full`, `security-baseline`)
4. preserved non-goal boundaries from system design

## Kept docs in this directory

| File | Scope |
| --- | --- |
| `docs/rspp_SystemDesign.md` | Repository-level architecture model and boundaries (includes current streaming/non-streaming execution progress snapshot) |
| `docs/CIValidationGates.md` | Repository-level gate policy and command map (includes implementation progress snapshot for live mode-proof/compare gating) |
| `docs/SecurityDataHandlingBaseline.md` | Repository-level security and data-handling policy |
| `docs/ContractArtifacts.schema.json` | Shared contract artifact schema |
| `docs/runbooks/runbooks_index.md` | Repository-level runbook index and scope rule |
| `docs/PRD.md` | Product requirements and MVP standards (includes execution progress snapshot for streaming vs non-streaming plan outcomes) |

## Progress snapshot pointers

Use these sections for the latest recorded progress on the streaming/non-streaming latency plan:
1. `docs/PRD.md` section `5.3 Execution progress snapshot (2026-02-13)`
2. `docs/rspp_SystemDesign.md` section `Implementation progress snapshot (2026-02-13)`
3. `docs/CIValidationGates.md` section `5.2 Implementation progress snapshot (2026-02-13)`
4. `internal/tooling/tooling_and_gates_guide.md` section `Latest implementation progress snapshot (2026-02-13)`

## Update workflow

1. Update folder guide files first for module behavior changes.
2. Update `docs/` only for repository-wide policy or boundary changes.
3. Keep this index and coverage matrix synchronized with canonical folder-level guide locations.
