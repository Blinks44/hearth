import type { SeoContentBundle } from "./types";

export const esSeoContent: SeoContentBundle = {
  alternatives: {
    "docker-desktop": {
      slug: "docker-desktop",
      path: "/alternatives/docker-desktop/",
      crumbLabel: "Docker Desktop",
      metaTitle: "Alternativa gratuita a Docker Desktop (Colima y Go) · Hearth",
      metaDescription:
        "Sustituye Docker Desktop por Colima open source en macOS o Docker Engine nativo en Linux dentro de Hearth, una app Go nativa ligera sin Electron.",
      heroTitle: "Una alternativa nativa y gratuita a Docker Desktop sin Electron.",
      heroSubtitle:
        "Ejecuta Colima open source en macOS o Docker Engine nativo en Linux. Inspecciona pilas Compose, contenedores, imágenes, volúmenes y logs en vivo en una app Go nativa sin Electron.",
      metrics: [
        {
          value: "~20 MB descarga",
          label: "Go nativo y UI mygo · <80 ms de arranque · sin Electron",
        },
        { value: "MIT Gratis", label: "Sin licencias de pago, sin cuentas, cero telemetría" },
        {
          value: "Colima + Linux",
          label: "VM open source en macOS, Engine nativo en Linux, más K8s y BD",
        },
      ],
      hearthColumnLabel: "Hearth (Colima / Linux Engine)",
      otherColumnLabel: "Docker Desktop",
      rows: [
        {
          criterion: "Licencia y precio",
          hearth: "Gratis y open source MIT para uso personal y equipos comerciales",
          other: "Requiere suscripción de pago en empresas por encima del umbral",
        },
        {
          criterion: "Peso y velocidad",
          hearth: "Binario Go nativo único (mygo), ~20 MB de descarga",
          other: "Interfaz Electron, suele consumir 1 GB+ de RAM en reposo",
        },
        {
          criterion: "VM en macOS",
          hearth: "Colima open source en un clic con controles de CPU/RAM/disco",
          other: "VM propietaria integrada en Docker Desktop",
        },
        {
          criterion: "Flujo en Linux",
          hearth: "Se conecta directo a Docker Engine nativo (sin VM extra)",
          other: "Ejecuta una capa de VM adicional en Linux",
        },
        {
          criterion: "Más allá de Docker",
          hearth: "Incluye UI Kubernetes, BD locales, runtimes, IA/MCP y limpieza",
          other: "Solo Docker y K8s integrado de un solo nodo",
        },
        {
          criterion: "Privacidad y telemetría",
          hearth: "Cero telemetría, sin cuenta obligatoria, 100 % local",
          other: "Avisos de cuenta y telemetría activados por defecto",
        },
      ],
      chooseHearth: {
        pill: "Recomendado para dev local",
        title: "Elige Hearth cuando…",
        points: [
          "Quieres una alternativa gratuita bajo licencia MIT sin límites comerciales",
          "Buscas Colima en macOS o Docker Engine nativo en Linux con una UI rápida",
          "Quieres contenedores, Kubernetes, BD locales y agentes IA en una app nativa en Go",
        ],
      },
      keepOther: {
        pill: "Cuándo Docker Desktop aún encaja",
        title: "Quédate con Docker Desktop cuando…",
        points: [
          "Tu empresa exige SSO de Docker Business y políticas de imágenes",
          "Dependes de extensiones específicas del marketplace de Docker",
          "Necesitas Docker Scout o Docker Build Cloud integrados de forma nativa",
        ],
      },
      steps: [
        {
          step: "Paso 01",
          title: "Instala Hearth (sin cambios ocultos)",
          desc: "Descarga Hearth (~20 MB; ~28 MB DMG universal) por DMG, Homebrew o .deb. El primer escaneo es 100 % de solo lectura.",
        },
        {
          step: "Paso 02",
          title: "Configura Colima o conecta Linux Engine",
          desc: "Abre Docker en Hearth y revisa el plan explícito: Colima, Docker CLI, Compose y Buildx con límites de CPU/RAM.",
        },
        {
          step: "Paso 03",
          title: "Detén Docker Desktop y recupera gigabytes",
          desc: "Comprueba tus pilas Compose en Hearth y limpia de forma segura cachés viejas y discos de la VM anterior.",
        },
      ],
    },
    "lens-freelens": {
      slug: "lens-freelens",
      path: "/alternatives/lens-freelens/",
      crumbLabel: "Lens y Freelens",
      metaTitle: "Alternativa nativa rápida a Lens, Freelens y Headlamp · Hearth",
      metaDescription:
        "Inspecciona pods Kubernetes, logs, diagnósticos de fallos, port-forwards, Secrets, Helm y clústeres kind en una UI nativa Go sin Electron.",
      heroTitle: "Una interfaz nativa de Kubernetes sin la lentitud de Electron.",
      heroSubtitle:
        "Construida directamente sobre Go client-go. Inspecciona pods, causas de fallos, logs en vivo, port-forwards, Helm, Secrets y clústeres kind locales en una app nativa en Go en vez de una ventana pesada de Electron.",
      metrics: [
        { value: "client-go directo", label: "Lee kubeconfig de forma nativa con arranque <80 ms" },
        { value: "Sin registro", label: "Sin Lens ID, sin nube, sin telemetría, open source MIT" },
        { value: "Todo tu stack", label: "Kubernetes junto a Docker, BD locales, runtimes e IA" },
      ],
      hearthColumnLabel: "Hearth (client-go nativo)",
      otherColumnLabel: "Lens / Freelens / Headlamp",
      rows: [
        {
          criterion: "Motor y memoria",
          hearth: "Binario Go nativo único (mygo), sin Electron",
          other: "App Electron/Chromium (Lens, Freelens) o interfaz web",
        },
        {
          criterion: "Cuenta y licencia",
          hearth: "Open source MIT, sin cuentas ni muros de pago",
          other: "Lens pide cuenta y planes de pago; Freelens/Headlamp son OSS",
        },
        {
          criterion: "Depuración de pods",
          hearth: "Diagnóstico de fallos, eventos, logs en vivo y shell exec",
          other: "Logs de pods, eventos, consola y vistas multipanel",
        },
        {
          criterion: "Helm, Secrets y port-forward",
          hearth: "Gestor de port-forward, lectura de Secrets y lista de Helm",
          other: "Disponible en vistas integradas o extensiones de la comunidad",
        },
        {
          criterion: "Clústeres locales kind",
          hearth: "Crea, inicia, detén y cambia clústeres kind desde la app",
          other: "Inspecciona clústeres existentes; creación desde CLI externa",
        },
        {
          criterion: "Seguridad al mutar",
          hearth: "Vista previa de plan explícito antes de reiniciar, borrar o escalar",
          other: "Acción inmediata o diálogo simple de confirmación",
        },
      ],
      chooseHearth: {
        pill: "Recomendado para uso diario",
        title: "Elige Hearth cuando…",
        points: [
          "Quieres arranque instantáneo y poca RAM al saltar entre clústeres locales y staging",
          "Necesitas pods, diagnóstico CrashLoop, logs en vivo, port-forward, Helm y kind en un lugar",
          "Quieres retirar interfaces separadas de Docker, Kubernetes y bases locales",
        ],
      },
      keepOther: {
        pill: "Cuándo Lens / Headlamp encajan mejor",
        title: "Quédate con Lens o Headlamp cuando…",
        points: [
          "Dependes de plugins propios del ecosistema Lens o Headlamp",
          "Necesitas gráficas integradas de Prometheus en grandes clústeres de producción",
          "Prefieres un panel web alojado dentro del propio clúster para equipos de plataforma",
        ],
      },
      steps: [
        {
          step: "Paso 01",
          title: "Abre Hearth y detecta ~/.kube/config",
          desc: "Hearth descubre todos tus contextos en solo lectura sin alterar el contexto activo.",
        },
        {
          step: "Paso 02",
          title: "Inspecciona pods, logs y port-forwards",
          desc: "Cambia de namespace, abre diagnósticos CrashLoop al instante o reenvía puertos en un clic.",
        },
        {
          step: "Paso 03",
          title: "Crea clústeres kind locales cuando los necesites",
          desc: "Gestiona clústeres kind junto a tus contenedores de Colima o Docker Engine sin salir de Hearth.",
        },
      ],
    },
    dbngin: {
      slug: "dbngin",
      path: "/alternatives/dbngin/",
      crumbLabel: "DBngin",
      metaTitle: "Alternativa gratuita y multiplataforma a DBngin · Hearth",
      metaDescription:
        "Ejecuta PostgreSQL, Redis, MySQL, MariaDB y MongoDB en macOS, Windows y Linux con detección de puertos y control de inicio automático en Hearth.",
      heroTitle: "Bases de datos locales y puertos en macOS, Windows y Linux.",
      heroSubtitle:
        "Instala, inicia y detén PostgreSQL, Redis, MySQL, MariaDB y MongoDB en nativo o Docker. Detecta conflictos de puertos, ajusta el inicio automático y abre psql o redis-cli en cualquier sistema.",
      metrics: [
        { value: "3 plataformas", label: "macOS, Windows y Linux (no solo macOS como DBngin)" },
        {
          value: "5 motores BD",
          label: "PostgreSQL, Redis, MySQL, MariaDB y MongoDB nativo o Docker",
        },
        {
          value: "Control de puertos",
          label: "Detección en vivo de conflictos con proceso y CLI rápida",
        },
      ],
      hearthColumnLabel: "Hearth (Bases y puertos)",
      otherColumnLabel: "DBngin / Postgres.app",
      rows: [
        {
          criterion: "Sistemas operativos",
          hearth: "macOS, Windows y Linux con la misma interfaz",
          other: "Solo macOS (DBngin y Postgres.app)",
        },
        {
          criterion: "Motores soportados",
          hearth: "PostgreSQL, Redis, MySQL, MariaDB y MongoDB",
          other: "PostgreSQL, MySQL y Redis (DBngin) o solo Postgres",
        },
        {
          criterion: "Nativo y Docker",
          hearth: "Gestiona servicios de Homebrew/SO y contenedores Docker juntos",
          other: "Solo binarios independientes gestionados por la app",
        },
        {
          criterion: "Conflictos de puertos",
          hearth: "Muestra qué proceso ocupa 5432, 6379 o 3306 antes de fallar",
          other: "Selección manual sin vista global de puertos ocupados",
        },
        {
          criterion: "Licencia y código",
          hearth: "100 % gratis y open source MIT",
          other: "Gratuito pero código cerrado (DBngin)",
        },
        {
          criterion: "Editor SQL",
          hearth: "No incluido — combina perfecto con TablePlus, DataGrip y psql",
          other: "No incluido — combina con TablePlus o psql",
        },
      ],
      chooseHearth: {
        pill: "Recomendado para BD locales",
        title: "Elige Hearth cuando…",
        points: [
          "Trabajas en macOS, Windows o Linux y prefieres una herramienta open source",
          "Combinas servicios nativos de Homebrew/SO con bases en Docker Compose",
          "Quieres alertas de puertos y acceso rápido a CLI junto al resto de tu entorno",
        ],
      },
      keepOther: {
        pill: "Cuándo DBngin / GUIs SQL aún encajan",
        title: "Conserva DBngin o TablePlus cuando…",
        points: [
          "Necesitas versiones menores aisladas en paralelo en macOS sin usar Docker",
          "Necesitas un editor visual SQL, explorador de tablas o diagramas (mantén TablePlus/DataGrip)",
          "Ya tienes directorios de datos de DBngin que aún no deseas migrar",
        ],
      },
      steps: [
        {
          step: "Paso 01",
          title: "Escanea servicios y puertos activos",
          desc: "Hearth detecta bases de Homebrew, del sistema y de Docker sin reiniciar nada ni tocar tus datos.",
        },
        {
          step: "Paso 02",
          title: "Instala motores o ajusta autostart",
          desc: "Añade PostgreSQL, Redis, MySQL, MariaDB o MongoDB desde un plan claro y decide qué arranca al iniciar sesión.",
        },
        {
          step: "Paso 03",
          title: "Conecta tu cliente SQL o CLI",
          desc: "Copia los datos de conexión, abre psql/redis-cli/mongosh y sigue usando TablePlus o DataGrip.",
        },
      ],
    },
  },
  guides: {
    "dev-environment-setup": {
      slug: "dev-environment-setup",
      path: "/guides/dev-environment-setup/",
      crumbLabel: "Entorno dev",
      metaTitle: "Guía de configuración dev para principiantes y vibe coding · Hearth",
      metaDescription:
        "Configura Node.js, Python, Git, Docker, PostgreSQL y Claude Code o Cursor en macOS, Windows o Linux en minutos sin romper el PATH.",
      heroTitle: "Configura un entorno dev limpio en minutos, sin romper el PATH.",
      heroSubtitle:
        "¿Empiezas tu primer proyecto con Claude Code o Cursor, o estrenas Mac, Windows o Linux? Hearth instala runtimes, Docker, bases de datos y herramientas IA desde un plan transparente.",
      secondaryCtaLabel: "Leer también: guía de agentes IA y MCP",
      secondaryCtaTo: "/guides/ai-agents-mcp/",
      presetsHeading: "Elige un preajuste listo o arma el tuyo",
      presets: [
        {
          pill: "Principiantes y Vibe Coding",
          pillTone: "accent",
          title: "Vibe Coding Starter",
          desc: "Pasa de un portátil recién estrenado a crear con agentes IA en 10 minutos.",
          stackLines: [
            "Git + GitHub CLI · mise",
            "Node.js 22 LTS · Python 3.13 (uv)",
            "Claude Code · Cursor · AGENTS.md compartido",
          ],
          foot: "Sin editar ~/.zshrc a mano",
        },
        {
          pill: "Web y Producto",
          pillTone: "info",
          title: "Full-Stack Web",
          desc: "Todo lo necesario para clonar repos modernos de TypeScript y Python.",
          stackLines: [
            "Homebrew/WinGet · mise",
            "Node 22 · Bun · pnpm · Python 3.13",
            "Docker + Colima · PostgreSQL · Redis",
          ],
          foot: "Respeta .nvmrc y mise.toml",
        },
        {
          pill: "Backend e Infra",
          pillTone: "success",
          title: "Backend & Cloud",
          desc: "Para APIs, contenedores y flujos locales de Kubernetes.",
          stackLines: [
            "mise · Go 1.27 · Rust",
            "Docker + Colima · kubectl · kind",
            "PostgreSQL 16 · Redis · Terraform",
          ],
          foot: "Exportable como hearth-setup.json",
        },
      ],
      stepsHeading: "Cómo funciona la configuración guiada",
      steps: [
        {
          step: "01 · Elegir",
          title: "Selecciona tu stack",
          desc: "Elige un preajuste Vibe Coding, Web o Backend, o importa un JSON compartido por tu equipo.",
        },
        {
          step: "02 · Plan",
          title: "Revisa el diff exacto",
          desc: "Ve cada comando y cambio de shell antes de ejecutar nada, con copias .bak automáticas.",
        },
        {
          step: "03 · Instalar",
          title: "Ejecuta con logs en vivo",
          desc: "Instala con Homebrew, WinGet, mise y Colima en orden de dependencias con reversión ante fallos.",
        },
        {
          step: "04 · Verificar",
          title: "Comprueba en una shell limpia",
          desc: "Hearth abre una login shell limpia para confirmar que cada binario se resuelve en el orden correcto.",
        },
      ],
      pitfallsHeading: "Errores comunes de setup que Hearth evita",
      pitfalls: [
        {
          title: "PATH roto y zsh lento",
          desc: "Mezclar Node de Homebrew, nvm y pyenv crea conflictos y demoras al abrir la terminal. Hearth usa mise y revisa which -a.",
        },
        {
          title: "Claves API en texto plano en MCP",
          desc: "Al conectar agentes IA, los secretos suelen quedar en JSON. Hearth los detecta y propone migrarlos a variables de entorno.",
        },
        {
          title: "Sobrescrituras silenciosas",
          desc: "Los scripts de instalación editan ~/.zshrc sin copia. Hearth solo toca su bloque marcado y guarda copias .bak.",
        },
      ],
    },
    "ai-agents-mcp": {
      slug: "ai-agents-mcp",
      path: "/guides/ai-agents-mcp/",
      crumbLabel: "Agentes IA y MCP",
      metaTitle: "Gestiona Claude Code, Codex, Cursor, AGENTS.md y MCP en Hearth",
      metaDescription:
        "Unifica AGENTS.md entre agentes IA, comparte skills, prueba servidores MCP en vivo, detecta secretos y sigue el gasto local de tokens.",
      heroTitle: "Unifica AGENTS.md, skills, servidores MCP y gasto local de IA.",
      heroSubtitle:
        "Deja de sincronizar cinco archivos ocultos entre Claude Code, Codex CLI, Cursor, GitHub Copilot y Gemini CLI. Hearth alinea tus instrucciones, verifica MCP en vivo y mide tu gasto en local.",
      secondaryCtaLabel: "Leer también: guía de entorno dev",
      secondaryCtaTo: "/guides/dev-environment-setup/",
      presetsHeading: "Tres flujos de IA que Hearth simplifica",
      presets: [
        {
          pill: "Instrucciones compartidas",
          pillTone: "accent",
          title: "Un solo ~/.agents/AGENTS.md",
          desc: "Edita tus reglas globales una sola vez y enlázalas de forma segura a cada agente.",
          stackLines: [
            "~/.agents/AGENTS.md (fuente única)",
            "CLAUDE.md · CODEX · Gemini · Cursor",
            "Diff lado a lado antes de enlazar",
          ],
          foot: "Copia .bak automática antes de enlazar",
        },
        {
          pill: "Control MCP",
          pillTone: "info",
          title: "Prueba MCP en vivo y servidor integrado",
          desc: "Valida cada servidor stdio/HTTP MCP y expón diagnósticos de Hearth a tu agente.",
          stackLines: [
            "hearth mcp (servidor stdio local)",
            "Handshake en vivo y conteo de herramientas",
            "Alertas de claves API en texto plano",
          ],
          foot: "Aprobación humana para cualquier cambio",
        },
        {
          pill: "Skills y gasto",
          pillTone: "success",
          title: "Skills compartidas y registro de tokens",
          desc: "Instala skills desde GitHub y consulta el consumo local de 7/30/90 días.",
          stackLines: [
            "Catálogo compartido ~/.agents/skills",
            "Diff y sincronización de copias divergentes",
            "Desglose local de logs JSONL y costes",
          ],
          foot: "100 % local · sin subir datos a la nube",
        },
      ],
      stepsHeading: "Cómo organiza Hearth tus agentes IA",
      steps: [
        {
          step: "01 · Descubrir",
          title: "Localiza cada agente y config",
          desc: "Escaneo en solo lectura para Claude Code, Codex, Cursor, Copilot, Gemini y OpenCode.",
        },
        {
          step: "02 · Unificar",
          title: "Sincroniza AGENTS.md y skills",
          desc: "Compara reglas o skills divergentes lado a lado y conéctalas a una única fuente ~/.agents/.",
        },
        {
          step: "03 · Auditar",
          title: "Prueba MCP y detecta secretos",
          desc: "Verifica salud MCP en vivo, detecta conflictos entre agentes y mueve claves a variables de entorno.",
        },
        {
          step: "04 · Medir",
          title: "Controla el gasto local",
          desc: "Revisa tokens y coste estimado a 7, 30 y 90 días directamente desde los registros locales.",
        },
      ],
      pitfallsHeading: "Problemas de configuración IA que Hearth evita",
      pitfalls: [
        {
          title: "Reglas desincronizadas entre 4+ agentes",
          desc: "Actualizar CLAUDE.md y olvidar Cursor o Codex produce estilos inconsistentes. Un AGENTS.md enlazado lo evita.",
        },
        {
          title: "Servidores MCP que fallan en silencio",
          desc: "Rutas incorrectas de Node rompen MCP con errores vagos. Hearth prueba cada servidor y muestra el comando exacto.",
        },
        {
          title: "Claves expuestas en dotfiles",
          desc: "Pegar tokens en mcp.json arriesga filtraciones. Hearth enmascara secretos en informes y alerta sobre texto plano.",
        },
      ],
    },
  },
};
