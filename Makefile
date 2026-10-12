GO ?= go
GOFMT ?= gofmt
PACKAGES := ./cmd/... ./internal/...
BINARY := bin/bundledock
VERSION ?= dev
RELEASE_DIR ?= dist/cli
export GO VERSION RELEASE_DIR

TOOLS_IMAGE ?= bundledock-toolbox:poc
TOOLS_CONSUMER ?= mcr.microsoft.com/devcontainers/go:2-1.27-trixie

.PHONY: release release-test help build run test vet fmt check clean tools-build tools-volume tools-smoke tools-test docs docs-serve docs-test

help:
	@printf '%s\n' \
	  'make build             Build bin/bundledock' \
	  'make release           Build local two-architecture CLI artifacts' \
	  'make release-test      Verify CLI release artifacts end to end' \
	  'make run ARGS="--help"  Run the CLI' \
	  'make test              Run tests' \
	  'make vet               Run go vet' \
	  'make fmt               Format Go source' \
	  'make check             Check formatting, tests, and vet' \
	  'make docs              Build the local Hugo documentation in public/' \
	  'make docs-serve        Preview documentation at http://localhost:1313/' \
	  'make docs-test         Check rendered documentation, links, and assets' \
	  'make clean             Remove the built binary'
	@printf '%s\n' 'make tools-build       Build the local OCI tool bundle'
	@printf '%s\n' 'make tools-volume      Initialize a versioned tool volume' \
	  'make tools-smoke       Test shared read-only tool execution' \
	  'make tools-test        Test native package extraction'

tools-build:
	docker build -f bundles/toolbox/Dockerfile -t $(TOOLS_IMAGE) .

tools-test:
	$(GO) test ./cmd/bundledock ./internal/toolmanager ./internal/toolconfig ./internal/toolbundle ./internal/toolvolume ./internal/dockercli

tools-volume:
	sh bundles/toolbox/volume.sh "$(TOOLS_IMAGE)"

tools-smoke:
	sh bundles/toolbox/smoke.sh "$(TOOLS_IMAGE)" "$(TOOLS_CONSUMER)"

build:
	$(GO) build -o $(BINARY) ./cmd/bundledock

run:
	$(GO) run ./cmd/bundledock $(ARGS)

test:
	$(GO) test $(PACKAGES)

vet:
	$(GO) vet $(PACKAGES)

fmt:
	$(GO) fmt $(PACKAGES)

check:
	python3 scripts/tool-inventory.py --check
	python3 scripts/tool-inventory.py --bundle code-server --check
	python3 scripts/tool-inventory.py --bundle node --check
	python3 scripts/tool-inventory.py --bundle bun --check
	@test -z "$$($(GOFMT) -l cmd internal)" || { echo 'Go source needs formatting; run make fmt'; exit 1; }
	$(MAKE) test vet

clean:
	rm -f $(BINARY)

release:
	sh scripts/release.sh

release-test:
	sh scripts/release-test.sh

# The docs wrapper verifies its own pinned archive, independent of PATH.
docs:
	bash scripts/hugo.sh --panicOnWarning

docs-serve:
	bash scripts/hugo.sh server --bind 127.0.0.1

docs-test:
	BUNDLEDOCK_DOCS_TEST=1 $(GO) test ./scripts -count=1
