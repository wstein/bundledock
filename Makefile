GO ?= go
GOFMT ?= gofmt
PACKAGES := ./cmd/... ./internal/...
BINARY := bin/bdock
VERSION ?= dev
RELEASE_DIR ?= dist/cli
export GO VERSION RELEASE_DIR

BUNDLE ?= toolbox
ifneq ($(words $(BUNDLE)),1)
$(error BUNDLE must be toolbox, code-server, node, node-current, bun, codex, agy, claude, opentofu or nixbox)
endif
ifneq ($(filter toolbox code-server node node-current bun codex agy claude opentofu nixbox,$(BUNDLE)),$(BUNDLE))
$(error BUNDLE must be toolbox, code-server, node, node-current, bun, codex, agy, claude, opentofu or nixbox)
endif
IMAGE ?= bundledock-$(BUNDLE):poc
CONSUMER ?= mcr.microsoft.com/devcontainers/go:2-1.27-trixie
export IMAGE CONSUMER

.PHONY: release release-test help build run test vet fmt check clean bundle-build bundle-volume bundle-smoke bundle-test docs docs-serve docs-test

help:
	@printf '%s\n' \
	  'make build             Build bin/bdock' \
	  'make release           Build local CLI artifacts (linux amd64/arm64, darwin arm64)' \
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
	@printf '%s\n' 'make bundle-build      Build the selected OCI bundle (BUNDLE=toolbox by default)'
	@printf '%s\n' 'make bundle-volume     Initialize the selected bundle volume' \
	  'make bundle-smoke      Verify selected bundle in a read-only consumer' \
	  'make bundle-test       Run focused source tests (no OCI build)' \
	  'BUNDLE=toolbox|code-server|node|node-current|bun|codex|agy|claude|opentofu|nixbox; override IMAGE and CONSUMER as needed'

bundle-build:
	docker build -f bundles/$(BUNDLE)/Dockerfile -t "$$IMAGE" .

bundle-test:
	$(GO) test ./cmd/bdock ./internal/toolmanager ./internal/toolconfig ./internal/toolbundle ./internal/toolvolume ./internal/dockercli

bundle-volume:
	sh scripts/bundle-volume.sh "$$IMAGE" "$(BUNDLE)"

bundle-smoke:
	sh bundles/$(BUNDLE)/scripts/smoke.sh "$$IMAGE" "$$CONSUMER"

build:
	$(GO) build -o $(BINARY) ./cmd/bdock

run:
	$(GO) run ./cmd/bdock $(ARGS)

test:
	$(GO) test $(PACKAGES)

vet:
	$(GO) vet $(PACKAGES)

fmt:
	$(GO) fmt $(PACKAGES)

check:
	python3 -B -m unittest discover -s scripts/tests
	python3 -B -m unittest discover -s scripts -p '*_test.py'
	python3 scripts/tool-inventory.py --check
	python3 scripts/tool-inventory.py --bundle code-server --check
	python3 scripts/tool-inventory.py --bundle node --check
	python3 scripts/tool-inventory.py --bundle node-current --check
	python3 scripts/tool-inventory.py --bundle bun --check
	python3 scripts/tool-inventory.py --bundle codex --check
	python3 scripts/tool-inventory.py --bundle agy --check
	python3 scripts/tool-inventory.py --bundle claude --check
	python3 scripts/tool-inventory.py --bundle opentofu --check
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
