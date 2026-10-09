GO ?= go
GOFMT ?= gofmt
PACKAGES := ./cmd/...
BINARY := bin/anchorage

.PHONY: help build run test vet fmt check clean

help:
	@printf '%s\n' \
	  'make build             Build bin/anchorage' \
	  'make run ARGS="--help"  Run the CLI' \
	  'make test              Run tests' \
	  'make vet               Run go vet' \
	  'make fmt               Format Go source' \
	  'make check             Check formatting, tests, and vet' \
	  'make clean             Remove the built binary'

build:
	$(GO) build -o $(BINARY) ./cmd/anchorage

run:
	$(GO) run ./cmd/anchorage $(ARGS)

test:
	$(GO) test $(PACKAGES)

vet:
	$(GO) vet $(PACKAGES)

fmt:
	$(GO) fmt $(PACKAGES)

check:
	@test -z "$$($(GOFMT) -l cmd)" || { echo 'Go source needs formatting; run make fmt'; exit 1; }
	$(MAKE) test vet

clean:
	rm -f $(BINARY)
