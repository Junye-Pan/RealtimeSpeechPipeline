# Internal Observability Module Guide (Implementation-Ready)

This guide is both:
- the target architecture for observability, and
- the execution reference for what is implemented today vs what is still missing.

Validation timestamp: `2026-02-16` (code audit focused on `internal/observability/*`).

Evidence baseline:
- PRD: `2.1`, `4.1.3`, `4.1.5`, `4.1.6`, `4.2.2`, `4.2.3`, `4.3`, `4.4.2`, `4.4.3`, `4.4.4`, `5.1`, `5.2`.
- Feature catalog: `F-067` to `F-077`, `F-131`, `F-142`, `F-149`, `F-150`, `F-151`, `F-152`, `F-153`, `F-154`, `F-160`, `F-170`, `F-176`, `NF-002`, `NF-008`, `NF-009`, `NF-011`, `NF-014`, `NF-018`, `NF-024`, `NF-025`, `NF-026`, `NF-027`, `NF-028`, `NF-029`, `NF-038`.

Status legend:
- `Implemented`: code exists and is usable now.
- `Partial`: foundational code exists, but contract is incomplete.
- `Planned`: module is target-only and not implemented yet.

## 1) Current State and Divergence Matrix

| Module | Status | Code evidence | Current capability | Divergence to close |
| --- | --- | --- | --- | --- |
| `OBS-01 Correlation Context` | `Planned` | no dedicated package under `internal/observability/telemetry/context` | Telemetry accepts ad-hoc correlation metadata. | Add canonical correlation derivation/enforcement for tenant/session/turn/node/edge/event/epoch (`F-067`, `NF-008`). |
| `OBS-02 TelemetryLane Intake` | `Implemented` | `internal/observability/telemetry/pipeline.go`, `internal/observability/telemetry/env.go` | Bounded queue, sampling, deterministic drop/accounting, non-blocking intake posture. | Split intake concerns from processors/exporters; enforce canonical correlation contract (`F-069`, `F-142`, `NF-002`). |
| `OBS-03 Telemetry Processors/Exporters` | `Partial` | `internal/observability/telemetry/otlp_http.go`, `internal/observability/telemetry/memory_sink.go` | Sink abstraction with OTLP HTTP + memory sink. | Missing separate metrics/tracing/logging processing modules and richer backend isolation (`F-068`, `F-070`). |
| `OBS-04 OR-02 Local Timeline Append` | `Implemented` | `internal/observability/timeline/recorder.go` | Stage-A recorder, bounded capacities, baseline/detail tracking, deterministic downgrade signaling. | Formalize append API boundary to runtime callers and make OR-02 manifest versioning explicit (`F-153`, `NF-028`). |
| `OBS-05 OR-02 Evidence Builder` | `Partial` | `internal/observability/timeline/recorder.go`, `internal/observability/timeline/artifact.go` | Baseline evidence structures, completeness helpers, artifact serialization. | Missing dedicated evidence builder module that guarantees canonical gate-anchor population end-to-end (`NF-024`, `NF-025`, `NF-026`, `NF-038`). |
| `OBS-06 Timeline Durable Export` | `Planned` | no `internal/observability/timeline/exporter` package | No async durable export/retry pipeline. | Implement local-append vs durable-export separation required by PRD `4.1.6` (`F-154`, `F-176`). |
| `OBS-07 Replay Engine/Cursor` | `Planned` | no `internal/observability/replay/engine` or cursor package | Replay execution engine is absent. | Add replay modes and deterministic cursor execution (`F-072`, `F-151`, `F-152`, `NF-009`). |
| `OBS-08 Divergence and Reporting` | `Partial` | `internal/observability/replay/comparator.go` | Divergence classes/helpers exist. | Missing report builder and CI/operator artifact publishing (`F-077`, `NF-009`). |
| `OBS-09 Access Control/Redaction` | `Implemented` | `internal/observability/replay/access.go`, `internal/observability/timeline/redaction.go` | Deny-by-default replay access + redaction policy wiring. | Expand policy hooks and complete audit linkage for all denial/redaction paths (`F-073`, `NF-011`, `NF-014`). |
| `OBS-10 Audit/Retention/Deletion` | `Implemented` | `internal/observability/replay/service.go`, `internal/observability/replay/audit_backend*.go`, `internal/observability/replay/retention*.go` | Immutable append flow, backend fallback, retention policies, deletion modes. | Harden toward tamper-evident post-MVP path and unify operational reporting (`F-116`, `F-170`). |
| `OBS-11 Extension Host/Conformance` | `Planned` | no `internal/observability/extensions` package | No plugin surface for exporter/comparator/redactor extensions. | Add extension points + conformance tests for custom integrations (`F-164`). |

## 2) Implemented Surfaces (Now)

### Telemetry

Implemented surfaces:
- Non-blocking, bounded telemetry queue with drop accounting and sampling controls:
  - `internal/observability/telemetry/pipeline.go`
  - `internal/observability/telemetry/env.go`
- Export backends:
  - OTLP HTTP sink: `internal/observability/telemetry/otlp_http.go`
  - Memory sink: `internal/observability/telemetry/memory_sink.go`
- Default emitter wiring:
  - global emitter override pattern in `internal/observability/telemetry` package

Contract posture:
- Strong on `F-069`, `F-142`, `NF-002` foundations.
- Partial for end-to-end correlation completeness (`NF-008`) because canonical correlation contract is not yet centralized.

### Timeline (OR-02)

Implemented surfaces:
- Stage-A evidence recorder with bounded capacities, baseline/detail evidence, and downgrade control signal emission:
  - `internal/observability/timeline/recorder.go`
- Artifact serialization:
  - `internal/observability/timeline/artifact.go`
- Redaction policy resolution:
  - `internal/observability/timeline/redaction.go`

Contract posture:
- Strong on baseline data-model scaffolding for `F-149`, `F-150`, `F-153`.
- Partial for PRD `4.1.6` export split (`F-154`) because durable export pipeline is still missing.

### Replay/Audit/Retention

Implemented surfaces:
- Replay authorization service with deny-by-default behavior:
  - `internal/observability/replay/access.go`
- Audit logging service + backend fallback:
  - `internal/observability/replay/service.go`
  - `internal/observability/replay/audit_backend*.go`
- Retention/deletion model and sweeper:
  - `internal/observability/replay/retention*.go`
- Divergence helper primitives:
  - `internal/observability/replay/comparator.go`

Contract posture:
- Strong on access control + retention/audit foundations.
- Partial on replay execution (`F-072`, `F-151`, `F-152`) and report artifacts (`F-077`).

## 3) Key Interfaces (Implementation and Planned Additions)

Current implementation patterns exist in package code, but the following interfaces are the required implementation contracts to close known gaps.

```go
package observability

// Missing central correlation contract (OBS-01).
type CorrelationContextResolver interface {
    ResolveFromEvent(e EventEnvelope) (CorrelationContext, error)
    ValidateCanonical(c CorrelationContext) error
}

// Implemented conceptually via telemetry pipeline; keep non-blocking guarantee.
type TelemetryIngress interface {
    EmitMetric(c CorrelationContext, m MetricSample) EmitOutcome
    EmitLog(c CorrelationContext, l LogRecord) EmitOutcome
    EmitSpan(c CorrelationContext, s SpanRecord) EmitOutcome
}

// Implemented locally in timeline recorder; split durable export is missing.
type TimelineAppender interface {
    Append(e EventEnvelope) (LocalAppendAck, error)
    AppendEvidence(delta OR02EvidenceDelta) (LocalAppendAck, error)
}

// Missing (OBS-06).
type TimelineDurableExporter interface {
    Export(batch []TimelineRecord) (ExportResult, error)
}

// Missing (OBS-07).
type ReplayEngine interface {
    Run(req ReplayRequest) (ReplayResult, error)
}

// Partially implemented in comparator helpers; report artifact layer missing.
type DivergenceReporter interface {
    Compare(baseline ReplayResult, candidate ReplayResult) ([]Divergence, error)
    BuildReport(findings []Divergence) ([]byte, error)
}
```

## 4) Data Model: Core Entities and Relations

Core entities currently present in code and/or required for completion:
- `BaselineEvidence`, `ProviderAttemptEvidence`, `HandoffEdgeEvidence`, `InvocationSnapshotEvidence` (implemented in `internal/observability/timeline/recorder.go`).
- `ReplayAccessRequest`/decision types (implemented in `internal/observability/replay/access.go`).
- Audit and retention entities (implemented in `internal/observability/replay/service.go` and `internal/observability/replay/retention*.go`).
- Divergence records (implemented in `internal/observability/replay/comparator.go`).

Required relation guarantees:
- One accepted turn must map to one complete OR-02 baseline evidence record (`NF-028`).
- Replay access decisions must be auditable and tenant-scoped (`NF-011`, `NF-014` posture).
- Timeline local records must be exportable later without changing correlation identity.

## 5) Prioritized Implementation Backlog (To Close Divergence)

### P0 - Correlation Contract Completion (`OBS-01`)

Deliverables:
- Add `internal/observability/telemetry/context` package.
- Implement canonical correlation resolver/validator for IDs used by logs/metrics/traces/timeline.
- Enforce usage from telemetry and timeline entrypoints.

Done criteria:
- Correlation completeness checks pass across telemetry types (`NF-008`).
- Missing/invalid correlation fails deterministically with explicit diagnostics.

### P0 - Durable Timeline Export (`OBS-06`)

Deliverables:
- Add `internal/observability/timeline/exporter` package with async queue, retry policy, and export lag metrics.
- Wire recorder local append to exporter pipeline with eventual-consistency markers.

Done criteria:
- Control progression and cancel paths remain non-blocking during exporter outage (`F-154`, `NF-026`).
- Deterministic downgrade under pressure remains valid (`F-176`).

### P0 - Replay Engine and Cursor (`OBS-07`)

Deliverables:
- Add replay engine package with explicit mode selection (`re-simulate`, `recorded-provider-playback`).
- Add cursor-based deterministic stepping and resume support.

Done criteria:
- Replay reproducibility tests pass (`NF-009`).
- Mode is explicit in replay outputs (`F-151`, `F-152`).

### P1 - Divergence Reporting Pipeline (`OBS-08`)

Deliverables:
- Build machine-readable report artifact layer on top of comparator findings.
- Add report publishing path for CI and operator tooling.

Done criteria:
- Replay diff reports include output/timing/provider-choice divergence (`F-077`).

### P1 - Extension and Conformance Surface (`OBS-11`)

Deliverables:
- Add extension interfaces for exporter/comparator/redaction plugins.
- Add conformance tests for extension behavior and observability invariants.

Done criteria:
- Extension integrations cannot bypass correlation/redaction/access invariants (`F-164` posture).

## 6) Design Invariants (Implementation Must Preserve)

1. Telemetry must be non-blocking under pressure; drops/sampling are explicit and counted (`F-069`, `F-142`, `NF-002`).
2. Correlation IDs must be consistent across logs/metrics/traces/timeline (`F-067`, `NF-008`).
3. OR-02 accepted-turn completeness must remain `100%` for required baseline fields (`NF-028`).
4. Timeline persistence must never block control progression or cancel fencing (`F-154`, `NF-026`).
5. Recording-level downgrade is deterministic and replay-visible (`F-176`).
6. Replay access is deny-by-default with auditable outcomes (`replay/access.go` + audit service).
7. Secrets/PII must be redacted before persistence/export (`F-117`, `NF-011`, `NF-014`).
8. Divergence classes remain stable when extending replay diagnostics (`F-077`, `NF-009`).
9. Gate-anchor timestamp integrity must be validated for gate calculations (`NF-024`, `NF-025`, `NF-026`, `NF-038`).
10. Retention/deletion remains tenant-scoped and auditable (`F-116`).

## 7) Tradeoffs (Current and Planned)

| Decision | Current choice | Alternative | Why this remains correct now |
| --- | --- | --- | --- |
| Telemetry under overload | Best-effort + bounded queue/drops | Blocking reliable telemetry | Protects hot path latency and lane priority (`F-142`, `NF-002`). |
| Timeline persistence | Local recorder first | Fully synchronous durable writes | Keeps runtime control/cancel responsiveness while export layer is added (`F-154`). |
| Replay maturity | Comparator/access/retention first | Full replay engine first | De-risked security + governance baseline early; replay execution is next P0 (`F-151`, `F-152`). |
| Security posture | Deny-by-default + redaction hooks | Open-by-default debug access | Aligns with `NF-011`/`NF-014` and minimizes leak risk in early stages. |
| Extensibility timing | Delay plugin surface until invariants are stable | Add plugins immediately | Prevents premature API freeze before core replay/export contracts settle. |

## 8) Verification Commands

Current required checks:
- `go test ./internal/observability/...`
- `make verify-quick`
- `make verify-full`
- `make security-baseline-check`

Add when missing modules are implemented:
- replay engine deterministic smoke suite (`OBS-07`)
- timeline export outage/retry suite (`OBS-06`)
- divergence report artifact schema validation (`OBS-08`)

