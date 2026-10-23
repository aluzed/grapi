VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GOBIN := $(shell go env GOPATH)/bin
GOLANGCI_LINT ?= $(GOBIN)/golangci-lint
GO_LICENSES ?= $(GOBIN)/go-licenses

.PHONY: docker ci admin admin-check build test race test-integration test-integration-homelab lint check-emdash check-stories bench-compat licenses notice

# What GitHub Actions used to run, locally (Actions is off on the repo).
ci: lint licenses admin-check test test-integration-homelab

# The admin SPA (Bun); build embeds it with the adminui tag.
admin:
	cd admin && bun install --frozen-lockfile && bun run build

admin-check:
	cd admin && bun install --frozen-lockfile && bun run typecheck && bun test src

build: admin
	go build -trimpath -tags adminui -ldflags "$(LDFLAGS)" -o dist/grapi ./cmd/grapi

test:
	go test ./...

race:
	go test -race ./...

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

# The image is built on the homelab only (COMPAT_HOST), never on this machine.
COMPAT_HOST ?= homelab
docker:
	rsync -a --delete --exclude .git --exclude /dist --exclude node_modules --exclude admin/dist --exclude compat/bin --exclude compat/report --exclude compat/golden ./ $(COMPAT_HOST):grapi-build/
	ssh $(COMPAT_HOST) 'cd grapi-build && docker build --build-arg VERSION=$(VERSION) -t grapi:$(VERSION) -t grapi:latest .'

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
