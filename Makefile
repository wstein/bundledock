GO ?= go
GOFMT ?= gofmt
PACKAGES := ./cmd/... ./internal/...
BINARY := bin/bundledock
VERSION ?= dev
RELEASE_DIR ?= dist/cli
export GO VERSION RELEASE_DIR

TOOLS_IMAGE ?= bundledock-toolbox:poc
TOOLS_CONSUMER ?= mcr.microsoft.com/devcontainers/go:2-1.27-trixie

.PHONY: release release-test help build run test vet fmt check clean tools-build tools-volume tools-smoke tools-test

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
	  'make clean             Remove the built binary'
	@printf '%s\n' 'make tools-build       Build the local OCI tool bundle'
	@printf '%s\n' 'make tools-volume      Initialize a versioned tool volume' \
	  'make tools-smoke       Test shared read-only tool execution' \
	  'make tools-test        Test native package extraction'

tools-build:
	docker build -f oci/Dockerfile -t $(TOOLS_IMAGE) .

tools-test:
	$(GO) test ./cmd/bundledock ./internal/toolmanager ./internal/toolconfig ./internal/toolbundle ./internal/toolvolume ./internal/dockercli

tools-volume:
	sh oci/volume.sh "$(TOOLS_IMAGE)"

tools-smoke:
	sh oci/smoke.sh "$(TOOLS_IMAGE)" "$(TOOLS_CONSUMER)"

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
	@test -z "$$($(GOFMT) -l cmd internal)" || { echo 'Go source needs formatting; run make fmt'; exit 1; }
	$(MAKE) test vet

clean:
	rm -f $(BINARY)

release:
	sh scripts/release.sh

release-test:
	sh scripts/release-test.sh
