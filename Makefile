GO ?= go
ACTIVE_GO := $(shell $(GO) env GOROOT)/bin/go

.DEFAULT_GOAL := help

.PHONY: help bootstrap fmt fmt-check vet lint test test-race crap vuln licenses docs build check

help:
	@echo "Sluice development commands"
	@echo "  make bootstrap  download the pinned Go toolchain and modules"
	@echo "  make fmt        format Go source"
	@echo "  make check      run every Phase 0 quality gate"
	@echo "  make build      build all Go packages"

bootstrap:
	$(GO) mod download

fmt:
	$(GO) tool gofumpt -w .

fmt-check:
	@test -z "$$($(GO) tool gofumpt -l .)" || \
		($(GO) tool gofumpt -d . && echo "Go files need formatting" && exit 1)

vet:
	$(GO) vet ./...

lint:
	$(GO) tool golangci-lint run ./...

test:
	$(GO) test -count=1 ./...

test-race:
	$(GO) test -race -count=1 ./...

crap:
	$(GO) tool go-crap scan ./... --fail-above --threshold 15 --min 15 --missing pessimistic --no-progress

vuln:
	$(GO) tool govulncheck ./...

licenses:
	"$(ACTIVE_GO)" run github.com/google/go-licenses/v2@v2.0.1 check ./... --allowed_licenses=Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,MIT,MPL-2.0,Unlicense

docs:
	./scripts/check-docs.sh

build:
	$(GO) build ./...

check: fmt-check vet lint test-race crap vuln licenses docs build
