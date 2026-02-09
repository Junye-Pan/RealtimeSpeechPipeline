.PHONY: test validate-contracts verify-quick verify-full

test:
	go test ./...

validate-contracts:
	go run ./cmd/rspp-cli validate-contracts

verify-quick:
	VERIFY_QUICK_CMD='go run ./cmd/rspp-cli validate-contracts && go run ./cmd/rspp-cli replay-smoke-report && go test ./api/controlplane ./api/eventabi ./internal/runtime/planresolver ./internal/runtime/turnarbiter ./internal/runtime/executor ./internal/runtime/buffering ./internal/observability/replay ./test/contract ./test/integration ./test/replay ./test/failover' bash scripts/verify.sh quick

verify-full:
	VERIFY_FULL_CMD='go run ./cmd/rspp-cli validate-contracts && go run ./cmd/rspp-cli replay-smoke-report && go test ./...' bash scripts/verify.sh full
