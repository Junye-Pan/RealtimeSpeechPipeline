.PHONY: test validate-contracts verify-quick verify-full live-provider-smoke a2-runtime-live security-baseline-check

test:
	go test ./...

validate-contracts:
	go run ./cmd/rspp-cli validate-contracts

verify-quick:
	VERIFY_QUICK_CMD='go run ./cmd/rspp-cli validate-contracts && go run ./cmd/rspp-cli replay-smoke-report && go run ./cmd/rspp-cli generate-runtime-baseline && go run ./cmd/rspp-cli slo-gates-report && go test ./api/controlplane ./api/eventabi ./internal/runtime/planresolver ./internal/runtime/turnarbiter ./internal/runtime/executor ./internal/runtime/buffering ./internal/runtime/guard ./internal/runtime/transport ./internal/observability/replay ./internal/observability/timeline ./internal/tooling/regression ./internal/tooling/ops ./test/contract ./test/integration ./test/replay && go test ./test/failover -run '\''TestF[137]'\''' bash scripts/verify.sh quick

verify-full:
	VERIFY_FULL_CMD='go run ./cmd/rspp-cli validate-contracts && go run ./cmd/rspp-cli replay-regression-report && go run ./cmd/rspp-cli generate-runtime-baseline && go run ./cmd/rspp-cli slo-gates-report && go test ./...' bash scripts/verify.sh full

live-provider-smoke:
	go test -tags=liveproviders ./test/integration -run TestLiveProviderSmoke -v

a2-runtime-live:
	RSPP_LIVE_PROVIDER_SMOKE=1 go test -tags=liveproviders ./test/integration -run TestLiveProviderSmoke -v
	RSPP_A2_RUNTIME_LIVE=1 RSPP_A2_RUNTIME_LIVE_STRICT=1 go test -tags=liveproviders ./test/integration -run TestA2RuntimeLiveScenarios -v

security-baseline-check:
	bash scripts/security-check.sh
