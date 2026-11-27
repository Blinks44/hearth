# Product Invariants

Non-negotiable product rules backed by automated tests. Every change must preserve these guarantees.

## Safety & Operations
- Discovery is read-only; no system mutations happen outside confirmed `ops.Plan` execution - test: `internal/scan/scan_test.go`, `internal/ops/ops_test.go`
- External commands run via explicit `argv` slices without shell strings, with timeouts and output caps - test: `internal/run/run_test.go`
- Shell rc edits write only inside `# >>> hearth >>>` / `# <<< hearth <<<` markers with timestamped backups; lines outside the block are commented out rather than deleted, and backups are restored on failed verification - test: `internal/ops/edit_test.go`, `internal/ops/pathfix_unix_test.go`
- Verification after an operation checks the environment of a fresh terminal, not the GUI process - test: `internal/scan/scan_test.go` (`TestDetectReadsNewTerminal`)
- After any completed operation, Hearth rescans and invalidates stale disk-size counts - test: `internal/ui/operation_test.go` (`TestOperationInvalidatesSizes`), `internal/disk/disk_test.go` (`TestCountProgress`)
- A failing step in a multi-tool setup plan rolls back only that tool's file edits - test: `internal/ops/edit_test.go` (`TestExecuteKeepsPartsDone`)
- Elevation (`Step.Admin`) is never silent; plans explicitly warn before prompting via OS mechanisms - test: `internal/ops/elevate_test.go`

## Secrets, Privacy & MCP
- Plaintext secrets in MCP configs/settings are detected by key name and value shape; environment variable references (`${VAR}`, `$VAR`, `${env:VAR}`, `{env:VAR}`, `%VAR%`) are not flagged; only names enter snapshots - test: `internal/ai/secrets_test.go`, `internal/issues/issues_test.go` (`TestPlainSecrets`)
- "Move to env var" never overwrites an existing terminal variable with a different value, hides secret values from plans and logs, and restores backups on failure - test: `internal/ops/secret_unix_test.go`
- Hearth's built-in MCP server (`hearth mcp`) never mutates state on read tools and excludes env values, MCP commands/args/URLs, instruction/hook bodies, raw errors, and session content - test: `internal/mcpserver/server_test.go`, `internal/mcpserver/source_test.go`
- MCP `propose_fix` accepts only an `issue_id`, rejects requests when the window is closed or busy, and never executes without human confirmation in the dialog - test: `internal/proposals/bridge_test.go`, `internal/mcpserver/proposal_test.go`, `internal/ui/mcp_proposal_test.go`
- Usage analytics read only session metadata (folder, model, timestamp, token counts) and never store conversation text - test: `internal/ai/usage_test.go`
- Agent Radar and `hearth notify` accept only strict lifecycle metadata (`agent`, `event`, `cwd`, `session_id`, `pid`, `timestamp`), never read, transmit, or store prompts or chat transcripts, and install/remove agent lifecycle hooks only via confirmed `FileEdit` plans with backup - test: `internal/proposals/events_test.go`, `notify_test.go`, `internal/ops/agent_hooks_test.go`, `internal/ai/radar_test.go`, `internal/ui/stage3_test.go`
- Listening process command-line arguments pass through secret redaction before entering snapshots or UI classification - test: `internal/inventory/devserver_test.go`, `internal/detect/databases_test.go`

## Environment, PATH & Projects
- `PATH` fix banners hide "Fix..." when no safe automatic plan exists (`ErrNoFix`) - test: `internal/ui/path_test.go` (`TestPathBannerHidesFixOnErrNoFix`)
- Reordering a directory first in `bash` writes to both `.bashrc` and the active login file so `.profile` does not override it - test: `internal/ops/pathfirst_unix_test.go` (`TestPathFirstBashLogin`)
- Shell startup profiling is read-only and never edits rc files; `nvm` -> `mise` migration comments out only `nvm` loader lines and preserves `~/.nvm` versions - test: `internal/sysenv/startup_test.go`, `internal/ops/nvm_unix_test.go`
- OS-provided runtimes (`NeededBySystem`, e.g., macOS system Python/Ruby) and Store stubs are never treated as removable duplicates; stubs are never executed - test: `internal/issues/issues_test.go`, `internal/ui/setup_test.go` (`TestSetupHaveSystemCopy`)
- Parent `global.json` applies only to directories containing a `.NET` project/solution file, avoiding false alerts in sibling language directories - test: `internal/projects/dotnet_test.go`
- Corepack `packageManager` versions not yet cached resolve as downloadable rather than missing-tool errors - test: `internal/projects/resolve_test.go` (`TestResolveCorepack`)
- Hearth stops only non-system processes owned by the current user, using PIDs captured from the scan and after plan confirmation - test: `internal/ops/process_test.go`, `internal/ui/ports_test.go`
- Managed project scripts run via explicit `argv` (no shell interpolation), stream bounded logs, and tear down their full process group on stop or app exit - test: `internal/projects/scripts_test.go`, `internal/ops/runner_test.go`, `internal/ui/stage3_test.go`

## Docker & Databases
- Docker discovery and live views pin the selected local socket via `--host` with empty `DOCKER_CONTEXT`, poll only while open, and never connect to remote contexts - test: `internal/docker/docker_test.go`, `internal/ui/docker_engine_test.go`
- Container Inspect masks `Config.Env`; Env reveals at most one value for 30 s and clears on close - test: `internal/docker/detail_test.go`, `internal/ui/docker_detail_test.go`
- Normal container stops (`SIGTERM` 143, `SIGINT` 130, `SIGKILL` 137 without `OOMKilled`) and stale `unhealthy` states on stopped containers are not flagged as crashes - test: `internal/docker/containers_test.go`
- Image, volume, and network removals verify targets before running, block removal when attached even to stopped containers, and protect `bridge`/`host`/`none` networks - test: `internal/ops/docker_images_test.go`, `internal/ops/docker_resources_test.go`
- Colima install plans preserve existing Docker settings with backup, install standalone Docker CLI if needed, and on Linux Hearth prefers native distro Docker Engine - test: `internal/ops/colima_test.go`
- Linux native Docker Engine service controls stop `docker.socket` alongside `docker.service` so socket activation does not restart it, and toggling autostart never passes `--now` - test: `internal/detect/docker_linux_test.go`, `internal/ops/docker_linux_test.go`, `internal/ui/docker_linux_test.go`
- Removing a Docker context blocks the active and built-in `default` contexts; removing a broken plugin link verifies in `Before` that the target is still a dangling symlink inside `cli-plugins` - test: `internal/ops/docker_test.go`, `internal/ops/docker_plugins_test.go`, `internal/ui/docker_test.go`, `internal/ui/docker_health_test.go`
- Linking `/var/run/docker.sock` on macOS installs a one-shot `LaunchDaemon` (`dev.hearth.docker-socket`) in the same elevated step so macOS restores the symlink after `/private/var/run` is cleared on reboot - test: `internal/ops/docker_socket_darwin_test.go`, `internal/ops/docker_socket_unix_test.go`
- Database connection strings never contain passwords; MySQL and MariaDB are never offered side-by-side; toggling autostart does not start or stop a running service - test: `internal/ui/databases_test.go`, `internal/ops/service_test.go`

## Kubernetes
- Scanning and opening the Kubernetes page never contact remote clusters; access checks and live cluster polling run only on explicit user action and stop when leaving the view - test: `internal/ui/kube_test.go`, `internal/ui/kube_cluster_test.go`, `internal/kube/kube_test.go`
- Switching context/namespace inside the cluster viewer never mutates `kubectl` or `kubeconfig`; changing the active `kubectl` or Docker context in the main window requires a confirmed plan, while the system tray menu and flyout expose explicit 1-click context switches using the same validated plan commands - test: `internal/ui/kube_cluster_test.go`, `internal/ops/kube_test.go`, `tray_test.go`
- Kubernetes Secrets fetch metadata and key sizes only; revealing a single value fetches by UID and hides after 30 s; Secret objects cannot be opened in the YAML editor - test: `internal/kube/secrets_test.go`, `internal/ui/kube_secrets_test.go`, `internal/ops/kube_resource_test.go`
- Kubernetes writes (edit, delete, scale, restart, pause, rollback, drain, apply, namespace delete, Helm rollback/delete) run server-side dry-run and enforce UID / `resourceVersion` preconditions - test: `internal/ops/kube_resource_test.go`, `internal/ops/kube_scale_test.go`, `internal/ops/kube_node_test.go`, `internal/ops/kube_apply_test.go`, `internal/ops/kube_helm_test.go`
- Port-forwarding binds strictly to `127.0.0.1`, tracks pod UID, and terminates on window close or app exit - test: `internal/kube/forward_test.go`, `internal/ops/forward_test.go`
- Creating a kubeconfig file refuses to overwrite existing files, writes private permissions (`0600` file, `0700` parent directory), masks embedded credentials in the preview diff, and never writes file contents to the operation journal; deleting a context runs via a confirmed plan (`kubectl config delete-context`, unsetting `current-context` when deleting the active context) - test: `internal/ops/kube_test.go`, `internal/ops/edit_test.go`, `internal/ui/kube_test.go`

## AI Skills & Instructions
- Installing a skill writes only the previewed snapshot bytes through `os.Root`, enforces archive size/path safety, never overwrites existing targets, and rolls back created files on failure - test: `internal/ops/skill_install_test.go`, `internal/skillcatalog/catalog_test.go`
- Shared skills live in `.agents/skills` for the matching scope, with symlinks in agent-specific folders; diverging copies are never overwritten - test: `internal/ops/skill_test.go`, `internal/ops/skill_install_test.go`
- Global instruction setup creates new files (`O_EXCL`) or file symlinks without overwriting existing files; on Windows symlink-permission errors, Hearth offers a separate confirmed copy plan - test: `internal/ops/instructions_test.go`, `internal/ui/instructions_test.go`

## Packaging, Updates & UI
- Bundled third-party licenses ship in `resources/NOTICE.txt` and are viewable in Settings -> About - test: `internal/ui/settings_test.go` (`TestSettingsPage`)
- Cleanup never allows selecting items that have no safe removal plan; pnpm store size is measured accurately and deleted as a whole - test: `internal/ui/cleanup_test.go`, `internal/disk/disk_test.go`
- Package-manager updates and installs never upgrade everything blindly; APT updates package lists in the same elevated command - test: `internal/ops/package_test.go`, `internal/ui/updates_test.go`
- "Needs attention", the Overview hero, and sidebar badge counts agree and include only warnings/errors or issues with an automatic fix - test: `internal/ui/overview_test.go`, `internal/ui/problems_test.go`
