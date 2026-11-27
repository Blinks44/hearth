# Changelog

All notable changes to Hearth are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.0] - 2026-11-27

### Added

- **AI Quotas & Limits:** Read-only subscription and rate-limit tracking across 18 AI coding providers (Claude Code, Codex, Antigravity, Gemini CLI, Cursor, GitHub Copilot, JetBrains AI, Grok, Windsurf, Cline, Kimi, OpenCode Go, Kiro, Kilo, Amp, Augment, CodeRabbit, HuggingFace) with canonical window ordering (`5h` before `Weekly`), burn-pace markers, inline reset times, adaptive polling, per-provider tray toggles and ordering, and optional low-quota (`<10%`) / reset desktop notifications.
- **Live tray control-center popover:** Native menu bar / system tray popover showing running dev servers, Docker containers, local databases, Kubernetes port-forwards, compact AI subscription quotas, Colima toggle, and 1-click Docker/Kubernetes context switching, with configurable section visibility and order.
- **Managed project script runner & dev-server inspector:** Launch and stop scripts from `package.json`, `Makefile`, `Taskfile.yml`, `justfile`, `mise.toml`, and `composer.json` directly inside Hearth with live streaming logs and clean process-group teardown; inspect listening ports with CPU/RAM metrics, framework classification, and browser-open checks.
- **AI Agent Radar:** Live passive detection of running coding agents plus opt-in lifecycle hooks (`hearth notify`) with desktop notifications when an agent finishes or waits for input.
- **Git clone, private Git skill catalogs & Cleanup 2.0:** Clone repositories directly from Hearth, connect private Git repositories as custom skill catalogs, and reclaim additional disk caches with clearer grouping and explanations.
- **Expanded MCP server (`hearth mcp`) & environment tools:** Added new read tools (`get_ai_quotas`, `get_instructions`, `get_models`, `get_usage`, etc.), MCP resources (`hearth://summary`, `hearth://projects`, `hearth://issues`, `hearth://ports`), diagnostic prompts, human-in-the-loop proposal tools (`propose_*`), plus global package management, allowlisted environment variable plans (`ANDROID_HOME`, `JAVA_HOME`), and Git credential/SSH helpers.

### Changed

- **Split macOS builds & smaller installed footprint:** Switched macOS releases from a universal DMG (~94 MB installed) to separate Apple Silicon (`arm64`) and Intel (`amd64`) DMGs (~44-48 MB installed), compressed embedded fonts, dropped duplicated resources, and kept a signed universal updater payload so existing installations auto-update smoothly.
- **Cleaner Project Overview & aligned tables:** Arranged secondary project cards (`Docker Compose`, `Databases`, `Kubernetes`, `Disk space`, `AI`, `Files`) into a height-balanced 2-column grid with unified card headers, removed the duplicate `Prepare environment` button from the warning banner, aligned Services & Ports into dedicated `CPU`, `RAM`, and fixed-slot action columns, moved Databases project links into inline chips to prevent URL truncation, synced inline button heights, and softened the Quotas sidebar badge.
- **Broader `Prepare environment` plans:** Added automatic installation of versioned PHP lines (`brew`/`winget`) together with Composer and local `mise.toml` installation for unversioned `mise`-backed project tools without altering global defaults.

### Fixed

- **Colima, Docker & runtime detection:** Improved Colima/Docker detection and setup reliability, fixed Windows SQLite file URI formatting in `aiquota`, stopped flagging pass-through `pyenv`/`rbenv` shims as duplicate installations, and stopped marking the immediately previous Rust stable minor as end-of-life.
- **Website release downloads:** Resolved `/releases/latest` via redirect header before falling back to the GitHub API to avoid unauthenticated rate limits on Cloudflare Workers, and added Apple Silicon / Intel macOS DMG download links.

## [1.0.5] - 2026-11-26

### Added

- **Redesigned Project detail view:** Added tabbed layout (`Overview`, `Services`, `AI`, `Files`), quick header buttons (`Editor`, `Terminal`, `README`), a read-only Git summary strip (branch, modified count, remote link, last commit), reinstallable dependency sizing with a jump to Cleanup, and dedicated project AI cards (skills with per-agent toggles, MCP servers, instructions, permissions, and hooks).
- **Create local skills & project filtering:** Create new `SKILL.md` skills directly from Hearth into shared storage and link them to installed agents, filter skills by project, and distinguish native agent skill folders from symlinks.
- **Bulk issue fixes & severity filtering:** Filter Issues by severity (`All`, `Important only`, `For information`) and apply multiple automatic fixes together in one confirmed plan (`Fix all`).
- **Cleanup settings & reset controls:** Configure inactive project age, minimum cache size, and excluded `$HOME` folders in Settings, clear stale hidden issues, and restore dismissed Overview tips.
- **Broader version pinning:** Added confirmed version-pinning support for `.sdkmanrc`, `global.json`, `.tool-versions`, and additional runtime version files.

### Changed

- **Lower memory usage & faster startup:** Reduced idle RAM usage and scan memory spikes by compacting AI usage caches in memory and on disk, pooling scan buffers, caching usage summaries between frames, sharing one file watcher per startup folder, and returning unused memory to the OS after background scans.
- **Clearer Overview & Updates status:** Split Databases and Ports into separate Overview summary tiles, included Hearth app updates in the Updates badge and counter, and grouped repeated project requirement checks per file.

### Fixed

- **macOS memory attribution for Colima & Podman VMs:** Starting a Colima or Podman VM from Hearth now disclaims macOS process responsibility so Activity Monitor and memory tools attribute the VM's RAM to the VM itself instead of Hearth.
- **Shell startup timing threshold:** Aligned the Startup tab warning threshold with the Issues page so fast shells are no longer flagged inconsistently.

## [1.0.4] - 2026-11-25

### Added

- **Expanded brand icons & context tiles:** Added official brand/logo SVG icons (including dark-theme variants) for container engines (`Colima`, `Podman`, `Rancher Desktop`, `Kiri`), package managers (`Nix`, `Flatpak`, `Snap`, `DNF`, `Pacman`, `Zypper`, `Chocolatey`, `MacPorts`, `WinGet`), and `k3s`, plus `28x28` entity icon tiles in Docker context rows/popovers and Kubernetes context popovers.

### Changed

- **Unified inspector detail cards:** Standardized detail and metadata cards across Docker, Package Managers (metadata, installed apps, related tools), Kubernetes resources (workloads, storage, policies, RBAC access), and Helm Overview onto one bordered key-value card layout with muted labels, selectable values, and inline icon actions.

### Fixed

- **UI icon & tooltip test stability:** Added static verification that all UI icon literals exist in `resources/icons/` and fixed `TestPackageNameTooltip` hover coordinates on Linux.

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
