# Provider Adapter Guide

This folder contains provider-specific STT/LLM/TTS adapters and shared HTTP/SSE transport helpers used by runtime provider invocation.
The same provider contract also applies to additional modalities (for example translation) when those adapters are introduced.

## Ownership and boundaries

- Primary owner: `Provider-Team`
- Cross-review: `Runtime-Team` for runtime contract changes, `Security-Team` for payload/redaction changes

This folder owns provider protocol adaptation only. It does not own turn scheduling, lifecycle arbitration, or authority policy.

## Folder map

| Path | Responsibility |
| --- | --- |
| `providers/common/httpadapter` | shared HTTP invocation path, timeout/retry handling, normalized outcome mapping, bounded payload capture |
| `providers/common/streamsse` | SSE stream parser utilities |
| `providers/stt/*` | STT adapters (`assemblyai`, `deepgram`, `google`) |
| `providers/llm/*` | LLM adapters (`anthropic`, `cohere`, `gemini`) |
| `providers/tts/*` | TTS adapters (`elevenlabs`, `google`, `polly`) |

Retry ownership note:
1. Runtime-owned retries: provider attempt loops, retry decisions, and provider-switch behavior are owned by `internal/runtime/provider/invocation`.
2. Provider adapter-owned attempt execution: `providers/common/httpadapter` performs a single HTTP attempt per invocation and returns normalized outcomes/backoff hints (including `Retry-After` mapping).

## Runtime integration contract

Runtime provider interfaces are defined in:
- `internal/runtime/provider/contracts`
- `internal/runtime/provider/registry`
- `internal/runtime/provider/invocation`

Adapter expectations:
1. Parse deterministic env config (`ConfigFromEnv`).
2. Construct explicit adapter instances (`NewAdapter`).
3. Return normalized `contracts.Outcome` classes.
4. Preserve streaming ordering/lifecycle for stream-capable providers.
5. Emit provider invocation metadata that preserves identity, cancellation scope, and replay correlation.

## Provider invocation and limits contract alignment

ProviderInvocation requirements (PRD alignment):
1. Treat each provider call as a single `ProviderInvocation` unit with stable identity (`provider_invocation_id`) and explicit attempt lineage.
2. Preserve explicit idempotency behavior for retries/provider switches (forward provider idempotency keys where supported; keep invocation lineage stable for dedupe/replay correlation).
3. Respect provider-invocation cancellation scope: after cancel acceptance, stop chunk emission promptly and return terminal outcome events that cooperate with runtime output-fence semantics.
4. Emit deterministic invocation evidence for observability/replay (provider/model identity, attempt index, normalized terminal outcome, and terminal timing markers).

Provider limits requirements (PRD alignment):
1. Normalize provider limit surfaces (rate-limit, concurrency saturation, payload-size constraints) into deterministic outcome classes and backoff hints.
2. Preserve structured retry guidance (`Retry-After` and equivalent hints) in normalized outcomes when upstream providers return it.
3. Perform deterministic fail-fast mapping for known payload-bound violations before send when the adapter can prove a request cannot be accepted upstream.

## Streaming interface policy

Provider interfaces:
- `contracts.Adapter` is the baseline request/response surface.
- `contracts.StreamingAdapter` is the required path for native provider streaming behavior.

Required native streaming behavior:
1. Emit deterministic `start`, ordered chunk events, and terminal `final`/`error`.
2. Preserve monotonic stream sequence inside one provider invocation.
3. Respect cancellation quickly and stop chunk emission after cancel fence.
4. Keep output chunk semantics stable across retries/provider switches.

Current implementation detail (sequence/lifecycle enforcement scope):
1. Runtime contract validation currently enforces per-chunk structural invariants (required IDs, modality, attempt bounds, non-negative sequence/timestamps, and kind-specific fields).
2. Runtime invocation observer currently validates each stream event independently before forwarding hooks.
3. Monotonic sequence progression and strict `start -> chunk(s) -> final/error` lifecycle ordering are currently enforced by adapter implementations/tests, not by a central runtime state machine.

Wrapper behavior for non-native adapters:
1. If `InvokeStream` is not implemented natively, runtime wraps `Invoke` with synthetic `start` + terminal `final`/`error`.
2. This wrapper preserves contract compatibility but does not provide true overlap latency benefits.
3. Migration target is native per-provider `InvokeStream` for STT, LLM, and TTS adapters used in live lanes.

Current implementation detail (wrapper path):
1. Runtime invocation currently enables streaming only when an adapter implements `contracts.StreamingAdapter`; otherwise it calls unary `Invoke` directly.
2. Synthetic `start/final/error` compatibility wrappers are currently implemented in adapter/helper surfaces (for example `providers/common/httpadapter`), rather than inside runtime invocation controller fallback logic.
3. `internal/runtime/provider/contracts.StaticAdapter` also provides helper/test-level default `InvokeStream` wrapping for static adapters.

Current implementation progress:
1. MVP live adapters now expose `InvokeStream` across STT/LLM/TTS provider sets.
2. Runtime invocation defaults to streaming when a provider implements `contracts.StreamingAdapter` (disable knobs: `RSPP_PROVIDER_STREAMING_DISABLE`, `RSPP_PROVIDER_STREAMING_{STT|LLM|TTS}_DISABLE`).
3. Some providers still use request/response upstream APIs and emit deterministic stream chunks from sampled responses; these paths satisfy stream contract semantics but do not provide full upstream transport overlap.
4. `providers/common/httpadapter` keeps the synthetic `start/final/error` stream wrapper as a compatibility fallback for adapters that have not implemented native stream logic.

## STT native streaming migration plan (documentation scope)

Target scope for migration is all STT adapters:
- `providers/stt/assemblyai`
- `providers/stt/deepgram`
- `providers/stt/google`

Planned migration rules:
1. `InvokeStream` should use provider-native incremental transport where available.
2. Unary `Invoke` remains supported, but unary and streaming success semantics must represent equivalent completion points for fair comparison.
3. Streaming event emission keeps deterministic lifecycle ordering (`start`, ordered chunk events, terminal `final` or `error`).
4. Adapter-level cadence controls are explicit and bounded; current additive knob:
   - `RSPP_STT_ASSEMBLYAI_POLL_INTERVAL_MS` (implemented; bounded in adapter config)
5. Live-smoke and observability artifacts must label execution mode and semantic parity status during A/B latency comparisons.

Normalized outcome classes include deterministic mapping for classes such as:
- `timeout`
- `overload`
- `blocked`
- `safety_block` (mapped blocked reason, not a standalone `OutcomeClass`)
- `infrastructure_failure`
- provider-specific mapped failures

## Data-handling and safety constraints

1. Payload capture in `common/httpadapter` stays bounded and mode-controlled.
2. Raw sensitive payload handling must follow repo redaction/classification policy.
3. Adapters must not emit ad-hoc control vocabularies outside runtime/API contracts.
4. Cancel behavior must cooperate with runtime cancellation and output-fence semantics.

## Test and gate touchpoints

- `go test ./providers/...`
- `go test ./internal/runtime/provider/...`
- `make verify-quick`
- `make live-provider-smoke` (optional live credentials path)

PRD MVP standards tie-in:
1. Provider adapter changes must preserve integrated happy-path first-output latency targets (`turn_open -> first DataLane chunk` p95 <= 1500 ms).
2. Provider streaming/cancel behavior must preserve integrated cancellation fence targets (`cancel acceptance -> egress output fence` p95 <= 150 ms).
3. MVP mode is `Simple` only: adapter defaults and contract behavior must remain compatible with ExecutionProfile default semantics; advanced profile overrides are deferred.
4. OR-02 baseline replay evidence is mandatory for accepted turns: provider invocation metadata must preserve replay-critical identity/outcome evidence required by the baseline contract.

## Change checklist

1. Keep outcome normalization consistent across providers.
2. Add/update tests for new provider env knobs and error mappings.
3. For streaming providers, validate deterministic chunk ordering.
4. Re-run provider + runtime invocation tests.

## Related repo-level docs

- `docs/repository_docs_index.md`
- `docs/rspp_SystemDesign.md`
- `docs/SecurityDataHandlingBaseline.md`
- `docs/CIValidationGates.md`
