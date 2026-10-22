VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GOBIN := $(shell go env GOPATH)/bin
GOLANGCI_LINT ?= $(GOBIN)/golangci-lint
GO_LICENSES ?= $(GOBIN)/go-licenses

.PHONY: ci build test test-integration test-integration-homelab lint check-emdash check-stories bench-compat licenses notice

# What GitHub Actions used to run, locally (Actions is off on the repo).
ci: lint licenses test test-integration-homelab

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o dist/grapi ./cmd/grapi

test:
	go test ./...

# Needs GRAPI_TEST_PG_DSN for Postgres; SQLite runs in a temp dir. One package at
# a time: the Postgres tests reset the shared database.
test-integration:
	go test -p 1 -tags integration ./...

# Same, against a throwaway Postgres on the homelab.
test-integration-homelab:
	./scripts/with-homelab-postgres.sh go test -p 1 -tags integration ./...

lint: check-emdash check-stories
	$(GOLANGCI_LINT) run ./...

check-emdash:
	./scripts/check-emdash.sh

check-stories:
	./scripts/check-stories.sh

FIXTURE ?= basic
MODE ?= self
DB ?= sqlite

# Runs on the homelab (COMPAT_HOST), never on this machine.
bench-compat:
	./compat/scripts/bench.sh $(FIXTURE) $(MODE) $(DB)

ALLOWED_LICENSES := MIT,BSD-2-Clause,BSD-3-Clause,Apache-2.0,ISC,MPL-2.0

licenses:
	$(GO_LICENSES) check ./... --allowed_licenses=$(ALLOWED_LICENSES)

notice:
	$(GO_LICENSES) report ./... --ignore github.com/aluzed/grapi > NOTICE
