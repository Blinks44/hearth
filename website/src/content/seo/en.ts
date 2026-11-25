import type { SeoContentBundle } from "./types";

export const enSeoContent: SeoContentBundle = {
  alternatives: {
    "docker-desktop": {
      slug: "docker-desktop",
      path: "/alternatives/docker-desktop/",
      crumbLabel: "Docker Desktop",
      metaTitle: "Free Docker Desktop Alternative (Colima & Native Go) · Hearth",
      metaDescription:
        "Replace Docker Desktop with open-source Colima on macOS or native Docker Engine on Linux inside Hearth, a ~18 MB MIT native Go app with zero Electron bloat.",
      heroTitle: "A free, native Docker Desktop alternative without Electron.",
      heroSubtitle:
        "Run open-source Colima on macOS or native Docker Engine on Linux. Inspect Compose stacks, containers, images, volumes, and live logs in one ~18 MB native Go app with zero Electron bloat.",
      metrics: [
        {
          value: "~18 MB binary",
          label: "Native Go & mygo UI · <80 ms cold start · zero Electron",
        },
        {
          value: "MIT Free",
          label: "No commercial seat fees, no sign-in, zero telemetry",
        },
        {
          value: "Colima + Linux",
          label: "Open-source VM on macOS, native Engine on Linux, plus K8s & DBs",
        },
      ],
      hearthColumnLabel: "Hearth (Colima / Linux Engine)",
      otherColumnLabel: "Docker Desktop",
      rows: [
        {
          criterion: "License & pricing",
          hearth: "Free & MIT open source for personal and commercial teams",
          other: "Paid seats required for companies above threshold",
        },
        {
          criterion: "UI footprint & speed",
          hearth: "~18 MB native Go binary (mygo), <80 ms cold start",
          other: "Electron desktop UI, often 1 GB+ idle memory",
        },
        {
          criterion: "macOS container VM",
          hearth: "One-click open-source Colima with CPU/RAM/disk controls",
          other: "Proprietary VM bundled inside Docker Desktop",
        },
        {
          criterion: "Linux workflow",
          hearth: "Connects directly to native Docker Engine (no extra VM)",
          other: "Runs an extra VM layer on Linux",
        },
        {
          criterion: "Beyond Docker",
          hearth: "Includes Kubernetes UI, local DBs, runtimes, AI/MCP & cleanup",
          other: "Docker & built-in single-node K8s only",
        },
        {
          criterion: "Privacy & telemetry",
          hearth: "Zero telemetry, no account required, 100% local",
          other: "Account prompts & usage telemetry enabled by default",
        },
      ],
      chooseHearth: {
        pill: "Recommended for local dev",
        title: "Choose Hearth when…",
        points: [
          "You want a free MIT alternative without commercial seat limits",
          "You want Colima on macOS or native Docker Engine on Linux with a fast UI",
          "You want Containers, Kubernetes, local DBs & AI agents in one ~18 MB app",
        ],
      },
      keepOther: {
        pill: "When Docker Desktop still fits",
        title: "Stick with Docker Desktop when…",
        points: [
          "Your enterprise mandates Docker Business SSO & image policies",
          "You rely on Docker Extensions marketplace plugins",
          "You need Docker Scout or Docker Build Cloud tightly integrated",
        ],
      },
      steps: [
        {
          step: "Step 01",
          title: "Install Hearth (zero background changes)",
          desc: "Download the ~18 MB native binary via DMG, Homebrew, or .deb. First scan is 100% read-only and never touches your containers.",
        },
        {
          step: "Step 02",
          title: "Set up Colima or connect Linux Engine",
          desc: "Open Docker in Hearth and review the explicit install plan: Colima, Docker CLI, Compose & Buildx with CPU/RAM controls.",
        },
        {
          step: "Step 03",
          title: "Stop Docker Desktop & reclaim GBs",
          desc: "Verify your Compose stacks and contexts in Hearth, then safely reclaim unused build caches and old VM disk space.",
        },
      ],
    },
    "lens-freelens": {
      slug: "lens-freelens",
      path: "/alternatives/lens-freelens/",
      crumbLabel: "Lens & Freelens",
      metaTitle: "Fast Native Lens, Freelens & Headlamp Alternative · Hearth",
      metaDescription:
        "Inspect Kubernetes pods, logs, crash diagnostics, port-forwards, Secrets, Helm releases, and kind clusters in a ~18 MB native Go client-go UI with zero Electron.",
      heroTitle: "A fast native Kubernetes UI without Electron lag.",
      heroSubtitle:
        "Built directly on Go client-go. Inspect pods, crash causes, live owner logs, port-forwards, Helm releases, Secrets, and local kind clusters in one ~18 MB native app instead of a 1 GB Electron window.",
      metrics: [
        {
          value: "Direct client-go",
          label: "Reads kubeconfig natively with <80 ms cold start",
        },
        {
          value: "Zero sign-in",
          label: "No Lens ID, no cloud sync, no telemetry, MIT open source",
        },
        {
          value: "Full local stack",
          label: "Kubernetes alongside Docker, local DBs, runtimes & AI tools",
        },
      ],
      hearthColumnLabel: "Hearth (Native client-go)",
      otherColumnLabel: "Lens / Freelens / Headlamp",
      rows: [
        {
          criterion: "Runtime & memory",
          hearth: "~18 MB native Go binary (mygo), tiny idle RAM",
          other: "Electron / Chromium desktop app (Lens, Freelens) or browser UI",
        },
        {
          criterion: "Account & licensing",
          hearth: "MIT open source, zero account or login walls",
          other: "Lens requires account & paid tiers; Freelens/Headlamp are OSS",
        },
        {
          criterion: "Crash & pod debugging",
          hearth: "Crash diagnosis, events, live pod/owner logs & exec shell",
          other: "Pod logs, events, shell & multi-panel views",
        },
        {
          criterion: "Helm, Secrets & port-forward",
          hearth: "Built-in port-forward manager, Secrets reveal & Helm list",
          other: "Available via built-in views or community extensions",
        },
        {
          criterion: "Local kind clusters",
          hearth: "Create, start, stop & switch local kind clusters in-app",
          other: "Inspects existing clusters; separate CLI needed for creation",
        },
        {
          criterion: "Safety & mutating ops",
          hearth: "Explicit plan preview before restarts, deletes, or scaling",
          other: "Immediate actions or simple confirmation modal",
        },
      ],
      chooseHearth: {
        pill: "Recommended for daily dev",
        title: "Choose Hearth when…",
        points: [
          "You want instant startup and low RAM when switching between local & staging clusters",
          "You need pods, crash diagnostics, live logs, port-forward, Helm & kind in one place",
          "You want to retire separate Docker, Kubernetes, and local database UIs",
        ],
      },
      keepOther: {
        pill: "When Lens / Headlamp still fit",
        title: "Stick with Lens or Headlamp when…",
        points: [
          "You depend on custom Lens or Headlamp plugin ecosystems",
          "You need built-in Prometheus metric charts across large production fleets",
          "You prefer a web-hosted in-cluster dashboard for platform teams",
        ],
      },
      steps: [
        {
          step: "Step 01",
          title: "Open Hearth & detect ~/.kube/config",
          desc: "Hearth discovers all kubeconfig contexts read-only without modifying active context or sending data anywhere.",
        },
        {
          step: "Step 02",
          title: "Inspect pods, logs & port-forwards",
          desc: "Switch namespaces, jump straight into CrashLoop diagnostics, stream owner logs, or forward local ports in one click.",
        },
        {
          step: "Step 03",
          title: "Spin up local kind clusters when needed",
          desc: "Create or clean up local kind clusters alongside your Colima / Docker Engine containers without leaving Hearth.",
        },
      ],
    },
    dbngin: {
      slug: "dbngin",
      path: "/alternatives/dbngin/",
      crumbLabel: "DBngin",
      metaTitle: "Free Cross-Platform DBngin Alternative (macOS, Windows, Linux) · Hearth",
      metaDescription:
        "Run local PostgreSQL, Redis, MySQL, MariaDB, and MongoDB on macOS, Windows, and Linux with port conflict detection and autostart controls in Hearth.",
      heroTitle: "Local databases & ports on macOS, Windows, and Linux.",
      heroSubtitle:
        "Install, start, and stop PostgreSQL, Redis, MySQL, MariaDB, and MongoDB natively or via Docker. Spot port conflicts, toggle autostart, and open psql or redis-cli without a macOS-only menu bar app.",
      metrics: [
        {
          value: "3 platforms",
          label: "macOS, Windows & Linux (unlike macOS-only DBngin)",
        },
        {
          value: "5 DB engines",
          label: "PostgreSQL, Redis, MySQL, MariaDB & MongoDB local or Docker",
        },
        {
          value: "Port safety",
          label: "Live port-conflict detection with process details & quick CLI",
        },
      ],
      hearthColumnLabel: "Hearth (Databases & Ports)",
      otherColumnLabel: "DBngin / Postgres.app",
      rows: [
        {
          criterion: "Operating systems",
          hearth: "macOS, Windows & Linux from one consistent UI",
          other: "macOS only (DBngin & Postgres.app)",
        },
        {
          criterion: "Supported engines",
          hearth: "PostgreSQL, Redis, MySQL, MariaDB & MongoDB",
          other: "PostgreSQL, MySQL & Redis (DBngin) or Postgres only",
        },
        {
          criterion: "Native & Docker services",
          hearth: "Manages Homebrew/system services and Docker DB containers together",
          other: "Managed standalone binaries only",
        },
        {
          criterion: "Port conflict detection",
          hearth: "Shows which process holds 5432, 6379, or 3306 before start fails",
          other: "Manual port selection without system-wide conflict view",
        },
        {
          criterion: "License & source",
          hearth: "100% free & MIT open source",
          other: "Freeware / closed source (DBngin)",
        },
        {
          criterion: "SQL query editor",
          hearth: "Not included — pairs cleanly with TablePlus, DataGrip & psql",
          other: "Not included — pairs with TablePlus or psql",
        },
      ],
      chooseHearth: {
        pill: "Recommended for local DBs",
        title: "Choose Hearth when…",
        points: [
          "You work across macOS, Windows, or Linux and want one open-source tool",
          "You run both Homebrew/native services and Docker Compose databases",
          "You want port-conflict alerts plus quick CLI launch alongside your rest of stack",
        ],
      },
      keepOther: {
        pill: "When DBngin / SQL GUIs still fit",
        title: "Keep DBngin or TablePlus when…",
        points: [
          "You need isolated side-by-side minor DB binary builds on macOS without Docker",
          "You need a full visual SQL query editor, table browser, or ER diagram (keep TablePlus or DataGrip)",
          "You rely on existing DBngin-managed data directories you do not want to migrate yet",
        ],
      },
      steps: [
        {
          step: "Step 01",
          title: "Scan running services & ports",
          desc: "Hearth detects existing Homebrew, system, and Docker databases automatically without restarting or touching data directories.",
        },
        {
          step: "Step 02",
          title: "Install or toggle autostart cleanly",
          desc: "Add PostgreSQL, Redis, MySQL, MariaDB, or MongoDB from an explicit install plan and control start-at-login per service.",
        },
        {
          step: "Step 03",
          title: "Connect your SQL client or CLI",
          desc: "Copy connection details, launch psql/redis-cli/mongosh directly, and keep using TablePlus or DataGrip for queries.",
        },
      ],
    },
  },
  guides: {
    "dev-environment-setup": {
      slug: "dev-environment-setup",
      path: "/guides/dev-environment-setup/",
      crumbLabel: "Dev Environment Setup",
      metaTitle: "Dev Environment Setup Guide for Beginners & Vibe Coding · Hearth",
      metaDescription:
        "Set up Node.js, Python, Git, Docker, PostgreSQL, and Claude Code or Cursor on macOS, Windows, or Linux in minutes without breaking PATH.",
      heroTitle: "Set up a clean dev environment in minutes, without breaking PATH.",
      heroSubtitle:
        "Starting your first project with Claude Code or Cursor, or onboarding a brand-new Mac, Windows, or Linux machine? Hearth installs runtimes, Docker, databases, and AI tools from one transparent plan.",
      secondaryCtaLabel: "Also read: AI Agents & MCP guide",
      secondaryCtaTo: "/guides/ai-agents-mcp/",
      presetsHeading: "Pick a ready-made preset or build your own",
      presets: [
        {
          pill: "Beginners & Vibe Coding",
          pillTone: "accent",
          title: "Vibe Coding Starter",
          desc: "Go from a fresh laptop to building with AI agents in 10 minutes.",
          stackLines: [
            "Git + GitHub CLI · mise",
            "Node.js 22 LTS · Python 3.13 (uv)",
            "Claude Code · Cursor · shared AGENTS.md",
          ],
          foot: "Zero manual ~/.zshrc editing",
        },
        {
          pill: "Web & Product",
          pillTone: "info",
          title: "Full-Stack Web",
          desc: "Everything needed to clone modern TypeScript & Python repos.",
          stackLines: [
            "Homebrew/WinGet · mise",
            "Node 22 · Bun · pnpm · Python 3.13",
            "Docker + Colima · PostgreSQL · Redis",
          ],
          foot: "Respects .nvmrc & mise.toml",
        },
        {
          pill: "Backend & Infra",
          pillTone: "success",
          title: "Backend & Cloud",
          desc: "For APIs, containers, and local Kubernetes workflows.",
          stackLines: [
            "mise · Go 1.27 · Rust",
            "Docker + Colima · kubectl · kind",
            "PostgreSQL 16 · Redis · Terraform",
          ],
          foot: "Exportable as hearth-setup.json",
        },
      ],
      stepsHeading: "How guided setup works",
      steps: [
        {
          step: "01 · Select",
          title: "Choose your stack",
          desc: "Pick a Vibe Coding, Web, or Backend preset, or import a shared JSON preset from your team.",
        },
        {
          step: "02 · Plan",
          title: "Review exact diff first",
          desc: "See every command and shell-block change before anything runs. Automatic .bak backups included.",
        },
        {
          step: "03 · Install",
          title: "Run with live logs",
          desc: "Installs through Homebrew, WinGet, mise & Colima in dependency order with rollback on failure.",
        },
        {
          step: "04 · Verify",
          title: "Check in a fresh shell",
          desc: "Hearth opens a clean login shell to confirm every binary resolves via the right PATH order.",
        },
      ],
      pitfallsHeading: "Common setup mistakes Hearth prevents",
      pitfalls: [
        {
          title: "Broken PATH & slow zsh startup",
          desc: "Mixing Homebrew node, nvm, and pyenv causes version conflicts and 1s+ terminal lag. Hearth defaults to fast mise and traces which -a.",
        },
        {
          title: "Plaintext API keys in MCP configs",
          desc: "When connecting AI agents to tools, secrets often end up hardcoded in JSON. Hearth flags them and offers a safe env-var migration plan.",
        },
        {
          title: "Silent config overwrites",
          desc: "Random install scripts append to ~/.zshrc without backups. Hearth edits only its own marker block and keeps timestamped .bak copies.",
        },
      ],
    },
    "ai-agents-mcp": {
      slug: "ai-agents-mcp",
      path: "/guides/ai-agents-mcp/",
      crumbLabel: "AI Agents & MCP",
      metaTitle: "Manage Claude Code, Codex, Cursor, AGENTS.md & MCP in One Place · Hearth",
      metaDescription:
        "Unify AGENTS.md across AI coding agents, share skills, probe MCP servers live, catch plaintext secrets, and track local token spend in Hearth.",
      heroTitle: "Unify AGENTS.md, skills, MCP servers, and local AI spend.",
      heroSubtitle:
        "Stop juggling five hidden dotfiles across Claude Code, Codex CLI, Cursor, GitHub Copilot, and Gemini CLI. Hearth keeps instructions synced, probes MCP servers live, and tracks local token spend privately.",
      secondaryCtaLabel: "Also read: Dev Environment Setup guide",
      secondaryCtaTo: "/guides/dev-environment-setup/",
      presetsHeading: "Three AI workflows Hearth simplifies",
      presets: [
        {
          pill: "Shared Instructions",
          pillTone: "accent",
          title: "One ~/.agents/AGENTS.md",
          desc: "Edit your global rules once and symlink them safely to every installed agent.",
          stackLines: [
            "~/.agents/AGENTS.md (single source)",
            "CLAUDE.md · CODEX · Gemini · Cursor",
            "Side-by-side diff before linking",
          ],
          foot: "Automatic .bak before symlinking",
        },
        {
          pill: "MCP Control Plane",
          pillTone: "info",
          title: "Live MCP Probe & Built-in Server",
          desc: "Verify every stdio/HTTP MCP server and expose Hearth read-only diagnostics to your agent.",
          stackLines: [
            "hearth mcp (local stdio server)",
            "Live handshake & tool count probe",
            "Plaintext API key secret alerts",
          ],
          foot: "Human approval required for all fixes",
        },
        {
          pill: "Skills & Spend",
          pillTone: "success",
          title: "Shared Skills & Token Ledger",
          desc: "Install skills from GitHub catalogs and see 7/30/90-day local token and cost estimates.",
          stackLines: [
            "~/.agents/skills shared catalog",
            "Diverging copy diff & sync",
            "Local JSONL usage & cost breakdown",
          ],
          foot: "100% local · zero cloud upload",
        },
      ],
      stepsHeading: "How Hearth organizes your AI setup",
      steps: [
        {
          step: "01 · Discover",
          title: "Find every agent & config",
          desc: "Read-only scan locates Claude Code, Codex, Cursor, Copilot, Gemini, and OpenCode configs automatically.",
        },
        {
          step: "02 · Unify",
          title: "Sync AGENTS.md & skills",
          desc: "Review diverging instruction files or skills side by side and link them to one shared ~/.agents/ source.",
        },
        {
          step: "03 · Audit",
          title: "Probe MCP & catch secrets",
          desc: "Run live MCP health checks, spot command conflicts between agents, and migrate plaintext keys to env vars.",
        },
        {
          step: "04 · Track",
          title: "Monitor local spend",
          desc: "Inspect 7, 30, and 90-day token usage and API-equivalent cost straight from local agent session logs.",
        },
      ],
      pitfallsHeading: "AI config headaches Hearth prevents",
      pitfalls: [
        {
          title: "Drifting rules across 4+ agents",
          desc: "Updating CLAUDE.md while forgetting Cursor or Codex rules leads to inconsistent code style. One symlinked AGENTS.md keeps them aligned.",
        },
        {
          title: "Broken or silently failing MCP servers",
          desc: "Wrong Node paths or missing binaries break MCP tools with vague errors. Hearth probes each server and shows the exact resolved command.",
        },
        {
          title: "Leaked keys in dotfiles",
          desc: "Pasting tokens directly into mcp.json risks accidental commits or backups. Hearth masks secrets in reports and flags plaintext values.",
        },
      ],
    },
  },
};
