LINT := go tool -modfile=tools/go.mod golangci-lint

.PHONY: dev build test e2e lint fmt check

dev: ## run the app with live reload
	go tool mygo dev

build: ## package the app for this machine into build/
	go tool mygo build

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
	go test -race ./...
