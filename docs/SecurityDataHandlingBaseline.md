# Security and Data Handling Baseline (Pre-Code)

## 1. Purpose

Define non-optional baseline rules before implementation for:
- data classification tags
- redaction behavior
- replay access constraints
- recording-level defaults (`L0`, `L1`, `L2`)

This baseline is aligned with:
- `docs/rspp_SystemDesign.md` section 4.1.9 and 4.1.6
- `docs/ModularDesign.md` CP-06, RK-05, RK-22, OR-01, OR-02, OR-03

## 2. Security principles

1. Least privilege by default for runtime, replay, and external-node calls.
2. Tenant boundary is strict and must never be bypassed by observability or replay.
3. Replay-critical evidence must be preserved without exposing sensitive raw content.
4. Recording fidelity increases only with explicit policy and stronger access constraints.

## 3. Data classification taxonomy (required tags)

Every payload crossing RK-22 ingress or RK-05 ABI boundary MUST carry exactly one primary data class:

- `audio_raw`: unprocessed user/assistant audio content.
- `text_raw`: unprocessed transcript or model text content.
- `PII`: personal identifiers (name, email, phone, account identifiers, addresses).
- `PHI`: protected health information.
- `derived_summary`: transformed/abstracted content with reduced sensitivity.
- `metadata`: non-content operational metadata (IDs, counters, timing markers).

Optional secondary tags:
- `tenant_sensitive`
- `compliance_hold`
- `retention_short`
- `retention_long`

### 3.1 Tagging rules

1. RK-22 MUST tag ingress payloads before ABI validation.
2. RK-05 MUST reject payloads without a primary classification tag.
3. RK-08 node outputs MUST preserve or explicitly transform tags.
4. OR-02 MUST store the final classification tag and redaction action for each recorded item.

## 4. Redaction baseline

## 4.1 Redaction actions

- `allow`: store and export as-is.
- `mask`: preserve structure but mask sensitive fields.
- `hash`: replace with irreversible digest.
- `drop`: omit payload content; keep only control metadata.
- `tokenize`: replace with reversible token (key-managed, audited access only).

## 4.2 Default action matrix

| Data class | OR-01 telemetry export | OR-02 timeline content | OR-03 replay response |
| --- | --- | --- | --- |
| `audio_raw` | `drop` (metrics only) | `drop` at L0, `mask/hash` at L1, policy-allowed full at L2 | never raw by default; policy-gated |
| `text_raw` | `mask` | `drop/hash` at L0, `mask` at L1, policy-allowed full at L2 | masked by default |
| `PII` | `drop` or `tokenize` | `hash` minimum; no plaintext persistence by default | masked/tokenized only |
| `PHI` | `drop` | `drop/hash` only unless explicit compliance policy | masked/tokenized only |
| `derived_summary` | `allow` | `allow` | `allow` (tenant-authorized) |
| `metadata` | `allow` | `allow` | `allow` |

Non-negotiable rules:
1. Secrets or credentials MUST NEVER appear in OR-01/OR-02/OR-03 payloads.
2. `PII` and `PHI` plaintext replay is denied unless explicit tenant policy allows it and access is audited.
3. Redaction decisions are replay evidence and MUST be recorded.

## 5. Replay access constraints

## 5.1 Access model

Replay access (OR-03) requires all:
1. tenant-scoped authorization
2. role permission for replay scope
3. policy-permitted data class visibility
4. audit logging of request and response scope

## 5.2 Minimum replay authorization attributes

- `tenant_id`
- `principal_id`
- `role`
- `purpose` (debug/incident/eval/compliance)
- `requested_scope` (session/turn/time range)
- `requested_fidelity` (`L0`, `L1`, `L2`)

Any missing attribute => deny.

## 5.3 Replay response constraints

1. Cross-tenant data is always denied.
2. Redaction policy is enforced at read time even if data exists at higher fidelity.
3. Replay APIs MUST return explicit redaction markers for omitted/masked fields.
4. Replay access events MUST be written to immutable audit logs.

## 6. Recording-level defaults (L0/L1/L2)

## 6.1 Global defaults

- Default level for production runtime: `L0`.
- `L1` enabled only for debug windows with TTL and explicit operator action.
- `L2` requires explicit tenant policy opt-in, capacity planning, and compliance approval.

## 6.2 Level behavior

### L0 (default)
- Keep replay-critical control evidence and summaries only.
- Raw payload content may be omitted or hashed.
- Never block runtime hot path.

### L1 (debug)
- Add sampled partial/delta evidence.
- Preserve deterministic downgrade markers when pressure occurs.
- Must still preserve L0 baseline evidence.

### L2 (forensics/eval)
- Full-fidelity capture for approved scopes only.
- On overload, deterministic downgrade to lower level is mandatory.
- If baseline evidence cannot be preserved, terminate with deterministic recording failure outcome.

## 6.3 Level transition controls

1. Runtime may downgrade (`recording_level_downgraded`) under pressure.
2. Runtime MUST NOT auto-upgrade level without control-plane policy.
3. Level changes must be reflected in control markers and replay metadata.

## 7. Retention and deletion baseline

1. Retention is class-based and tenant-policy-driven.
2. `PII`/`PHI` retention windows must be shorter or equal to policy limits.
3. Delete requests must remove or cryptographically render inaccessible all persisted data for the target scope.
4. Audit logs for access/deletion events are retained per compliance policy.

### 7.1 Contract shape (implemented scaffold 2026-02-09)

1. Retention policy resolution is tenant-scoped through a runtime-local resolver contract.
2. Retention policy must declare:
   - default retention window (`default_retention_ms`)
   - class retention windows (`max_retention_by_class_ms`)
   - explicit `PII`/`PHI` policy limits (`pii_retention_limit_ms`, `phi_retention_limit_ms`)
3. Deletion request contract must include:
   - tenant scope (`tenant_id`)
   - exactly one selector (`session_id` or `turn_id`)
   - deletion mode (`hard_delete` or `crypto_inaccessible`)
   - requestor + timestamp (`requested_by`, `requested_at_ms`)

### 7.2 Baseline backend path and behavior

1. Retention/deletion scaffold contract and in-memory baseline implementation live in `internal/observability/replay/retention.go`.
2. Immutable replay-audit sink interface and backend access path wrapper live in `internal/observability/replay/service.go`.
3. Replay access authorization must fail closed when immutable audit append fails or audit sink is unavailable.
4. `hard_delete` removes matching replay artifacts for the requested tenant/session-or-turn scope.
5. `crypto_inaccessible` keeps matching replay artifacts but makes reads fail with an inaccessible outcome.
6. Deleting replay artifacts does not delete immutable audit log entries.

## 8. Pre-code acceptance checklist (status: implemented 2026-02-09)

- [x] Classification tags implemented at ingress and enforced at ABI boundary.
  - Evidence: `internal/runtime/transport/classification.go`, `internal/runtime/transport/classification_test.go`, `internal/runtime/eventabi/gateway_test.go`
- [x] Redaction matrix encoded into runtime policy config.
  - Evidence: `internal/security/policy/policy.go`, `internal/security/policy/policy_test.go`, `internal/observability/timeline/redaction.go`, `internal/observability/timeline/redaction_test.go`
- [x] Replay auth attributes and deny-by-default checks defined.
  - Evidence: `api/observability/types.go`, `api/observability/types_test.go`, `internal/observability/replay/access.go`, `internal/observability/replay/access_test.go`
- [x] L0/L1/L2 defaults and transition controls approved.
  - Evidence: `internal/security/policy/policy.go` (`ValidateAutomatedRecordingLevelTransition`), `internal/observability/timeline/recorder.go` (`recording_level_downgraded`)
- [x] Security audit event schema agreed before OR-03 implementation.
  - Evidence: `api/observability/types.go` (`ReplayAuditEvent`), `internal/observability/replay/access.go` (`BuildReplayAuditEvent`)
