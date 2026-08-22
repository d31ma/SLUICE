GO ?= go
ACTIVE_GO := $(shell $(GO) env GOROOT)/bin/go

.DEFAULT_GOAL := help

.PHONY: help bootstrap runtime-tools fmt fmt-check vet lint test test-race crap vuln licenses docs workflows runtime-contract app-bundle-contract runtime-stage-macos app-package-macos app-package-windows app-runtime-qualify-macos filesystem-qualify-macos linux-qualify-arm64 build check

help:
	@echo "Sluice development commands"
	@echo "  make bootstrap  download pinned Go modules and native runtime tools"
	@echo "  make fmt        format Go source"
	@echo "  make check      run the complete local quality gate"
	@echo "  make runtime-contract  validate pinned runtime bundle metadata"
	@echo "  make workflows  validate immutable GitHub Actions references"
	@echo "  make runtime-stage-macos  stage and execute the macOS arm64 runtime payload"
	@echo "  make app-package-macos  stage and exercise an unsigned native macOS arm64 Sluice.app"
	@echo "  make app-package-windows  stage and exercise a native Windows amd64 portable package"
	@echo "  make app-runtime-qualify-macos  record a ten-minute native packaged-process baseline"
	@echo "  make filesystem-qualify-macos  run native arm64 APFS destructive fixtures in an isolated image"
	@echo "  make linux-qualify-arm64  run native Linux arm64 runtime, IPC, filesystem, and AppDir qualification"
	@echo "  make build      build all Go packages"

bootstrap: runtime-tools
	$(GO) mod download

runtime-tools:
	./scripts/bootstrap-runtime-tools.sh

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

crap: runtime-tools
	PATH="$(CURDIR)/tools/bin:$$PATH" SLUICE_REQUIRE_INSTALLED_RUNTIMES=1 $(GO) tool go-crap scan ./... --fail-above --threshold 15 --min 15 --missing pessimistic --no-progress

vuln:
	$(GO) tool govulncheck ./...

licenses:
	"$(ACTIVE_GO)" run github.com/google/go-licenses/v2@v2.0.1 check ./... --allowed_licenses=Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,MIT,MPL-2.0,Unlicense

docs:
	./scripts/check-docs.sh

workflows:
	./scripts/check-workflows.sh

runtime-contract:
	./scripts/test-runtime-bundle-contract.sh

app-bundle-contract:
	./scripts/check-app-bundle-contract.sh

runtime-stage-macos:
	./scripts/test-runtime-bundle-macos.sh

app-package-macos:
	./scripts/test-app-bundle-macos.sh

app-package-windows:
	pwsh -NoLogo -NoProfile -File ./scripts/test-app-bundle-windows.ps1

app-runtime-qualify-macos:
	./scripts/qualify-app-runtime-macos.sh

filesystem-qualify-macos:
	./scripts/test-filesystem-macos.sh

linux-qualify-arm64:
	./scripts/qualify-linux-arm64-container.sh

build:
	$(GO) build ./...

check: fmt-check vet lint test-race crap vuln licenses docs workflows runtime-contract app-bundle-contract build
