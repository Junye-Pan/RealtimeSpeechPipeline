.PHONY: test validate-contracts verify-quick verify-full verify-mvp live-provider-smoke livekit-smoke a2-runtime-live security-baseline-check codex-artifact-policy-check livekit-local-up livekit-local-validate livekit-local-down

test:
	go test ./...

validate-contracts:
	go run ./cmd/rspp-cli validate-contracts

verify-quick:
	VERIFY_QUICK_CMD='go run ./cmd/rspp-cli validate-contracts && go run ./cmd/rspp-cli validate-contracts-report && go run ./cmd/rspp-cli replay-smoke-report && go run ./cmd/rspp-cli generate-runtime-baseline && go run ./cmd/rspp-cli slo-gates-report && go test ./api/controlplane ./api/eventabi ./internal/runtime/planresolver ./internal/runtime/turnarbiter ./internal/runtime/executor ./internal/runtime/buffering ./internal/runtime/guard ./internal/runtime/transport ./internal/observability/replay ./internal/observability/timeline ./internal/observability/telemetry ./internal/tooling/regression ./internal/tooling/ops ./internal/tooling/release ./transports/livekit ./test/contract ./test/integration ./test/replay && go test ./test/failover -run '\''TestF[137]'\''' bash scripts/verify.sh quick

verify-full:
	VERIFY_FULL_CMD='go run ./cmd/rspp-cli validate-contracts && go run ./cmd/rspp-cli validate-contracts-report && go run ./cmd/rspp-cli replay-regression-report && go run ./cmd/rspp-cli generate-runtime-baseline && go run ./cmd/rspp-cli slo-gates-report && go test ./...' bash scripts/verify.sh full

verify-mvp:
	VERIFY_MVP_CMD='go run ./cmd/rspp-cli validate-contracts && go run ./cmd/rspp-cli validate-contracts-report && go run ./cmd/rspp-cli replay-regression-report && go run ./cmd/rspp-cli generate-runtime-baseline && go run ./cmd/rspp-cli slo-gates-report && go run ./cmd/rspp-cli live-latency-compare-report && go run ./cmd/rspp-cli slo-gates-mvp-report && go test ./...' bash scripts/verify.sh mvp

live-provider-smoke:
	go test -tags=liveproviders ./test/integration -run TestLiveProviderSmoke -v

livekit-smoke:
	go test -tags=livekitproviders ./test/integration -run TestLiveKitSmoke -v

a2-runtime-live:
	RSPP_LIVE_PROVIDER_SMOKE=1 go test -tags=liveproviders ./test/integration -run TestLiveProviderSmoke -v
	RSPP_A2_RUNTIME_LIVE=1 RSPP_A2_RUNTIME_LIVE_STRICT=1 go test -tags=liveproviders ./test/integration -run TestA2RuntimeLiveScenarios -v

security-baseline-check:
	bash scripts/security-check.sh

codex-artifact-policy-check:
	bash scripts/check-codex-artifact-policy.sh

livekit-local-up:
	bash ops/livekit-local/scripts/bootstrap.sh

livekit-local-validate:
	bash ops/livekit-local/scripts/validate.sh

livekit-local-down:
	docker compose -f ops/livekit-local/docker-compose.yml down
