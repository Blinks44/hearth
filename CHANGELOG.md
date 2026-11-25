# Changelog

All notable changes to Hearth are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.3] - 2026-11-25

### Added

- **Redesigned Package Managers workspace:** Two-level Docker/Kubernetes-style layout with direct entry into the active manager, breadcrumb manager/view switchers (`Cmd+J` / `Ctrl+J`), Overview, Packages, Updates, Dependencies (with safe confirmed autoremove), and Install views, split-view package inspector with metadata and dependency/dependent navigation, full-row hover/click selection, and a floating bulk action bar.
- **Explicit package catalog search:** On-demand search (`Search` / Enter, never on every keystroke) across Homebrew (name + description search with `brew desc` enrichment), APT, DNF, Pacman, Zypper, APK, Flatpak, Snap, WinGet, Scoop, and Chocolatey with exact/prefix prioritization, installed-state matching, and confirmed install plans.
- **Per-OS Available package managers catalog:** Curated safe package managers per OS (macOS: Homebrew, Nix, MacPorts; Windows: WinGet, Scoop, Chocolatey; Linux: Flatpak, Homebrew, Nix) with official install plans where supported.
- **Entity icon tiles across lists:** Added `28x28` brand/category icon tiles across Package Managers, Kubernetes contexts, Docker engines, and npm/pnpm/Yarn rows.

### Changed

- **Unified UI control hierarchy:** Replaced verbose status buttons with green toggles for binary running/active states (Colima, Linux Docker Engine, Ollama, Kubernetes Deployment rollout pause/resume), compact `24x24` icon buttons for universal actions (refresh index, edit Colima resources, open folder, external links, clear cache, download), and full-row clickable navigation rows with right chevrons for cross-section jumps (`Open in Cleanup`, `Databases`, `Details`, `Review`, `Licenses`).

### Fixed

- **macOS Docker socket persistence across reboots:** Persist the `/var/run/docker.sock` symlink created for Colima via a `/Library/LaunchDaemons/dev.hearth.docker-socket.plist` LaunchDaemon so Testcontainers and Docker SDK clients keep working after a macOS restart.

## [1.0.2] - 2026-11-25

### Added

- **Linux Docker Engine service controls:** Manage native `docker.service` on Linux from the Docker page with confirmed `systemd` plans (`pkexec`), including a boot autostart toggle (`systemctl enable/disable` without changing the running state) and Start/Stop controls (stopping `docker.socket` alongside `docker.service` so socket activation does not immediately restart it).
- **Confirmed Docker cleanup actions:** Remove inactive non-`default` Docker contexts (`docker context rm`), delete dangling CLI plugin symlinks in `~/.docker/cli-plugins`, and install `docker-credential-helper` through Homebrew on macOS when `docker-credential-osxkeychain` is missing.

### Fixed

- **Homebrew multi-keg update detection and verification:** Attach formula updates to the linked keg (or the latest installed keg for keg-only formulae) and check upgrades against the latest matching Cellar entry so older uncleaned kegs do not mask newly upgraded versions.

## [1.0.1] - 2026-11-25

### Fixed

- **Aligned row action icons across tables:** Action buttons (Copy, Show in folder, Open source, and the `...` menu) now stay in consistent right-aligned columns in PATH, Packages, Ports, Databases, and Models instead of shifting horizontally when a row has no menu or path.

## [1.0.0] - 2026-11-24

First public release of Hearth for macOS (Universal), Windows (x64 & ARM64), and Linux (x64 & ARM64).

### Added

- **Native cross-platform desktop UI:** Single ~18 MB Go binary built on `mygo` v0.10 (no Electron, no WebView, no cgo), light/dark/system themes, 6 languages (`en`, `de`, `es`, `fr`, `ru`, `zh-Hans`), native menu bar / system tray with quick container/Colima/context actions, and Ed25519-signed auto-updates.
- **Languages, runtimes & project setup:** Detection, `which -a` resolution, version management, and project pin verification for Node.js, Bun, Python, Go, Rust, PHP, Java, Ruby, .NET, C/C++, npm/pnpm/Yarn/Corepack, Composer, Git, Editors (VS Code, Cursor, Devin Desktop/Windsurf, JetBrains), and Mobile (Xcode, Android SDK, Flutter).
- **Safe operation engine ("What will change"):** Preview exact `argv` commands, file diffs, administrator notes, and third-party terms before confirming; marker-scoped shell rc edits (`# >>> hearth >>>`) with timestamped backups, automatic rollback on failed verification, and fresh-terminal verification.
- **Docker management:** Contexts, engines, projects, plugins, first-class Colima setup/VM resource management on macOS, native Docker Engine support on Linux, live socket-pinned engine viewer (Overview, Containers grouped by Compose, Images, Volumes, Networks, streaming logs, shell, masked Env reveal), and `docker events` real-time updates.
- **Kubernetes management:** Merged `KUBECONFIG` inspection without background cluster calls, local `kind` cluster management, project-to-context links, live cluster viewer (Overview, Pods, Deployments, Jobs/CronJobs, Services, ConfigMaps, masked Secrets, PVC/StorageClasses, Nodes cordon/drain, HPA/PDB/Quotas, Namespaces, CRDs, Helm releases, RBAC `can-i`), streaming pod/owner logs, `127.0.0.1` port-forwarding, and server-side dry-run manifest apply.
- **Local databases & ports:** PostgreSQL, MySQL/MariaDB, Redis, MongoDB, SQL Server, and SQLite servers/services/containers, Compose port-conflict detection, OS service install/autostart/start/stop, terminal CLI launch, and TCP port inspector.
- **CLI & clouds:** `aws`, `gcloud`, `az`, `terraform`, `tofu`, and `mise` utilities with active profile/project/subscription inspection, on-demand identity checks, native login launch, and project Terraform/OpenTofu version checks.
- **AI coding agents ecosystem:** Unified inspection for Claude Code, Codex, Cursor, GitHub Copilot CLI, Gemini CLI, OpenCode, Antigravity CLI, Devin, Grok Build, and Kimi Code; shared `~/.agents/skills` canonical storage and public skills catalog (Anthropic, OpenAI, Vercel, `skills.sh`); unified MCP servers with plaintext-secret detection and "Move to env var" migration; `~/.agents/AGENTS.md` sync; Ollama & LM Studio model management; local session token/cost usage analytics.
- **Built-in MCP server (`hearth mcp`):** Headless stdio server with secret-free read tools (`get_tools`, `get_projects`, `get_issues`, `get_agents`) and human-confirmed `propose_fix` via local token-authenticated bridge.
- **Maintenance & New Computer setup:** Actionable "Needs attention" diagnostics, OS package manager management (Homebrew, WinGet, Scoop, Chocolatey, APT, DNF, Pacman, Zypper, APK, Snap, Flatpak, Nix), disk cleanup, shell startup profiler (with `nvm` -> `mise` migration), and guided New Computer setup with exportable bundles.
