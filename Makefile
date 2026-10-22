VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GOLANGCI_LINT ?= golangci-lint

.PHONY: build test test-integration lint check-emdash bench-compat licenses notice

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o dist/grapi ./cmd/grapi

test:
	go test ./...

# Needs GRAPI_TEST_PG_DSN for Postgres; SQLite runs in a temp dir.
test-integration:
	go test -tags integration ./...

lint: check-emdash
	$(GOLANGCI_LINT) run ./...

check-emdash:
	./scripts/check-emdash.sh

bench-compat:
	go run ./compat/cmd/bench

ALLOWED_LICENSES := MIT,BSD-2-Clause,BSD-3-Clause,Apache-2.0,ISC,MPL-2.0

licenses:
	go-licenses check ./... --allowed_licenses=$(ALLOWED_LICENSES)

notice:
	go-licenses report ./... --ignore github.com/aluzed/grapi > NOTICE
