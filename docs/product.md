# Hearth Product Overview

Hearth is a free, local desktop application for macOS, Windows, and Linux that gives developers one calm place to inspect, maintain, and safely configure their local development environment.

## Core Workflows

1. **Understand what is installed and active:** Distinguish what is on disk, what a new terminal runs (`PATH` order), what GUI apps see, and what each project expects.
2. **Keep projects ready:** Read project pin files statically (`.nvmrc`, `.node-version`, `package.json`, `mise.toml`, `.tool-versions`, `.python-version`, `pyproject.toml`, `go.mod`, `rust-toolchain.toml`, `global.json`, `.ruby-version`, `composer.json`, `*.tf`, Compose, and Kubernetes manifests) and install missing runtimes or tools without silently changing global defaults.
3. **Inspect and control local infrastructure:** Manage local Docker engines (with first-class Colima support on macOS and native Docker Engine on Linux), containers, images, volumes, networks, Kubernetes clusters, local databases, listening TCP ports, and cloud CLIs.
4. **Manage AI coding agents:** Inspect installed agents, shared/per-agent skills, curated skills catalogs, MCP servers, global/project instruction files, plaintext-secret warnings, and local token usage / estimated cost.
5. **Maintain a clean machine:** Surface actionable issues ("Needs attention"), runtime and package-manager updates, shell startup bottlenecks, and safe disk cleanup with transparent explanations.

## Feature Map

### Overview, Projects & New Computer Setup
- **Overview:** Status hero, live environment strip (Docker, Kubernetes, Databases & Ports, AI today, MCP), top issues with one-click fix plans, tool cards, compact project summary, and copyable secret-free diagnostic report.
- **Projects:** Discover folders from common dev directories or add manually; inspect every tool requirement, linked Kubernetes context, project databases, and AI configs; prepare a project with a dependency-aware multi-step plan and post-scan verification.
- **Install / New Computer:** Guided setup on fresh machines and catalog install on configured machines. Defaults to `mise` for languages when no version manager is present, respects existing managers, and supports exporting/importing setup bundles across machines.

### Languages & Runtimes
- **Node.js, Bun, Python, Go, Rust, PHP, Java, Ruby, .NET, C/C++:** Full version tables with source badges (`mise`, `fnm`, `nvm`, `volta`, `uv`, `pyenv`, `rustup`, `rbenv`, `sdkman`, `brew`, `winget`, `scoop`, `apt`, etc.), `which -a` resolution traces, manager cards, global packages/toolchains, and online available-version lists.
- **JS package managers (`npm`, `pnpm`, `Yarn`, Corepack)**, **Composer**, **Git** (whitelisted global config, `gh` hosts, SSH public keys without reading private keys), **Editors** (VS Code, Cursor, Devin Desktop/Windsurf, JetBrains IDEs — CLI `PATH` setup, extension lists, old versions, index caches), and **Mobile** (Xcode runtimes & DerivedData, Android SDK/AVDs, Flutter/pub caches).

### Infrastructure
- **Docker:** Contexts, engines, projects, plugins, and `PATH`. First-class Colima management on macOS (install, standalone CLI, Compose/Buildx plugins, VM resources, disk reclaim, autostart) and native Docker Engine on Linux. Live local engine view: Overview, Containers (grouped by Compose project, live stats, details, masked Env with 30 s single-value reveal, streaming logs, terminal shell), Images (pull, untag, remove), Volumes, and Networks.
- **Kubernetes:** Merged `KUBECONFIG` contexts (no background cluster calls during scan), starter/pasted `~/.kube/config` creation, kubeconfig editor links, context deletion, explicit access checks, local `kind` clusters, project-to-context links, and live cluster view: Overview, Pods (grouped by owner, factual diagnosis, streaming logs, pod/owner logs, shell, `127.0.0.1` port-forward), Deployments (scale, restart, pause/resume, rollback), Jobs/CronJobs, Services, ConfigMaps, Secrets (metadata only; single-key 30 s reveal), PVC/StorageClasses, Nodes (cordon/drain), HPA/PDB/Quotas, Namespaces, CRDs, Helm releases, RBAC `can-i`, and server-side dry-run manifest apply.
- **Databases & Ports:** Local PostgreSQL, MySQL/MariaDB, Redis, MongoDB, SQL Server, and SQLite servers/services/containers, port-conflict detection against Compose files, one-click OS service install/autostart/start/stop, terminal CLI launch, and listening TCP port inspector.
- **CLI & Clouds:** `aws`, `gcloud`, `az`, `terraform`, `tofu`, and `mise` utilities — active profile/project/subscription, explicit identity check, native login launch, and project Terraform/OpenTofu version checks.

### AI Ecosystem
- **Agents:** Claude Code, Codex, Cursor, GitHub Copilot CLI, Gemini CLI, OpenCode, Antigravity CLI, Devin, Grok Build, Kimi Code — versions, configs, hooks/permission summaries.
- **Skills:** Unified view across agents and projects, shared `~/.agents/skills` canonical storage with agent symlinks, duplicate detection/deduplication, and public catalog browser (Anthropic, OpenAI, Vercel, `skills.sh`).
- **MCP:** Aggregated user and project MCP servers across agents, conflict detection, plaintext secret detection with "Move to env var" migration plans, and one-click connection of Hearth's own read-only + `propose_fix` MCP server (`hearth mcp`).
- **Instructions, Local Models & Usage:** Global (`~/.agents/AGENTS.md` sync) and project instruction files; Ollama and LM Studio models (pull, unload, remove, server start/stop); local session token/cost analytics from agent history files.

### System & Maintenance
- **Issues ("Needs attention"):** Actionable problems across `PATH`, version mismatches, Docker, Kubernetes, secrets, and shell startup, with hide/restore support.
- **Updates & Package Managers:** Unified runtime/tool updates and two-level OS package manager workspace (Homebrew, WinGet, Scoop, Chocolatey, APT, DNF, Pacman, Zypper, APK, Snap, Flatpak, Nix): direct entry into the active manager, per-OS Available managers catalog with official install plans, Overview/Packages/Updates/Dependencies/Install views, split-view package inspector, bulk upgrades/removals, safe autoremove of unused dependencies, index refresh, and explicit user-triggered catalog search.
- **Cleanup:** Reclaimable disk space across unused tool versions, package/build caches, Docker disk usage, old editor extensions/caches, simulator runtimes, stale AI history (>90 days), and unused local models (>=1 GB).
- **PATH & Shell Startup:** Terminal vs. GUI `PATH` comparison, source file/line attribution, add/remove/reorder entries with rollback verification, macOS `launchctl` PATH sync, and zsh/bash/fish/PowerShell startup profiling (with `nvm` -> `mise` migration).
- **Tray / Menu Bar & Settings:** Native menu bar / tray with running containers, Colima controls, and quick Docker/Kubernetes context switching; optional macOS Dock icon hiding when closed to menu bar; 6 UI languages (`en`, `de`, `es`, `fr`, `ru`, `zh-Hans`); light/dark/system themes; signed auto-updates.

## Boundaries

Hearth intentionally does **not**:
- Run commands or edit files without showing a plan and waiting for confirmation (except explicit quick context switches from the native tray menu).
- Upload telemetry, require an account, or transmit user code, secrets, or environment variable values.
- Install application dependencies (`node_modules`, `.venv`, Cargo crates) or act as a database query GUI / code editor.
