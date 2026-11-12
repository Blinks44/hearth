LINT := go tool -modfile=tools/go.mod golangci-lint

.PHONY: dev build build-macos build-windows build-linux test e2e lint fmt check

dev: ## run the app with live reload
	go tool mygo dev

build: ## package the app for this machine into build/
	go tool mygo build

# arm64 targets match the test VMs in docs/manual-checks.md
build-macos: ## package for macOS arm64 into build/darwin-arm64
	go tool mygo build -platform darwin/arm64

build-windows: ## package for Windows arm64 into build/windows-arm64
	go tool mygo build -platform windows/arm64

build-linux: ## package for Linux arm64 into build/linux-arm64
	go tool mygo build -platform linux/arm64

test:
	go test ./...

e2e: ## build the app with the automation plugin and walk its pages (needs a desktop session)
	go test -tags e2e -count=1 -v ./e2e

lint:
	$(LINT) run
	go tool mygo vet .

fmt:
	$(LINT) fmt

check: ## everything CI runs: formatting, lint, mygo vet, catalogs, tests with -race
	$(LINT) fmt --diff
	$(LINT) run
	go tool mygo vet .
	go tool mygo i18n check
	go test -race -timeout 30m ./...
