# Hearth Architecture

Hearth is a local desktop application written in Go 1.27 using [mygo](https://mygo.egoist.dev) v0.10 (native immediate-mode UI, no cgo, no WebView).

## Design Principles

1. **Read-only by default.** Local discovery never mutates the machine. Optional network checks (version indexes and package-manager metadata updates) require the network toggle to be enabled and never install packages, alter profiles, or accept third-party terms. Every system change runs as an explicit *operation* with a previewed plan, diff, and user confirmation.
2. **Three distinct facts instead of a single "installed" flag:** *installed* (files on disk), *available on PATH* (what a new terminal actually runs, and in what order), and *resolved in a project* (version pin files vs. installed copies). On macOS/Linux, GUI app PATH is tracked separately from terminal PATH.
3. **Local-first, zero telemetry.** No accounts, no Hearth backend, no analytics. Version lists and package update indexes are cached locally so the app stays usable offline.
4. **Respect existing setups.** Hearth detects whichever version or package manager owns a tool (`mise`, `fnm`, `nvm`, `volta`, `pyenv`, `uv`, `rustup`, `rbenv`, `brew`, `winget`, `scoop`, `apt`, `dnf`, `pacman`, etc.) and works through it. Shell rc files are edited only inside `# >>> hearth >>>` / `# <<< hearth <<<` markers with timestamped backups; lines outside the Hearth block are commented out rather than deleted, and only after explicit confirmation.
5. **Fast startup.** The window renders the last cached snapshot immediately while a fresh scan runs in the background.

## Package Layout

```
main.go                  entry point: fonts, theme, window, tray, MCP subcommand dispatch
internal/
  run/                   command execution: argv, ctx, timeout, output cap; Runner interface + test fakes
  sysenv/                terminal login-shell env, GUI env, Windows registry PATH, Go-native which -a, startup profiler
  inventory/             core data model (Tool, Installation, Source, PathEntry, Agent, Snapshot) - pure data, no I/O
  scan/                  parallel background scanner (worker cap = 4), generation-based cancellation
  detect/                tool, runtime, database, Docker, Kubernetes, CLI, editor, mobile, and model detectors
  sources/               installation provenance classifier by real path (brew, mise, fnm, scoop, winget, apt, ...)
  projects/              project version-pin parsers (.nvmrc, mise.toml, go.mod, global.json, pyproject.toml, *.tf, ...)
  ai/                    AI agents, skills, MCP server declarations, global/project instructions, session usage & cost
  pkgmgr/                OS package managers (Homebrew, WinGet, Scoop, Chocolatey, APT, DNF, Pacman, Zypper, APK, Snap, Flatpak, Nix)
  versions/              online version indexes (nodejs.org, go.dev, endoflife.date, npm registry) with disk cache
  updates/               available runtime and tool updates computed over Snapshot + projects + version indexes (no I/O)
  issues/                diagnostic rule engine ("Needs attention") over Snapshot and projects (no I/O; UI formats copy)
  cleanup/               reclaimable disk candidates with reasons over Snapshot, sizes, and projects (no I/O)
  disk/                  background disk-usage calculator (tool copies, caches, package sizes, docker system df)
  docker/                live local Docker engine viewer pinned to a local socket (--host), polled only while open
  kube/                  live Kubernetes viewer (kubectl + client-go JSONPath + Helm), polled only while open
  containerlog/          shared streaming log buffer, filtering, and export for Docker and Kubernetes
  catalog/               "New Computer" / Install catalog: per-OS install methods, dependencies, and conflict rules
  ops/                   confirmed operations: Plan -> confirmation -> Execute with streaming log -> Verify -> rollback
  mcpserver/             headless stdio MCP server (`hearth mcp`) built on github.com/modelcontextprotocol/go-sdk
  proposals/             localhost loopback bridge (127.0.0.1 + random token) for MCP `propose_fix` requests
  skillcatalog/          explicit public GitHub archive reader for curated skills catalogs with size/path guards
  history/               snapshot diffs and recent scan history (no I/O)
  report/                secret-redacted diagnostic text report (no I/O)
  store/                 JSON settings, projects, snapshot/size/history caches (`PathUserData` / `PathCache`)
  watch/                 file watchers (projects, agent dirs, shell rc) and `docker events` live refresh
  ui/                    mygo views, shell, navigation, pages, dialogs, and reusable components
locales/                 en.json (source), de.json, es.json, fr.json, ru.json, zh-Hans.json, catalog_gen.go
resources/               app & tray icons, embedded fonts (Inter, JetBrains Mono), SVG icons, NOTICE.txt
e2e/                     end-to-end test driving the built app via mygo's `automation` plugin (`-tags e2e`)
website/                 landing page (TanStack Start + ModUIX on Cloudflare Workers), isolated CI
```

Dependencies flow downward: `ui -> (inventory, ops, store, ...)`, `detect/ai/projects -> (run, sysenv, sources, inventory)`. Pure packages (`inventory`, `issues`, `updates`, `cleanup`, `history`, `report`, `catalog`) perform no I/O. The UI thread never performs I/O or spawns processes; background results are published exclusively through `win.Update(...)`.

## Scanning & Threading

- Scans run on launch, on manual refresh, and on watched file changes (`mygo.WatchFiles` over project roots, `~/.claude`, `~/.codex`, and shell startup files).
- When a local Docker engine responds to a scan, `watch.Service.Docker` subscribes to `docker events` (debounced at 300 ms) and refreshes only the `docker` and `databases` detectors without a full scan.
- Detectors run concurrently (semaphore of 4) with per-command timeouts (default 5 s) and output caps. One failing detector never aborts a scan; that tool reports an error state with its reason.
- Starting a new scan cancels any in-flight scan and discards stale results via generation counters. Snapshots persist to `PathCache/snapshot.json`.
- Views are pure and side-effect-free; mygo may evaluate view functions multiple times per frame.

## Terminal vs. GUI Environment

GUI applications launched from the Dock, Start Menu, or desktop launcher do not inherit a user's interactive terminal `PATH`:

- **macOS / Linux:** Hearth runs the user's login interactive shell (`$SHELL -ilc`) in its own session (`Setsid`) without stdin, capturing `/usr/bin/env -0` between random markers (surviving multi-line values and shell banner noise).
- **Windows:** Hearth builds the new-session environment via `CreateEnvironmentBlock` (`golang.org/x/sys/windows`) and resolves real paths via `GetFinalPathNameByHandle` (handling NTFS junctions used by Scoop and `mise`), while treating `WindowsApps` App Execution Aliases without a backing package as stubs rather than real binaries.
- `which -a` is implemented directly in Go against the captured `PATH` (respecting `PATHEXT` on Windows) instead of shelling out to `which`.

## Operations & Safety

Every mutating action is represented by an `ops.Plan` (`Title`, `Why`, `Action`, `Steps`, `Notes`, `Changes`, `Verify`):

- **Preview before execution:** `ui.App.Propose` displays exact command argv, file diffs (`ui.DiffViewer`), administrator/license notes, and consequences before anything runs.
- **No shell-string execution:** External commands are always passed as explicit `argv` slices through `internal/run`.
- **Elevation:** `Step.Admin` uses native OS prompts (`osascript ... with administrator privileges` on macOS, `pkexec` on Linux, UAC on Windows) and is always flagged in the plan beforehand.
- **Reversible edits:** Shell rc files are updated inside `# >>> hearth >>>` / `# <<< hearth <<<` markers with timestamped backups; failed verification restores the backup automatically.
- **Live cluster & engine guardrails:** Kubernetes writes use server-side dry-run and bind to object UID and `resourceVersion`; Secret values are never stored in snapshots or logs and are revealed one key at a time for 30 s. Docker mutations pin the local socket (`--host`), verify full container/image IDs before execution, and protect running containers, attached volumes, and system networks (`bridge`, `host`, `none`).

## Built-in MCP Server (`hearth mcp`)

Running `hearth mcp` starts a headless stdio MCP server (`github.com/modelcontextprotocol/go-sdk`) before any window or font initialization:

- **Read tools:** `get_tools`, `get_projects`, `get_issues`, `get_agents`. Responses read the latest cached snapshot (or run a fresh scan on `refresh: true`) and return strict field projections that exclude environment variable values, MCP commands/args/URLs, instruction/hook bodies, session transcripts, and raw detector errors.
- **Human-in-the-loop fix requests (`propose_fix`):** Accepts only an `issue_id` (SHA-256 of the issue target) and sends it to the running Hearth window over a local `127.0.0.1` bridge protected by a per-session `0600` token (`mcp-bridge.json`). The window rebuilds the standard fix plan, marks it as requested by an AI agent, and waits for explicit human confirmation. Arbitrary commands, diffs, or auto-approvals are structurally impossible.

## Testing Strategy

| Layer | Approach |
| --- | --- |
| `sources`, `projects`, `versions`, `issues`, `cleanup`, `updates` | Table-driven stdlib tests with fixtures in `testdata/` |
| Detectors & operations | Fake `run.Runner` + temporary filesystem roots (`t.TempDir()`) |
| Real shell / OS integration | `//go:build integration` tests |
| UI views & interactions | `ui.NewTester` over fake snapshots (including layout bounds and narrow-window regressions) |
| Full app binary | `e2e/` (`-tags e2e`) using mygo's `automation` plugin on an isolated `HOME` |
