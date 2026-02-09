.PHONY: test validate-contracts verify-quick verify-full

test:
	go test ./...

validate-contracts:
	go run ./cmd/rspp-cli validate-contracts

verify-quick:
	VERIFY_QUICK_CMD='go run ./cmd/rspp-cli validate-contracts && go test ./test/contract ./internal/runtime/turnarbiter' bash scripts/verify.sh quick

verify-full:
	VERIFY_FULL_CMD='go run ./cmd/rspp-cli validate-contracts && go test ./...' bash scripts/verify.sh full
