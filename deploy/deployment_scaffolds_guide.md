# Deployment Scaffolds Guide

## Scope and Evidence

This guide proposes the target deployment architecture for framework scaffolds under `deploy/`, using:
- `docs/PRD.md` (sections `1.1`, `2.1`, `3.2`, `4.1.6` to `4.1.9`, `4.2`, `4.3`, `5.1`, `5.2`, `5.3`)
- `docs/rspp_SystemDesign.md` (sections `3`, `4`, `5`, `6`)
- `docs/RSPP_features_framework.json` (feature IDs cited inline)

Current repo status:
- `deploy/k8s/kubernetes_deployment_guide.md`, `deploy/helm/helm_deployment_guide.md`, and `deploy/terraform/terraform_deployment_guide.md` are placeholder-only.
- `deploy/` contains only 4 markdown files and no deployable artifacts (`*.yaml`, `*.tf`, `Chart.yaml`, `values*.yaml`).
- Deployment scaffolds are not implemented yet; this document defines the target module architecture and implementation path.

## Current Progress and Divergence Snapshot (2026-02-16)

Codebase-verified deployment progress:
- Deployment-adjacent release handoff logic exists in `internal/tooling/release/release.go` with rollout config validation and deterministic release manifests (`F-093`, `F-100`, `NF-016`).
- CLI release command wiring exists in `cmd/rspp-cli/main.go` (`publish-release`) and can consume rollout config artifacts (`F-126`, `F-127`).
- Gate execution chains exist in `Makefile` (`verify-quick`, `verify-full`, `verify-mvp`) and `scripts/verify.sh` (`NF-024` to `NF-029`, `NF-038`).
- Control-plane rollout and lease baseline services exist in `internal/controlplane/rollout/rollout.go` and `internal/controlplane/lease/lease.go` (`F-093`, `F-095`, `F-156`).

Codebase-verified deployment divergence:
- No Kubernetes, Helm, or Terraform implementation artifacts exist under `deploy/` despite the target architecture requiring those delivery surfaces (`F-103`, `F-104`, `F-108`, `F-125`, `NF-007`).
- No deployment profile artifacts exist for MVP/post-MVP boundary enforcement (`F-078`, `F-083`, `F-095`, `F-110` to `F-113`).
- No deployment-local verification harness exists in `deploy/verification` even though promotion is gate-driven (`F-127`, `NF-024` to `NF-029`, `NF-038`).

## Target Scaffold Layout

```text
deploy/
  deployment_scaffolds_guide.md
  profiles/
    mvp-single-region/
    postmvp-multiregion/
  k8s/
    base/
    runtime/
    controlplane/
    routing/
    scaling/
    security/
    observability/
    extensions/
    overlays/
  helm/
    charts/
      rspp-platform/
      rspp-runtime/
    values/
  terraform/
    modules/
      network/
      cluster/
      state/
      security/
      observability/
      release/
    envs/
      dev/
      stage/
      prod/
  verification/
    gates/
    smoke/
```

PRD-aligned phase note:
- MVP baseline deploy profile: LiveKit transport path, `simple/v1`, single-region lease authority (`F-078`, `F-083`, `F-095`, PRD `5.2`).
- Post-MVP expansion: active-active multi-region, skew governance, fairness/residency/supply-chain overlays (`F-110` to `F-113`, `F-162`, `F-172` to `F-174`, `F-168` to `F-171`, `NF-036`, `NF-037`).

Layout rationale (inline feature anchors):
- `k8s/runtime` + `k8s/scaling` are first-class because stateless runtime scaling and overload controls are mandatory (`F-103` to `F-109`, `F-049`, `F-098`, `NF-004`, `NF-006`).
- `k8s/routing` is split explicitly because rollout/lease/failover safety are independent control concerns (`F-093`, `F-095`, `F-096`, `F-100`, `NF-027`).
- `verification/gates` exists as a separate deployment module because promotion is gated by the seven MVP checks (`NF-024` to `NF-029`, `NF-038`).

## 1) Module List With Responsibilities

| Module | Path | Responsibilities | Feature trace |
| --- | --- | --- | --- |
| Deployment profiles | `deploy/profiles` | Deployment posture manifests that pin runtime topology and operational mode boundaries (`mvp-single-region`, `postmvp-multiregion`). | `F-078`, `F-079`, `F-110` to `F-113`, `NF-007` |
| Kubernetes base primitives | `deploy/k8s/base` | Namespace, service account, config, service, and policy baseline shared by runtime/control-plane workloads. | `F-103`, `F-108`, `F-114`, `F-120`, `NF-007` |
| Runtime workload scaffolds | `deploy/k8s/runtime` | Stateless runtime deployment templates with drain behavior, probes, resource and concurrency controls. | `F-103` to `F-109`, `NF-016`, `NF-004`, `NF-005`, `NF-006` |
| Control-plane workload scaffolds | `deploy/k8s/controlplane` | Registry/routing/admission/lease service deployments and API ingress surfaces. | `F-092` to `F-102`, `F-155` to `F-159` |
| Routing, rollout, and authority | `deploy/k8s/routing` | Canary/percentage/tenant rollout templates, rollback targets, lease-epoch safety hooks, migration policy switches. | `F-093`, `F-095`, `F-096`, `F-100`, `F-110` to `F-113`, `NF-027` |
| Scaling and admission | `deploy/k8s/scaling` | Autoscaling and admission templates using queue depth/latency/capacity signals; overload and fairness controls. | `F-049`, `F-050`, `F-098`, `F-104`, `F-109`, `F-172` to `F-174`, `NF-033` |
| Security and tenancy | `deploy/k8s/security` | Tenant isolation boundaries, RBAC, mTLS mode, token verification, retention/redaction policy wiring. | `F-114` to `F-120`, `F-117`, `F-119`, `NF-011`, `NF-012`, `NF-014` |
| Observability and replay | `deploy/k8s/observability` | Metrics/traces/logs/replay pipeline, OR-02 evidence retention classes, per-tenant replay access controls. | `F-067` to `F-077`, `F-149` to `F-154`, `F-176`, `NF-028` |
| Extensions and external execution | `deploy/k8s/extensions` | Custom node runtime classes, external node boundary adapters, resource/time limits, trust posture policy slots. | `F-011`, `F-121` to `F-124`, `F-160`, `F-164`, `F-171`, `NF-036` |
| Helm packaging | `deploy/helm/charts/*` | Reusable package layer for runtime/control-plane charts, values composition, and profile-driven installs. | `F-125`, `F-126`, `F-129`, `NF-007` |
| Terraform environment scaffolds | `deploy/terraform/modules/*`, `deploy/terraform/envs/*` | Cloud/cluster/state backend/KMS/network/bootstrap modules and environment composition. | `F-120`, `F-168` to `F-170`, `NF-031`, `NF-035` |
| Verification and gate scaffolds | `deploy/verification` | Deploy-time gate assertions and smoke flows for latency/cancel/authority/replay/lifecycle. | `F-127`, `NF-024` to `NF-029`, `NF-038`, `NF-030`, `NF-031` |

### 1.1 Current Progress and Divergence Matrix (Implementation-Focused)

| Target area | Current codebase state | Divergence from target | First implementation artifact (minimum) | Feature trace |
| --- | --- | --- | --- | --- |
| `deploy/profiles` | Directory missing | No deploy profile artifacts for MVP or post-MVP modes | `deploy/profiles/mvp-single-region/rollout.json` and `deploy/profiles/postmvp-multiregion/README.md` | `F-078`, `F-083`, `F-095`, `F-110` |
| `deploy/k8s/base` | Directory missing | No namespace/service-account/RBAC/network baseline | `deploy/k8s/base/namespace.yaml`, `serviceaccount.yaml`, `rbac.yaml` | `F-103`, `F-114`, `F-115`, `F-120` |
| `deploy/k8s/runtime` | Only placeholder guide exists | No runtime Deployment/Service/Probe/Drain/HPA/PDB manifests | `deploy/k8s/runtime/deployment.yaml`, `service.yaml`, `hpa.yaml`, `pdb.yaml` | `F-103` to `F-109`, `NF-016`, `NF-005` |
| `deploy/k8s/controlplane` | Directory missing | No control-plane workload scaffold | `deploy/k8s/controlplane/deployment.yaml`, `service.yaml`, `configmap.yaml` | `F-092` to `F-102`, `F-159` |
| `deploy/k8s/routing` | Directory missing | No rollout/authority policy scaffold despite lease+rollout logic in code | `deploy/k8s/routing/rollout-policy.yaml`, `lease-policy.yaml` | `F-093`, `F-095`, `F-100`, `F-156` |
| `deploy/k8s/security` | Directory missing | No tenancy/security policy manifests in deployment layer | `deploy/k8s/security/networkpolicy.yaml`, `tenant-rbac.yaml`, `mtls-mode.md` | `F-114` to `F-120`, `NF-011`, `NF-012` |
| `deploy/k8s/observability` | Directory missing | No deployment wiring for metrics/traces/replay retention | `deploy/k8s/observability/otel-config.yaml`, `replay-retention-policy.yaml` | `F-067` to `F-073`, `F-153`, `NF-028` |
| `deploy/k8s/extensions` | Directory missing | No runtime class scaffolds for custom/external nodes | `deploy/k8s/extensions/node-runtime-class.yaml`, `external-node-policy.yaml` | `F-121` to `F-124`, `F-171` |
| `deploy/helm/charts` | Directory missing | No chart packaging of deployment modules | `deploy/helm/charts/rspp-platform/Chart.yaml`, `values.yaml` | `F-125`, `F-126`, `F-129` |
| `deploy/terraform/modules` | Directory missing | No infra provision scaffolds | `deploy/terraform/modules/cluster/main.tf`, `variables.tf`, `outputs.tf` | `NF-007`, `NF-031`, `F-120` |
| `deploy/verification` | Directory missing | No deployment-local gate runner or smoke scripts | `deploy/verification/gates/mvp_gate_runner.sh`, `deploy/verification/smoke/README.md` | `F-127`, `NF-024` to `NF-029`, `NF-038` |
| Release handoff integration | Implemented in tooling/CLI only | Deploy folder does not provide rollout config artifacts consumed by `publish-release` | `deploy/profiles/mvp-single-region/rollout.json` wired to `go run ./cmd/rspp-cli publish-release ...` | `F-093`, `F-100`, `F-126` |

## 2) Key Interfaces (Pseudocode)

Interface intent mapping:
- `DeploymentScaffoldCompiler` defines the artifact-to-render contract for K8s/Helm/Terraform deploy outputs (`F-125`, `F-126`, `F-129`, `NF-007`).
- `RolloutController` captures canary/percentage progression, status, and rollback behavior as first-class operations (`F-093`, `F-100`, `F-165`, `NF-030`).
- `AuthoritySafetyHooks` exists to enforce and verify lease-epoch correctness in rendered runtime deployments (`F-095`, `F-156`, `NF-027`).
- `ExtensionRuntimeRegistry` is the deployment control point for custom/external node runtime classes (`F-121`, `F-122`, `F-123`, `F-124`, `F-171`).
- `GateValidator` binds rollout promotion to MVP gate evidence (`F-127`, `NF-024` to `NF-029`, `NF-038`).

```go
type DeploymentScaffoldCompiler interface {
    ValidateBundle(bundle DeploymentBundle) ([]ValidationIssue, error)
    RenderK8s(bundle DeploymentBundle, profile DeploymentProfile) ([]Manifest, error)
    RenderHelm(bundle DeploymentBundle, profile DeploymentProfile) (ChartPackage, error)
    RenderTerraform(bundle DeploymentBundle, profile DeploymentProfile) (TerraformPlanInput, error)
}

type RolloutController interface {
    Start(plan RolloutPlan) (RolloutID, error)
    Status(id RolloutID) (RolloutStatus, error)
    Rollback(id RolloutID, target BundleRef) error
}

type AuthoritySafetyHooks interface {
    InjectLeaseEpochChecks(in []Manifest, policy LeasePolicy) ([]Manifest, error)
    ValidateStaleEpochRejection(observed RuntimeEvidence) error
}

type ExtensionRuntimeRegistry interface {
    RegisterNodeRuntimeClass(class NodeRuntimeClass) error
    RegisterExtensionArtifact(artifact ExtensionArtifact) error
    ResolveExecutionTarget(node NodeRef, tenant TenantID) (ExecutionTarget, error)
}

type GateValidator interface {
    ValidateMVPGates(evidence GateEvidenceSet) ([]GateResult, error) // NF-024..NF-029, NF-038
}
```

## 3) Data Model: Core Entities and Relations

| Entity | Core fields | Relations |
| --- | --- | --- |
| `DeploymentBundle` | `bundle_id`, `pipeline_bundle_ref`, `runtime_image_digests`, `chart_version`, `terraform_module_refs`, `compat_window` | Pinned by `RolloutPlan`; rendered into K8s/Helm/Terraform outputs (`F-015`, `F-092`, `F-100`, `NF-010`). |
| `DeploymentProfile` | `profile_id`, `region_mode`, `transport_mode`, `execution_profile_ref`, `security_mode` | Governs compilation defaults and enabled modules (`F-078`, PRD `5.2`). |
| `RuntimeWorkloadTemplate` | `workload_id`, `replica_policy`, `probe_policy`, `drain_policy`, `resource_limits`, `concurrency_limits` | Produced from bundle+profile; consumed by autoscaling and gate checks (`F-103` to `F-109`, `NF-016`). |
| `ControlPlaneTemplate` | `services`, `registry_config`, `routing_snapshot_source`, `admission_policy_ref` | Emits turn-start decisions consumed by runtime deployments (`F-092` to `F-099`, `F-159`). |
| `LeasePolicy` | `lease_ttl`, `epoch_source`, `rotation_strategy`, `rejection_policy` | Attached to runtime ingress/egress filters and migration hooks (`F-095`, `F-156`, `NF-027`). |
| `RolloutPlan` | `rollout_id`, `bundle_ref`, `strategy`, `cohorts`, `rollback_ref`, `guard_conditions` | Drives canary/percentage/tenant rollout and rollback behavior (`F-093`, `F-100`, `F-165`). |
| `AutoscalingPolicy` | `policy_id`, `metrics`, `targets`, `min_replicas`, `max_replicas`, `stabilization` | Binds runtime/control-plane templates to capacity controls (`F-104`, `F-172`, `F-173`, `NF-005`). |
| `TenantSecurityPolicy` | `tenant_id`, `quota_policy_ref`, `rbac_bindings`, `retention_class`, `mtls_mode` | Enforced in workload/network/pipeline-level scaffolds (`F-114` to `F-120`, `F-050`). |
| `ExtensionArtifact` | `extension_id`, `kind`, `digest`, `trust_level`, `resource_policy`, `runtime_class` | Bound to custom node or external node execution path (`F-121` to `F-124`, `F-171`). |
| `NodeRuntimeClass` | `class_id`, `execution_mode`, `isolation_boundary`, `timeout_policy`, `observability_policy` | Referenced by extension artifacts and runtime templates (`F-122`, `F-123`, `F-160`). |
| `ReplayEvidencePolicy` | `recording_level`, `required_or02_fields`, `redaction_policy`, `retention_policy` | Feeds observability deployment and gate validation (`F-073`, `F-153`, `F-176`, `NF-028`). |
| `GateEvidenceSet` | `rollout_id`, `metrics`, `authority_events`, `replay_completeness`, `lifecycle_events` | Output of deployment verification; blocks promotion on gate failure (`NF-024` to `NF-029`, `NF-038`). |

Core relations:
- `DeploymentBundle` + `DeploymentProfile` -> rendered deployment artifacts (`k8s`, `helm`, `terraform`) (`F-125`, `F-126`).
- `RolloutPlan` points to exactly one active `DeploymentBundle`; rollback points to an immutable prior bundle (`F-093`, `F-100`).
- `LeasePolicy` applies to runtime ingress and egress enforcement and must emit auditable authority outcomes (`F-095`, `F-156`).
- `ExtensionArtifact` requires a `NodeRuntimeClass`; trust and limits are enforced before activation (`F-123`, `F-124`, `F-171`).
- `GateEvidenceSet` is generated per rollout stage and must satisfy MVP gates before promotion (`NF-024` to `NF-029`, `NF-038`).

## 4) Extension Points: Plugins, Custom Nodes, Custom Runtimes

Plugin extension points:
- `DeploymentPlugin`: adds profile overlays or manifest transforms without bypassing contract validation (`F-126`, `F-129`, `NF-010`).
- `RolloutStrategyPlugin`: adds new rollout strategies beyond canary/percentage while preserving rollback and gate semantics (`F-093`, `F-100`, `NF-030`).
- `ObservabilityExporterPlugin`: custom telemetry/replay sinks with OR-02 compatibility guarantees (`F-070`, `F-071`, `F-153`, `NF-028`).

Custom node deployment path:
- `InProcessNode` path: packaged with runtime image for low-latency trusted execution (`F-011`, `F-121`).
- `ExternalNode` path: out-of-process execution with explicit limits/timeouts/cancel propagation and required observability injection (`F-122`, `F-123`, PRD `4.1.7`).
- `SandboxNode` path (post-MVP): stronger isolation with trust attestations and SBOM checks (`F-124`, `F-171`, `NF-036`).

Custom runtime deployment path:
- `RuntimeClass` contract allows alternate runtime builds only if they preserve lane priority, lifecycle ordering, lease-epoch fencing, and replay evidence obligations (`F-139`, `F-142`, `F-149`, `F-156`, `F-176`, `NF-029`).
- Runtime variants must be capability-labeled and skew-checked against control-plane/schema expectations (`F-162`, `NF-032`).

## 5) Design Invariants (Implementation MUST Preserve)

1. Runtime workloads remain stateless for correctness; only declared durable state is externalized and reattached. (`F-103`, `F-155`, PRD `4.1.8`)
2. Lease epoch authority checks are enforced at ingress and egress; stale outputs/events are rejected. (`F-095`, `F-156`, `NF-027`)
3. Rollouts always target immutable bundle refs; mutable `latest` deployment targets are forbidden. (`F-015`, `F-093`, `F-100`)
4. Graceful shutdown must drain active sessions per policy before pod termination. (`F-106`, `NF-016`)
5. Runtime pods must expose readiness and liveness probes compatible with K8s scheduling behavior. (`F-108`)
6. Autoscaling and admission are driven by latency/queue/capacity signals, not CPU-only heuristics. (`F-049`, `F-098`, `F-104`, `F-172`)
7. Control-plane decision snapshots are versioned and runtime-local for hot-path enforcement. (`F-159`, PRD `4.1.8`)
8. Security boundaries enforce tenant isolation, RBAC, token integrity, and secret redaction by default. (`F-114`, `F-115`, `F-117`, `F-119`, `NF-011`, `NF-012`)
9. Optional mTLS mode must be deployment-configurable without changing runtime semantics. (`F-120`)
10. Observability and replay pipelines must preserve correlation IDs end-to-end. (`F-067`, `F-070`, `NF-008`)
11. OR-02 replay-critical evidence completeness is mandatory for accepted turns regardless of recording level. (`F-153`, `F-176`, `NF-028`)
12. Telemetry processing must never block ControlLane progression. (PRD `4.2`, `F-147`)
13. MVP deployment profile must enforce LiveKit transport + `simple/v1` + single-region authority defaults. (`F-078`, `F-083`, `F-095`, PRD `5.2`)
14. Post-MVP multi-region overlays must preserve single-authority execution per session. (`F-110`, `F-111`, `F-113`)
15. External/custom node execution requires declared resource/time limits and trust metadata before scheduling. (`F-123`, `F-171`)
16. Deployment promotion requires passing all seven MVP gates (latency, cancel fence, authority, replay, lifecycle, E2E). (`NF-024` to `NF-029`, `NF-038`)

## 6) Tradeoffs: Major Decisions and Alternatives

| Decision area | Chosen direction | Alternative | Tradeoff summary | Feature trace |
| --- | --- | --- | --- | --- |
| Orchestrator target | Kubernetes-first deployment scaffolds | Multi-orchestrator parity from day one | K8s-first aligns with explicit portability target and existing runtime semantics; multi-orchestrator parity increases early complexity and dilutes gating focus. | `F-103`, `F-108`, `NF-007` |
| Packaging model | Helm for app packaging + Terraform for environment provisioning | Helm-only or Terraform-only | Split model keeps app lifecycle and infra lifecycle independently versioned; single-tool strategy is simpler but less modular for multi-env governance. | `F-125`, `F-126`, `F-129`, `NF-031` |
| Runtime state posture | Stateless runtime pods + external durable state | Stateful runtime pods with local durable session data | Stateless runtime improves scaling/failover and disposable compute; stateful pods reduce external dependencies but conflict with migration/failover semantics. | `F-103`, `F-155`, `F-157`, PRD `4.1.8` |
| Authority topology | Single-region lease authority for MVP | Active-active multi-region in MVP | Single-region MVP lowers split-brain and rollout risk; active-active improves regional resilience but requires more coordination and failure-mode complexity early. | `F-095`, `F-156`, `F-110` to `F-113`, PRD `5.2` |
| Rollout strategy | Immutable bundle + explicit rollout/rollback manifests | Mutable in-place config updates | Immutable bundles improve auditability and safe rollback; mutable updates are faster operationally but weaken traceability and reproducibility. | `F-015`, `F-093`, `F-097`, `F-100` |
| Node execution model | Mixed mode (in-process trusted + external/sandbox untrusted) | External-only node execution | Mixed mode preserves low-latency for trusted nodes and isolation for risky nodes; external-only simplifies trust model but adds overhead and operational cost. | `F-121` to `F-124`, `F-171`, `NF-036` |
| Replay baseline | Mandatory OR-02 baseline (L0) with deterministic downgrade policy | Always-on maximum fidelity recording | L0 baseline preserves deploy-time cost/perf bounds while meeting gate requirements; always-on high fidelity improves forensics but increases storage/privacy/tail-latency risk. | `F-153`, `F-154`, `F-176`, `NF-028` |

## 7) Implementation-Ready Rollout Plan

Execution order is strict so every phase leaves a runnable/validatable intermediate state.

Phase 0: scaffold bootstrap in `deploy/`
- Create missing target directories and add `README.md` ownership/contract notes for each module path in this guide.
- Definition of done: `find deploy -maxdepth 3 -type d` includes all target scaffold paths; placeholder-only state is removed.
- Feature anchors: `F-125`, `F-126`, `NF-007`.

Phase 1: MVP profile + release handoff wiring
- Add `deploy/profiles/mvp-single-region/rollout.json` compatible with `internal/tooling/release.ValidateRolloutConfig`.
- Add `deploy/profiles/mvp-single-region/profile.md` declaring LiveKit + `simple/v1` + single-region authority posture.
- Add `deploy/profiles/postmvp-multiregion/profile.md` as deferred contract boundary.
- Definition of done: `go run ./cmd/rspp-cli publish-release <spec_ref> deploy/profiles/mvp-single-region/rollout.json` succeeds after gate artifacts exist.
- Feature anchors: `F-078`, `F-083`, `F-093`, `F-095`, `F-100`, `NF-016`.

Phase 2: Kubernetes baseline manifests
- Implement `deploy/k8s/base`, `deploy/k8s/runtime`, `deploy/k8s/controlplane`, `deploy/k8s/security`, `deploy/k8s/observability` minimum manifests.
- Runtime manifest minimums: deployment, service, readiness/liveness probes, graceful termination/drain configuration, resource limits, and HPA hooks.
- Definition of done: `kubectl apply --dry-run=client -f deploy/k8s/base -f deploy/k8s/runtime -f deploy/k8s/controlplane -f deploy/k8s/security -f deploy/k8s/observability` passes.
- Feature anchors: `F-103` to `F-109`, `F-092` to `F-099`, `F-114` to `F-120`, `F-067` to `F-071`, `NF-005`, `NF-016`.

Phase 3: routing/authority and extension scaffolds
- Add `deploy/k8s/routing/rollout-policy.yaml` and `deploy/k8s/routing/lease-policy.yaml` to map deploy-time policy values into control-plane services.
- Add `deploy/k8s/extensions` runtime class and external-node policy scaffold docs/manifests.
- Definition of done: manifests validate with `kubectl --dry-run=client` and link to existing rollout/lease service fields in control-plane code.
- Feature anchors: `F-093`, `F-095`, `F-100`, `F-121` to `F-124`, `F-156`, `F-171`.

Phase 4: Helm packaging and Terraform modules
- Add `deploy/helm/charts/rspp-platform` and `deploy/helm/charts/rspp-runtime` with values files for MVP profile.
- Add `deploy/terraform/modules/*` and `deploy/terraform/envs/dev|stage|prod` skeletons with shared variables and outputs.
- Definition of done: `helm lint deploy/helm/charts/rspp-platform` and `terraform -chdir=deploy/terraform/envs/dev validate` pass.
- Feature anchors: `F-125`, `F-126`, `F-129`, `NF-007`, `NF-031`, `F-120`.

Phase 5: deploy-local verification harness
- Add `deploy/verification/gates/mvp_gate_runner.sh` that orchestrates artifact generation + gate checks + publish-release dry-run.
- Add `deploy/verification/smoke` scripts for deploy preflight checks and post-deploy smoke expectations.
- Definition of done: one command in `deploy/verification/gates` can reproduce gate evidence and release-manifest generation deterministically.
- Feature anchors: `F-127`, `NF-024` to `NF-029`, `NF-038`, `NF-030`.

## 8) Validation Commands for This Plan

Use these existing repository commands as the implementation acceptance baseline:
- `make verify-quick`
- `make verify-full`
- `make verify-mvp` (for changes touching MVP deployment posture/gate evidence expectations)
- `go run ./cmd/rspp-cli publish-release <spec_ref> <rollout_cfg_path>`

Minimum deterministic artifact sequence before `publish-release`:
- `go run ./cmd/rspp-cli validate-contracts-report`
- `go run ./cmd/rspp-cli replay-regression-report`
- `go run ./cmd/rspp-cli generate-runtime-baseline`
- `go run ./cmd/rspp-cli slo-gates-report`
- `go run ./cmd/rspp-cli publish-release <spec_ref> <rollout_cfg_path>`

Expected behavior for deployment-scaffold readiness:
- Gate commands pass without regressing authority/replay/lifecycle metrics (`NF-024` to `NF-029`, `NF-038`).
- `publish-release` emits `.codex/release/release-manifest.json` with valid rollout config and readiness inputs (`F-100`, `NF-016`).
