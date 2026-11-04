LINT := go tool -modfile=tools/go.mod golangci-lint

.PHONY: dev build test lint fmt check

dev: ## run the app with live reload
	go tool mygo dev

build: ## package the app for this machine into build/
	go tool mygo build

test:
	go test ./...

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
