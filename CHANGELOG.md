# Changelog

All notable changes to Hearth are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
