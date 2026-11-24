# Contributing to Hearth

Thanks for your interest in contributing to Hearth!

## Development Setup

### Prerequisites
- **Go 1.27.3+** (pinned in `go.mod`)
- macOS, Windows 10+, or Linux with a desktop session (GTK 3 on Linux; no cgo required for builds)
- Optional: `make` (on Windows you can run the `go tool` commands directly)

### Clone & Download Dependencies

```sh
git clone https://github.com/Blinks44/hearth.git
cd hearth
go mod download
go -C tools mod download
```

Run the app with live reload:

```sh
make dev            # or: go tool mygo dev
```

To test a specific UI language:

```sh
go tool mygo dev -lang de   # en, de, es, fr, ru, zh-Hans
```

## Making Changes

1. Read [docs/architecture.md](docs/architecture.md) for package boundaries and [docs/domain.md](docs/domain.md) for product invariants.
2. Keep pull requests focused on one bug fix or feature.
3. Follow Hearth's core rules:
   - Discovery stays read-only; mutations go through `internal/ops` with a previewed plan and confirmation.
   - No I/O or process execution on the UI thread; publish async results via `win.Update(...)`.
   - Run external commands via `internal/run` (`argv` + timeout), never shell strings.
   - Put user-facing strings in `locales/en.json` (and update sibling locale files, then run `go tool mygo generate` / `make check` so `locales/catalog_gen.go` stays in sync).
   - Add table-driven unit tests (`internal/...`) or `ui.NewTester` tests (`internal/ui/`) covering the behavior you changed.

## Running Checks

Before opening a pull request, run the full check suite:

```sh
make check
```

This runs:
- `golangci-lint fmt --diff`
- `golangci-lint run`
- `go tool mygo vet .`
- `go tool mygo i18n check`
- `go test -race` across all non-UI packages and `go test ./internal/ui`

To run the desktop end-to-end test locally:

```sh
make e2e
```

If you edit the landing page under `website/`, run its separate check suite:

```sh
pnpm --dir website check
```

## Commit Style & Pull Requests

- Use [Conventional Commits](https://www.conventionalcommits.org/): `feat: ...`, `fix: ...`, `docs: ...`, `chore: ...`, `test: ...`.
- Open a PR against `main`, describe the problem and verification steps clearly, and attach screenshots for UI changes (light and dark mode when relevant).

## Translations

UI strings live in `locales/` with `en.json` as the source of truth. Contributions improving `de.json`, `es.json`, `fr.json`, `ru.json`, `zh-Hans.json`, or adding new locales are very welcome. Run `go tool mygo i18n check` to verify key parity and placeholder formatting.
