import type { SeoContentBundle } from "./types";

export const deSeoContent: SeoContentBundle = {
  alternatives: {
    "docker-desktop": {
      slug: "docker-desktop",
      path: "/alternatives/docker-desktop/",
      crumbLabel: "Docker Desktop",
      metaTitle: "Kostenlose Docker-Desktop-Alternative (Colima & Go) · Hearth",
      metaDescription:
        "Ersetze Docker Desktop durch Open-Source-Colima unter macOS oder native Docker Engine unter Linux in Hearth, einer schlanken nativen Go-App ohne Electron.",
      heroTitle: "Eine kostenlose, native Docker-Desktop-Alternative ohne Electron.",
      heroSubtitle:
        "Nutze Open-Source-Colima unter macOS oder native Docker Engine unter Linux. Verwalte Compose-Stacks, Container, Images, Volumes und Live-Logs in einer nativen Go-App ohne Electron.",
      metrics: [
        {
          value: "~20 MB Download",
          label: "Natives Go & mygo UI · <80 ms Kaltstart · kein Electron",
        },
        {
          value: "MIT Kostenlos",
          label: "Keine kommerziellen Lizenzgebühren, kein Login, keine Telemetrie",
        },
        {
          value: "Colima + Linux",
          label: "Open-Source-VM auf macOS, native Engine auf Linux, plus K8s & DBs",
        },
      ],
      hearthColumnLabel: "Hearth (Colima / Linux Engine)",
      otherColumnLabel: "Docker Desktop",
      rows: [
        {
          criterion: "Lizenz & Preis",
          hearth: "Kostenlos & MIT Open Source für private und kommerzielle Teams",
          other: "Bezahlte Lizenzen für Unternehmen ab Schwellenwert nötig",
        },
        {
          criterion: "Speicher & Tempo",
          hearth: "Einzelne native Go-Binärdatei (mygo), ~20 MB Download",
          other: "Electron-Oberfläche, oft 1 GB+ RAM im Leerlauf",
        },
        {
          criterion: "macOS Container-VM",
          hearth: "Open-Source-Colima per Klick mit CPU/RAM/Disk-Steuerung",
          other: "Proprietäre VM in Docker Desktop gebündelt",
        },
        {
          criterion: "Linux-Workflow",
          hearth: "Verbindet sich direkt mit nativer Docker Engine (keine Extra-VM)",
          other: "Startet unter Linux eine zusätzliche VM-Schicht",
        },
        {
          criterion: "Über Docker hinaus",
          hearth: "Inkl. Kubernetes UI, lokale DBs, Runtimes, KI/MCP & Bereinigung",
          other: "Nur Docker & integriertes Einzelknoten-K8s",
        },
        {
          criterion: "Privatsphäre",
          hearth: "Keine Telemetrie, kein Konto nötig, 100 % lokal",
          other: "Konto-Hinweise & Nutzungstelemetrie standardmäßig aktiv",
        },
      ],
      chooseHearth: {
        pill: "Empfohlen für lokale Entwicklung",
        title: "Wähle Hearth, wenn…",
        points: [
          "Du eine kostenlose MIT-Alternative ohne Sitzplatzlimits möchtest",
          "Du Colima auf macOS oder native Docker Engine auf Linux mit schneller UI willst",
          "Du Container, Kubernetes, lokale DBs & KI-Agenten in einer nativen Go-App bündeln willst",
        ],
      },
      keepOther: {
        pill: "Wann Docker Desktop noch passt",
        title: "Bleib bei Docker Desktop, wenn…",
        points: [
          "Dein Unternehmen Docker Business SSO & Image-Policies vorschreibt",
          "Du auf Plugins aus dem Docker Extensions Marketplace angewiesen bist",
          "Du eng integriertes Docker Scout oder Docker Build Cloud brauchst",
        ],
      },
      steps: [
        {
          step: "Schritt 01",
          title: "Hearth installieren (ohne Hintergrundänderungen)",
          desc: "Lade Hearth (~20 MB Download; ~28 MB Universal-DMG) per DMG, Homebrew oder .deb. Der erste Scan ist strikt read-only.",
        },
        {
          step: "Schritt 02",
          title: "Colima einrichten oder Linux Engine verbinden",
          desc: "Öffne Docker in Hearth und prüfe den klaren Plan: Colima, Docker CLI, Compose & Buildx inkl. CPU/RAM-Limits.",
        },
        {
          step: "Schritt 03",
          title: "Docker Desktop stoppen & Speicher freigeben",
          desc: "Prüfe deine Compose-Stacks in Hearth und bereinige anschließend alte Build-Caches und VM-Dateien sicher.",
        },
      ],
    },
    "lens-freelens": {
      slug: "lens-freelens",
      path: "/alternatives/lens-freelens/",
      crumbLabel: "Lens & Freelens",
      metaTitle: "Schnelle native Lens-, Freelens- & Headlamp-Alternative · Hearth",
      metaDescription:
        "Untersuche Kubernetes-Pods, Logs, Crash-Diagnosen, Port-Forwards, Secrets, Helm und kind-Cluster in einer nativen Go-UI ohne Electron.",
      heroTitle: "Eine schnelle native Kubernetes-UI ohne Electron-Trägheit.",
      heroSubtitle:
        "Direkt auf Go client-go aufgebaut. Prüfe Pods, Absturzursachen, Live-Logs, Port-Forwards, Helm-Releases, Secrets und lokale kind-Cluster in einer nativen Go-App ohne Electron.",
      metrics: [
        { value: "Direktes client-go", label: "Liest kubeconfig nativ mit <80 ms Kaltstart" },
        { value: "Kein Login", label: "Keine Lens ID, kein Cloud-Sync, keine Telemetrie, MIT" },
        {
          value: "Kompletter Dev-Stack",
          label: "Kubernetes neben Docker, lokalen DBs, Runtimes & KI",
        },
      ],
      hearthColumnLabel: "Hearth (Natives client-go)",
      otherColumnLabel: "Lens / Freelens / Headlamp",
      rows: [
        {
          criterion: "Runtime & Speicher",
          hearth: "Einzelne native Go-Binärdatei (mygo), kein Electron",
          other: "Electron-/Chromium-App (Lens, Freelens) oder Browser-UI",
        },
        {
          criterion: "Konto & Lizenz",
          hearth: "MIT Open Source, kein Konto und keine Paywall",
          other: "Lens verlangt Konto & Tarife; Freelens/Headlamp sind OSS",
        },
        {
          criterion: "Crash- & Pod-Debugging",
          hearth: "Crash-Diagnose, Events, Live-Logs & Exec-Shell direkt integriert",
          other: "Pod-Logs, Events, Shell & Multi-Panel-Ansichten",
        },
        {
          criterion: "Helm, Secrets & Port-Forward",
          hearth: "Eingebauter Port-Forward-Manager, Secrets-Ansicht & Helm-Liste",
          other: "Integriert oder über Community-Erweiterungen verfügbar",
        },
        {
          criterion: "Lokale kind-Cluster",
          hearth: "Erstellen, starten, stoppen & wechseln direkt in der App",
          other: "Zeigt bestehende Cluster; Erstellung per separater CLI",
        },
        {
          criterion: "Sicherheit bei Änderungen",
          hearth: "Explizite Plan-Vorschau vor Neustarts, Löschen oder Skalieren",
          other: "Sofortige Ausführung oder einfaches Bestätigungsfenster",
        },
      ],
      chooseHearth: {
        pill: "Empfohlen für tägliche Arbeit",
        title: "Wähle Hearth, wenn…",
        points: [
          "Du blitzschnellen Start und wenig RAM beim Wechsel zwischen lokalen und Staging-Clustern willst",
          "Du Pods, Crash-Diagnose, Live-Logs, Port-Forward, Helm & kind an einem Ort brauchst",
          "Du getrennte UIs für Docker, Kubernetes und lokale Datenbanken ersetzen willst",
        ],
      },
      keepOther: {
        pill: "Wann Lens / Headlamp besser passen",
        title: "Bleib bei Lens oder Headlamp, wenn…",
        points: [
          "Du auf spezielle Plugins aus dem Lens- oder Headlamp-Ökosystem angewiesen bist",
          "Du integrierte Prometheus-Metrikdiagramme für große Produktionscluster brauchst",
          "Du ein im Cluster gehostetes Web-Dashboard für Plattform-Teams suchst",
        ],
      },
      steps: [
        {
          step: "Schritt 01",
          title: "Hearth öffnen & ~/.kube/config erkennen",
          desc: "Hearth liest alle kubeconfig-Kontexte rein lesend ein, ohne den aktiven Kontext zu ändern.",
        },
        {
          step: "Schritt 02",
          title: "Pods, Logs & Port-Forwards prüfen",
          desc: "Wechsle Namespaces, springe direkt in CrashLoop-Diagnosen oder leite Ports mit einem Klick weiter.",
        },
        {
          step: "Schritt 03",
          title: "Lokale kind-Cluster bei Bedarf starten",
          desc: "Erstelle oder bereinige lokale kind-Cluster direkt neben deinen Colima-/Docker-Containern.",
        },
      ],
    },
    dbngin: {
      slug: "dbngin",
      path: "/alternatives/dbngin/",
      crumbLabel: "DBngin",
      metaTitle: "Kostenlose plattformübergreifende DBngin-Alternative · Hearth",
      metaDescription:
        "Starte PostgreSQL, Redis, MySQL, MariaDB und MongoDB lokal unter macOS, Windows und Linux mit Port-Konflikterkennung und Autostart in Hearth.",
      heroTitle: "Lokale Datenbanken & Ports unter macOS, Windows und Linux.",
      heroSubtitle:
        "Installiere und steuere PostgreSQL, Redis, MySQL, MariaDB und MongoDB nativ oder über Docker. Erkenne Port-Konflikte, schalte Autostart um und öffne psql oder redis-cli plattformübergreifend.",
      metrics: [
        { value: "3 Plattformen", label: "macOS, Windows & Linux (statt nur macOS bei DBngin)" },
        {
          value: "5 DB-Engines",
          label: "PostgreSQL, Redis, MySQL, MariaDB & MongoDB lokal oder Docker",
        },
        {
          value: "Port-Sicherheit",
          label: "Live-Port-Konflikterkennung mit Prozessinfo & direkter CLI",
        },
      ],
      hearthColumnLabel: "Hearth (Datenbanken & Ports)",
      otherColumnLabel: "DBngin / Postgres.app",
      rows: [
        {
          criterion: "Betriebssysteme",
          hearth: "macOS, Windows & Linux mit einheitlicher Oberfläche",
          other: "Nur macOS (DBngin & Postgres.app)",
        },
        {
          criterion: "Unterstützte Engines",
          hearth: "PostgreSQL, Redis, MySQL, MariaDB & MongoDB",
          other: "PostgreSQL, MySQL & Redis (DBngin) oder nur Postgres",
        },
        {
          criterion: "Native & Docker-Dienste",
          hearth: "Verwaltet Homebrew-/Systemdienste und Docker-DBs gemeinsam",
          other: "Nur eigene verwaltete Einzel-Binaries",
        },
        {
          criterion: "Port-Konflikte",
          hearth: "Zeigt vor dem Start, welcher Prozess 5432, 6379 oder 3306 belegt",
          other: "Manuelle Portwahl ohne systemweite Konfliktansicht",
        },
        {
          criterion: "Lizenz & Quellcode",
          hearth: "100 % kostenlos & MIT Open Source",
          other: "Freeware / Closed Source (DBngin)",
        },
        {
          criterion: "SQL-Abfrage-Editor",
          hearth: "Nicht enthalten — ergänzt TablePlus, DataGrip & psql ideal",
          other: "Nicht enthalten — ergänzt TablePlus oder psql",
        },
      ],
      chooseHearth: {
        pill: "Empfohlen für lokale DBs",
        title: "Wähle Hearth, wenn…",
        points: [
          "Du unter macOS, Windows oder Linux arbeitest und ein Open-Source-Tool suchst",
          "Du sowohl native Dienste als auch Docker-Compose-Datenbanken nutzt",
          "Du Port-Konflikte und schnellen CLI-Zugriff neben dem restlichen Stack willst",
        ],
      },
      keepOther: {
        pill: "Wann DBngin / SQL-GUIs weiter passen",
        title: "Behalte DBngin oder TablePlus, wenn…",
        points: [
          "Du isolierte parallele Minor-Versionen ohne Docker unter macOS brauchst",
          "Du einen visuellen SQL-Editor und Tabellenbrowser suchst (behalte TablePlus/DataGrip)",
          "Du bestehende DBngin-Datenverzeichnisse noch nicht migrieren möchtest",
        ],
      },
      steps: [
        {
          step: "Schritt 01",
          title: "Laufende Dienste & Ports scannen",
          desc: "Hearth erkennt Homebrew-, System- und Docker-Datenbanken automatisch ohne Neustart.",
        },
        {
          step: "Schritt 02",
          title: "Datenbanken installieren oder Autostart steuern",
          desc: "Installiere PostgreSQL, Redis, MySQL, MariaDB oder MongoDB per klarem Plan und steuere den Autostart.",
        },
        {
          step: "Schritt 03",
          title: "SQL-Client oder CLI verbinden",
          desc: "Kopiere Verbindungsdaten, öffne psql/redis-cli/mongosh und nutze weiter TablePlus oder DataGrip.",
        },
      ],
    },
  },
  guides: {
    "dev-environment-setup": {
      slug: "dev-environment-setup",
      path: "/guides/dev-environment-setup/",
      crumbLabel: "Dev-Setup",
      metaTitle: "Dev-Umgebung einrichten für Einsteiger & Vibe Coding · Hearth",
      metaDescription:
        "Installiere Node.js, Python, Git, Docker, PostgreSQL und Claude Code oder Cursor unter macOS, Windows und Linux in Minuten ohne kaputten PATH.",
      heroTitle: "Richte deine Dev-Umgebung in Minuten ein, ohne PATH zu zerstören.",
      heroSubtitle:
        "Startest du dein erstes Projekt mit Claude Code oder Cursor oder richtest einen neuen Mac, Windows- oder Linux-Rechner ein? Hearth installiert Runtimes, Docker, DBs und KI-Tools nach einem transparenten Plan.",
      secondaryCtaLabel: "Auch lesen: KI-Agenten & MCP Guide",
      secondaryCtaTo: "/guides/ai-agents-mcp/",
      presetsHeading: "Wähle ein fertiges Preset oder baue dein eigenes",
      presets: [
        {
          pill: "Einsteiger & Vibe Coding",
          pillTone: "accent",
          title: "Vibe Coding Starter",
          desc: "Vom frischen Laptop in 10 Minuten bereit fürs Bauen mit KI-Agenten.",
          stackLines: [
            "Git + GitHub CLI · mise",
            "Node.js 22 LTS · Python 3.13 (uv)",
            "Claude Code · Cursor · geteilte AGENTS.md",
          ],
          foot: "Kein manuelles Bearbeiten der ~/.zshrc",
        },
        {
          pill: "Web & Produkt",
          pillTone: "info",
          title: "Full-Stack Web",
          desc: "Alles, um moderne TypeScript- & Python-Repos sofort zu starten.",
          stackLines: [
            "Homebrew/WinGet · mise",
            "Node 22 · Bun · pnpm · Python 3.13",
            "Docker + Colima · PostgreSQL · Redis",
          ],
          foot: "Respektiert .nvmrc & mise.toml",
        },
        {
          pill: "Backend & Infra",
          pillTone: "success",
          title: "Backend & Cloud",
          desc: "Für APIs, Container und lokale Kubernetes-Workflows.",
          stackLines: [
            "mise · Go 1.27 · Rust",
            "Docker + Colima · kubectl · kind",
            "PostgreSQL 16 · Redis · Terraform",
          ],
          foot: "Exportierbar als hearth-setup.json",
        },
      ],
      stepsHeading: "So funktioniert das geführte Setup",
      steps: [
        {
          step: "01 · Auswahl",
          title: "Stack wählen",
          desc: "Wähle ein Preset für Vibe Coding, Web oder Backend oder importiere eine Team-JSON-Datei.",
        },
        {
          step: "02 · Plan",
          title: "Exaktes Diff vorab prüfen",
          desc: "Sieh jeden Befehl und jede Shell-Änderung vor dem Start inklusive .bak-Sicherung.",
        },
        {
          step: "03 · Installation",
          title: "Ausführen mit Live-Logs",
          desc: "Installiert über Homebrew, WinGet, mise & Colima in richtiger Reihenfolge mit Rollback bei Fehlern.",
        },
        {
          step: "04 · Prüfung",
          title: "In frischer Shell verifizieren",
          desc: "Hearth prüft in einer sauberen Login-Shell, ob jedes Binary über den richtigen PATH startet.",
        },
      ],
      pitfallsHeading: "Typische Setup-Fehler, die Hearth verhindert",
      pitfalls: [
        {
          title: "Kaputter PATH & langsamer zsh-Start",
          desc: "Das Mischen von Homebrew-Node, nvm und pyenv verursacht Versionskonflikte und langsame Terminals. Hearth nutzt schnelles mise und prüft which -a.",
        },
        {
          title: "Klartext-API-Keys in MCP-Configs",
          desc: "Beim Verbinden von KI-Agenten landen Keys oft direkt in JSON-Dateien. Hearth erkennt sie und schlägt Umgebungsvariablen vor.",
        },
        {
          title: "Stilles Überschreiben von Configs",
          desc: "Zufällige Installationsskripte schreiben ohne Backup in ~/.zshrc. Hearth ändert nur seinen eigenen Marker-Block mit .bak-Kopie.",
        },
      ],
    },
    "ai-agents-mcp": {
      slug: "ai-agents-mcp",
      path: "/guides/ai-agents-mcp/",
      crumbLabel: "KI-Agenten & MCP",
      metaTitle: "Claude Code, Codex, Cursor, AGENTS.md & MCP zentral verwalten · Hearth",
      metaDescription:
        "Vereine AGENTS.md für alle KI-Agenten, teile Skills, teste MCP-Server live, finde Klartext-Secrets und verfolge lokale Token-Kosten in Hearth.",
      heroTitle: "Vereine AGENTS.md, Skills, MCP-Server und lokale KI-Kosten.",
      heroSubtitle:
        "Schluss mit fünf versteckten Dotfiles für Claude Code, Codex CLI, Cursor, GitHub Copilot und Gemini CLI. Hearth hält Regeln synchron, testet MCP-Server live und zählt lokale Token-Ausgaben privat.",
      secondaryCtaLabel: "Auch lesen: Dev-Setup-Guide",
      secondaryCtaTo: "/guides/dev-environment-setup/",
      presetsHeading: "Drei KI-Workflows, die Hearth vereinfacht",
      presets: [
        {
          pill: "Geteilte Regeln",
          pillTone: "accent",
          title: "Eine ~/.agents/AGENTS.md",
          desc: "Bearbeite deine globalen Regeln einmal und verlinke sie sicher in jeden Agenten.",
          stackLines: [
            "~/.agents/AGENTS.md (eine Quelle)",
            "CLAUDE.md · CODEX · Gemini · Cursor",
            "Side-by-Side-Diff vor dem Verlinken",
          ],
          foot: "Automatisches .bak vor jedem Symlink",
        },
        {
          pill: "MCP-Kontrolle",
          pillTone: "info",
          title: "Live-MCP-Test & eingebauter Server",
          desc: "Prüfe jeden stdio/HTTP-MCP-Server und gib deinem Agenten Leserechte auf Hearth-Diagnosen.",
          stackLines: [
            "hearth mcp (lokaler stdio-Server)",
            "Live-Handshake & Tool-Zählung",
            "Warnungen bei Klartext-API-Keys",
          ],
          foot: "Jede Änderung erfordert deine Freigabe",
        },
        {
          pill: "Skills & Verbrauch",
          pillTone: "success",
          title: "Geteilte Skills & Token-Übersicht",
          desc: "Installiere Skills aus GitHub-Katalogen und sieh lokale 7/30/90-Tage-Schätzungen.",
          stackLines: [
            "~/.agents/skills gemeinsamer Katalog",
            "Diff & Abgleich abweichender Kopien",
            "Lokale JSONL-Auswertung & Kosten",
          ],
          foot: "100 % lokal · kein Cloud-Upload",
        },
      ],
      stepsHeading: "So ordnet Hearth dein KI-Setup",
      steps: [
        {
          step: "01 · Erkennen",
          title: "Alle Agenten & Configs finden",
          desc: "Read-only-Scan findet Claude Code, Codex, Cursor, Copilot, Gemini und OpenCode automatisch.",
        },
        {
          step: "02 · Vereinen",
          title: "AGENTS.md & Skills synchronisieren",
          desc: "Vergleiche abweichende Anweisungen und verlinke sie auf eine gemeinsame Quelle unter ~/.agents/.",
        },
        {
          step: "03 · Prüfen",
          title: "MCP testen & Secrets abfangen",
          desc: "Starte Live-MCP-Checks, finde Konflikte zwischen Agenten und verschiebe Keys in Env-Variablen.",
        },
        {
          step: "04 · Verfolgen",
          title: "Lokale Ausgaben im Blick behalten",
          desc: "Sieh 7-, 30- und 90-Tage-Token-Verbrauch direkt aus lokalen Agent-Sitzungslogs.",
        },
      ],
      pitfallsHeading: "KI-Config-Probleme, die Hearth verhindert",
      pitfalls: [
        {
          title: "Auseinanderlaufende Regeln bei 4+ Agenten",
          desc: "Wer CLAUDE.md ändert, aber Cursor oder Codex vergisst, bekommt inkonsistenten Code. Eine verlinkte AGENTS.md hält alles synchron.",
        },
        {
          title: "Stumm fehlschlagende MCP-Server",
          desc: "Falsche Node-Pfade brechen MCP-Tools mit unklaren Fehlern. Hearth testet jeden Server und zeigt den aufgelösten Befehl.",
        },
        {
          title: "Geleakte Keys in Dotfiles",
          desc: "Direkt in mcp.json kopierte Tokens drohen in Commits zu landen. Hearth maskiert Secrets und warnt bei Klartext.",
        },
      ],
    },
  },
};
