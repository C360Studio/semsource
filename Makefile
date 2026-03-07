.PHONY: build test test-unit test-integration test-race lint

build:
	go build ./...

# Unit tests only (default, fast, no external deps)
test: test-unit

test-unit:
	go test ./...

# Integration tests (fsnotify, git binary, testcontainers)
test-integration:
	go test -tags=integration ./...

# All tests with race detector
test-race:
	go test -race -tags=integration ./...

# Coverage report
test-coverage:
	go test -tags=integration -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

lint:
	go vet ./...
