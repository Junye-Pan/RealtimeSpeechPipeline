# Security and Data Handling Baseline (Repository-Level)

## 1. Purpose

Define repository-wide, non-optional baseline policy for:

- payload classification
- redaction behavior
- replay access controls
- recording-level and retention/deletion posture

Implementation details for individual modules are canonical in domain guide files.

## 2. Security principles

1. Least privilege by default.
2. Strict tenant isolation across runtime, observability, and replay access.
3. Replay-critical evidence preservation without leaking sensitive raw content.
4. Recording fidelity increases only through explicit policy and stronger access controls.

## 3. Payload classification taxonomy

Every payload must carry exactly one primary class:

- `audio_raw`
- `text_raw`
- `PII`
- `PHI`
- `derived_summary`
- `metadata`

Optional policy tags may include retention/compliance or tenant sensitivity hints.

## 4. Redaction baseline

Redaction actions:

- `allow`
- `mask`
- `hash`
- `drop`
- `tokenize`

### Default action matrix

| Data class | Telemetry export | Timeline content | Replay response |
| --- | --- | --- | --- |
| `audio_raw` | drop by default | reduced at lower levels; policy-gated at highest level | policy-gated; not raw by default |
| `text_raw` | masked by default | reduced or masked by level | masked by default |
| `PII` | drop/tokenize | hash or stronger | masked/tokenized only |
| `PHI` | drop | drop/hash unless explicit policy | masked/tokenized only |
| `derived_summary` | allow | allow | allow with tenant authorization |
| `metadata` | allow | allow | allow |

Non-negotiable rules:

1. Secrets/credentials must never be emitted in observability payloads.
2. Plaintext `PII`/`PHI` replay is denied unless explicit tenant/compliance policy allows it.
3. Redaction decisions must be audit-visible as replay evidence.

## 5. Replay access constraints

Replay access requires all of:

1. tenant-scoped authorization
2. role permission for replay scope
3. policy-permitted data class visibility
4. immutable audit logging of request and response scope

Minimum request attributes:

- `tenant_id`
- `principal_id`
- `role`
- `purpose`
- `requested_scope`
- `requested_fidelity` (`L0`, `L1`, `L2`)

Missing required attributes result in deny-by-default.

## 6. Recording levels

### Global defaults

- Production default: `L0`
- `L1`: explicit, time-bounded debug windows
- `L2`: explicit tenant policy + compliance and capacity approval

### Level behavior summary

- `L0`: replay-critical control evidence only, no hot-path blocking
- `L1`: adds partial debug evidence while preserving `L0` baseline guarantees
- `L2`: approved full-fidelity scopes with mandatory deterministic downgrade under pressure

Level transitions:

1. Runtime may downgrade under pressure with explicit marker.
2. Runtime must not auto-upgrade without policy.
3. Level changes must be recorded for replay/audit context.

## 7. Retention and deletion baseline

1. Retention is class-based and tenant-policy-driven.
2. `PII`/`PHI` windows must remain within policy limits.
3. Delete workflows must support hard-delete or cryptographic inaccessibility semantics.
4. Access/deletion audit logs are retained according to compliance policy.

## 8. Canonical detail pointers

- Event and redaction contract types: `api/eventabi/eventabi_api_guide.md`, `api/observability/observability_api_guide.md`
- Runtime and observability enforcement paths: `internal/runtime/runtime_kernel_guide.md`, `internal/observability/observability_module_guide.md`
- Provider/transport boundary behavior: `providers/provider_adapter_guide.md`, `transports/transport_adapter_guide.md`
- Gate enforcement: `docs/CIValidationGates.md`, `internal/tooling/tooling_and_gates_guide.md`
