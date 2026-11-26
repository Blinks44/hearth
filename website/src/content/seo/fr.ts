import type { SeoContentBundle } from "./types";

export const frSeoContent: SeoContentBundle = {
  alternatives: {
    "docker-desktop": {
      slug: "docker-desktop",
      path: "/alternatives/docker-desktop/",
      crumbLabel: "Docker Desktop",
      metaTitle: "Alternative gratuite à Docker Desktop (Colima & Go) · Hearth",
      metaDescription:
        "Remplacez Docker Desktop par Colima open source sur macOS ou Docker Engine natif sur Linux dans Hearth, une app Go native légère sans Electron.",
      heroTitle: "Une alternative native et gratuite à Docker Desktop sans Electron.",
      heroSubtitle:
        "Utilisez Colima open source sur macOS ou Docker Engine natif sur Linux. Inspectez piles Compose, conteneurs, images, volumes et logs en direct dans une app Go native sans Electron.",
      metrics: [
        { value: "~20 Mo téléch.", label: "Go natif & UI mygo · démarrage <80 ms · zéro Electron" },
        { value: "MIT Gratuit", label: "Sans licences payantes, sans compte, zéro télémétrie" },
        {
          value: "Colima + Linux",
          label: "VM open source sur macOS, Engine natif sur Linux, plus K8s & BDD",
        },
      ],
      hearthColumnLabel: "Hearth (Colima / Linux Engine)",
      otherColumnLabel: "Docker Desktop",
      rows: [
        {
          criterion: "Licence & tarif",
          hearth: "Gratuit & open source MIT pour particuliers et équipes commerciales",
          other: "Licences payantes requises au-delà du seuil entreprise",
        },
        {
          criterion: "Empreinte & vitesse",
          hearth: "Binaire Go natif unique (mygo), ~20 Mo à télécharger",
          other: "Interface Electron, souvent 1 Go+ de RAM au repos",
        },
        {
          criterion: "VM conteneurs macOS",
          hearth: "Colima open source en un clic avec réglages CPU/RAM/disque",
          other: "VM propriétaire intégrée dans Docker Desktop",
        },
        {
          criterion: "Workflow Linux",
          hearth: "Se connecte directement à Docker Engine natif (sans VM extra)",
          other: "Ajoute une couche VM supplémentaire sous Linux",
        },
        {
          criterion: "Au-delà de Docker",
          hearth: "Inclut UI Kubernetes, BDD locales, runtimes, IA/MCP & nettoyage",
          other: "Uniquement Docker & K8s mono-nœud intégré",
        },
        {
          criterion: "Vie privée & télémétrie",
          hearth: "Zéro télémétrie, aucun compte requis, 100 % local",
          other: "Invites de connexion & télémétrie activées par défaut",
        },
      ],
      chooseHearth: {
        pill: "Recommandé pour le dev local",
        title: "Choisissez Hearth quand…",
        points: [
          "Vous voulez une alternative MIT gratuite sans limites de postes commerciaux",
          "Vous voulez Colima sur macOS ou Docker Engine natif sur Linux avec une UI rapide",
          "Vous voulez Conteneurs, Kubernetes, BDD locales & agents IA dans une app Go native",
        ],
      },
      keepOther: {
        pill: "Quand Docker Desktop reste adapté",
        title: "Gardez Docker Desktop quand…",
        points: [
          "Votre entreprise impose Docker Business SSO et des politiques d’images",
          "Vous dépendez de plugins du marketplace Docker Extensions",
          "Vous avez besoin de Docker Scout ou Docker Build Cloud intégrés",
        ],
      },
      steps: [
        {
          step: "Étape 01",
          title: "Installez Hearth (sans changements silencieux)",
          desc: "Téléchargez Hearth (~20 Mo ; ~28 Mo DMG universel) via DMG, Homebrew ou .deb. Le premier scan est 100 % en lecture seule.",
        },
        {
          step: "Étape 02",
          title: "Configurez Colima ou connectez Linux Engine",
          desc: "Ouvrez Docker dans Hearth et validez le plan clair : Colima, Docker CLI, Compose & Buildx avec limites CPU/RAM.",
        },
        {
          step: "Étape 03",
          title: "Arrêtez Docker Desktop & libérez des Go",
          desc: "Vérifiez vos piles Compose dans Hearth puis nettoyez sereinement les anciens caches et disques VM.",
        },
      ],
    },
    "lens-freelens": {
      slug: "lens-freelens",
      path: "/alternatives/lens-freelens/",
      crumbLabel: "Lens & Freelens",
      metaTitle: "Alternative native rapide à Lens, Freelens & Headlamp · Hearth",
      metaDescription:
        "Inspectez pods Kubernetes, logs, diagnostics de crash, port-forwards, Secrets, Helm et clusters kind dans une UI Go client-go native sans Electron.",
      heroTitle: "Une interface Kubernetes native rapide, sans lourdeur Electron.",
      heroSubtitle:
        "Basée directement sur Go client-go. Inspectez pods, causes de crash, logs en direct, port-forwards, Helm, Secrets et clusters kind locaux dans une app Go native au lieu d’une lourde fenêtre Electron.",
      metrics: [
        { value: "client-go natif", label: "Lit kubeconfig directement avec démarrage <80 ms" },
        { value: "Zéro connexion", label: "Pas de Lens ID, pas de cloud, pas de télémétrie · MIT" },
        { value: "Stack complet", label: "Kubernetes avec Docker, BDD locales, runtimes & IA" },
      ],
      hearthColumnLabel: "Hearth (client-go natif)",
      otherColumnLabel: "Lens / Freelens / Headlamp",
      rows: [
        {
          criterion: "Moteur & mémoire",
          hearth: "Binaire Go natif unique (mygo), zéro Electron",
          other: "App Electron / Chromium (Lens, Freelens) ou interface web",
        },
        {
          criterion: "Compte & licence",
          hearth: "Open source MIT, aucun compte ni paywall",
          other: "Lens exige un compte & offres payantes ; Freelens/Headlamp sont OSS",
        },
        {
          criterion: "Débogage pods & crash",
          hearth: "Diagnostic CrashLoop, événements, logs live & shell exec",
          other: "Logs pods, événements, console & vues multi-panneaux",
        },
        {
          criterion: "Helm, Secrets & port-forward",
          hearth: "Gestionnaire port-forward intégré, lecture Secrets & liste Helm",
          other: "Disponible via vues intégrées ou extensions communautaires",
        },
        {
          criterion: "Clusters locaux kind",
          hearth: "Créez, démarrez, arrêtez et changez de cluster kind dans l’app",
          other: "Inspecte les clusters existants ; création via CLI séparée",
        },
        {
          criterion: "Sécurité des actions",
          hearth: "Aperçu explicite du plan avant redémarrage, suppression ou scaling",
          other: "Action immédiate ou simple fenêtre de confirmation",
        },
      ],
      chooseHearth: {
        pill: "Recommandé au quotidien",
        title: "Choisissez Hearth quand…",
        points: [
          "Vous voulez un démarrage instantané et peu de RAM entre clusters locaux et staging",
          "Vous voulez pods, diagnostic crash, logs live, port-forward, Helm & kind réunis",
          "Vous souhaitez remplacer des UI séparées pour Docker, Kubernetes et bases locales",
        ],
      },
      keepOther: {
        pill: "Quand Lens / Headlamp restent utiles",
        title: "Gardez Lens ou Headlamp quand…",
        points: [
          "Vous dépendez d’extensions spécifiques de l’écosystème Lens ou Headlamp",
          "Vous avez besoin de graphiques Prometheus intégrés pour de grands clusters de prod",
          "Vous préférez un tableau de bord web hébergé dans le cluster pour vos équipes plateforme",
        ],
      },
      steps: [
        {
          step: "Étape 01",
          title: "Ouvrez Hearth & détectez ~/.kube/config",
          desc: "Hearth lit vos contextes kubeconfig en lecture seule sans modifier votre contexte actif.",
        },
        {
          step: "Étape 02",
          title: "Inspectez pods, logs & port-forwards",
          desc: "Changez de namespace, diagnostiquez un CrashLoop ou redirigez un port local en un clic.",
        },
        {
          step: "Étape 03",
          title: "Lancez des clusters kind au besoin",
          desc: "Créez ou nettoyez vos clusters kind locaux à côté de vos conteneurs Colima / Docker Engine.",
        },
      ],
    },
    dbngin: {
      slug: "dbngin",
      path: "/alternatives/dbngin/",
      crumbLabel: "DBngin",
      metaTitle: "Alternative gratuite et multiplateforme à DBngin · Hearth",
      metaDescription:
        "Lancez PostgreSQL, Redis, MySQL, MariaDB et MongoDB sur macOS, Windows et Linux avec détection des conflits de ports et démarrage auto dans Hearth.",
      heroTitle: "Bases locales & ports sur macOS, Windows et Linux.",
      heroSubtitle:
        "Installez, démarrez et arrêtez PostgreSQL, Redis, MySQL, MariaDB et MongoDB en natif ou via Docker. Repérez les conflits de ports, réglez le démarrage auto et ouvrez psql ou redis-cli partout.",
      metrics: [
        {
          value: "3 plateformes",
          label: "macOS, Windows & Linux (contrairement à DBngin macOS uniquement)",
        },
        {
          value: "5 moteurs BDD",
          label: "PostgreSQL, Redis, MySQL, MariaDB & MongoDB natif ou Docker",
        },
        {
          value: "Ports maîtrisés",
          label: "Détection live des ports occupés avec processus & CLI rapide",
        },
      ],
      hearthColumnLabel: "Hearth (Bases & Ports)",
      otherColumnLabel: "DBngin / Postgres.app",
      rows: [
        {
          criterion: "Systèmes d’exploitation",
          hearth: "macOS, Windows & Linux dans une interface cohérente",
          other: "macOS uniquement (DBngin & Postgres.app)",
        },
        {
          criterion: "Moteurs pris en charge",
          hearth: "PostgreSQL, Redis, MySQL, MariaDB & MongoDB",
          other: "PostgreSQL, MySQL & Redis (DBngin) ou Postgres seul",
        },
        {
          criterion: "Natif & Docker",
          hearth: "Gère services Homebrew/système et bases Docker ensemble",
          other: "Binaires autonomes gérés uniquement",
        },
        {
          criterion: "Conflits de ports",
          hearth: "Montre quel processus occupe 5432, 6379 ou 3306 avant échec",
          other: "Sélection manuelle sans vue globale des ports utilisés",
        },
        {
          criterion: "Licence & code source",
          hearth: "100 % gratuit & open source MIT",
          other: "Gratuit mais code fermé (DBngin)",
        },
        {
          criterion: "Éditeur SQL",
          hearth: "Non inclus — fonctionne idéalement avec TablePlus, DataGrip & psql",
          other: "Non inclus — fonctionne avec TablePlus ou psql",
        },
      ],
      chooseHearth: {
        pill: "Recommandé pour les BDD locales",
        title: "Choisissez Hearth quand…",
        points: [
          "Vous travaillez sur macOS, Windows ou Linux et cherchez un outil open source",
          "Vous combinez services natifs et bases dans Docker Compose",
          "Vous voulez les alertes de ports et l’accès CLI rapide avec le reste de votre stack",
        ],
      },
      keepOther: {
        pill: "Quand DBngin / clients SQL restent adaptés",
        title: "Gardez DBngin ou TablePlus quand…",
        points: [
          "Vous avez besoin de versions mineures isolées côte à côte sur macOS sans Docker",
          "Vous cherchez un éditeur SQL visuel ou explorateur de tables (gardez TablePlus/DataGrip)",
          "Vous avez déjà des répertoires DBngin que vous ne souhaitez pas encore migrer",
        ],
      },
      steps: [
        {
          step: "Étape 01",
          title: "Scannez services & ports actifs",
          desc: "Hearth détecte automatiquement vos bases Homebrew, système et Docker sans rien redémarrer.",
        },
        {
          step: "Étape 02",
          title: "Installez ou gérez le démarrage auto",
          desc: "Ajoutez PostgreSQL, Redis, MySQL, MariaDB ou MongoDB via un plan explicite et contrôlez le lancement au démarrage.",
        },
        {
          step: "Étape 03",
          title: "Connectez votre client SQL ou CLI",
          desc: "Copiez les infos de connexion, lancez psql/redis-cli/mongosh et gardez TablePlus ou DataGrip pour vos requêtes.",
        },
      ],
    },
  },
  guides: {
    "dev-environment-setup": {
      slug: "dev-environment-setup",
      path: "/guides/dev-environment-setup/",
      crumbLabel: "Installation dev",
      metaTitle: "Guide de setup dev pour débutants & vibe coding · Hearth",
      metaDescription:
        "Installez Node.js, Python, Git, Docker, PostgreSQL et Claude Code ou Cursor sur macOS, Windows ou Linux en quelques minutes sans casser le PATH.",
      heroTitle: "Configurez un environnement dev propre en minutes, sans casser le PATH.",
      heroSubtitle:
        "Vous lancez votre premier projet avec Claude Code ou Cursor, ou vous préparez un nouveau Mac, Windows ou Linux ? Hearth installe runtimes, Docker, BDD et outils IA depuis un plan clair.",
      secondaryCtaLabel: "Lire aussi : guide Agents IA & MCP",
      secondaryCtaTo: "/guides/ai-agents-mcp/",
      presetsHeading: "Choisissez un préréglage prêt à l’emploi ou créez le vôtre",
      presets: [
        {
          pill: "Débutants & Vibe Coding",
          pillTone: "accent",
          title: "Vibe Coding Starter",
          desc: "Passez d’un ordinateur neuf à vos premiers projets IA en 10 minutes.",
          stackLines: [
            "Git + GitHub CLI · mise",
            "Node.js 22 LTS · Python 3.13 (uv)",
            "Claude Code · Cursor · AGENTS.md partagé",
          ],
          foot: "Zéro édition manuelle de ~/.zshrc",
        },
        {
          pill: "Web & Produit",
          pillTone: "info",
          title: "Full-Stack Web",
          desc: "Tout le nécessaire pour cloner des dépôts TypeScript & Python modernes.",
          stackLines: [
            "Homebrew/WinGet · mise",
            "Node 22 · Bun · pnpm · Python 3.13",
            "Docker + Colima · PostgreSQL · Redis",
          ],
          foot: "Respecte .nvmrc & mise.toml",
        },
        {
          pill: "Backend & Infra",
          pillTone: "success",
          title: "Backend & Cloud",
          desc: "Pour APIs, conteneurs et workflows Kubernetes en local.",
          stackLines: [
            "mise · Go 1.27 · Rust",
            "Docker + Colima · kubectl · kind",
            "PostgreSQL 16 · Redis · Terraform",
          ],
          foot: "Exportable en hearth-setup.json",
        },
      ],
      stepsHeading: "Comment fonctionne l’installation guidée",
      steps: [
        {
          step: "01 · Choisir",
          title: "Sélectionnez votre stack",
          desc: "Choisissez Vibe Coding, Web ou Backend, ou importez le preset JSON de votre équipe.",
        },
        {
          step: "02 · Plan",
          title: "Vérifiez le diff exact",
          desc: "Consultez chaque commande et bloc shell avant exécution avec sauvegarde .bak incluse.",
        },
        {
          step: "03 · Installer",
          title: "Exécutez avec logs en direct",
          desc: "Installe via Homebrew, WinGet, mise & Colima dans l’ordre des dépendances avec rollback si besoin.",
        },
        {
          step: "04 · Vérifier",
          title: "Testez dans un shell propre",
          desc: "Hearth ouvre un login shell neuf pour confirmer la résolution de chaque binaire dans le bon PATH.",
        },
      ],
      pitfallsHeading: "Erreurs de configuration évitées par Hearth",
      pitfalls: [
        {
          title: "PATH cassé & zsh lent",
          desc: "Mélanger Node Homebrew, nvm et pyenv crée conflits et lenteurs au démarrage. Hearth privilégie mise et trace which -a.",
        },
        {
          title: "Clés API en clair dans MCP",
          desc: "En connectant des agents IA, les clés finissent souvent dans des JSON. Hearth les signale et propose des variables d’environnement.",
        },
        {
          title: "Écrasements silencieux de configs",
          desc: "Des scripts modifient ~/.zshrc sans sauvegarde. Hearth édite uniquement son bloc dédié et garde des copies .bak.",
        },
      ],
    },
    "ai-agents-mcp": {
      slug: "ai-agents-mcp",
      path: "/guides/ai-agents-mcp/",
      crumbLabel: "Agents IA & MCP",
      metaTitle: "Gérer Claude Code, Codex, Cursor, AGENTS.md & MCP au même endroit · Hearth",
      metaDescription:
        "Unifiez AGENTS.md pour tous vos agents IA, partagez vos skills, testez les serveurs MCP en direct et suivez vos dépenses locales dans Hearth.",
      heroTitle: "Unifiez AGENTS.md, skills, serveurs MCP et dépenses IA locales.",
      heroSubtitle:
        "Arrêtez de jongler entre cinq fichiers cachés pour Claude Code, Codex CLI, Cursor, GitHub Copilot et Gemini CLI. Hearth garde vos règles synchronisées, sonde MCP en direct et suit vos tokens en local.",
      secondaryCtaLabel: "Lire aussi : guide d’installation dev",
      secondaryCtaTo: "/guides/dev-environment-setup/",
      presetsHeading: "Trois workflows IA simplifiés par Hearth",
      presets: [
        {
          pill: "Instructions partagées",
          pillTone: "accent",
          title: "Un seul ~/.agents/AGENTS.md",
          desc: "Modifiez vos règles globales une fois et liez-les proprement à chaque agent.",
          stackLines: [
            "~/.agents/AGENTS.md (source unique)",
            "CLAUDE.md · CODEX · Gemini · Cursor",
            "Diff côte à côte avant création de lien",
          ],
          foot: "Sauvegarde .bak automatique avant lien",
        },
        {
          pill: "Contrôle MCP",
          pillTone: "info",
          title: "Test MCP live & serveur intégré",
          desc: "Vérifiez chaque serveur stdio/HTTP MCP et exposez le diagnostic Hearth à votre agent.",
          stackLines: [
            "hearth mcp (serveur stdio local)",
            "Handshake live & nombre d’outils",
            "Alertes clés API en texte clair",
          ],
          foot: "Validation humaine pour chaque correctif",
        },
        {
          pill: "Skills & budget",
          pillTone: "success",
          title: "Skills partagées & suivi tokens",
          desc: "Installez des skills depuis GitHub et suivez votre estimation locale sur 7/30/90 jours.",
          stackLines: [
            "Catalogue partagé ~/.agents/skills",
            "Diff & synchro des copies divergentes",
            "Analyse locale JSONL & estimation coûts",
          ],
          foot: "100 % local · zéro envoi cloud",
        },
      ],
      stepsHeading: "Comment Hearth organise votre setup IA",
      steps: [
        {
          step: "01 · Découvrir",
          title: "Trouvez chaque agent & config",
          desc: "Scan en lecture seule pour Claude Code, Codex, Cursor, Copilot, Gemini et OpenCode.",
        },
        {
          step: "02 · Unifier",
          title: "Synchronisez AGENTS.md & skills",
          desc: "Comparez vos fichiers divergents et reliez-les à la source unique ~/.agents/.",
        },
        {
          step: "03 · Auditer",
          title: "Sondez MCP & détectez les secrets",
          desc: "Testez vos serveurs MCP, repérez les conflits entre agents et passez les clés en variables d’env.",
        },
        {
          step: "04 · Suivre",
          title: "Surveillez les dépenses locales",
          desc: "Consultez tokens et coûts estimés sur 7, 30 et 90 jours directement depuis vos logs locaux.",
        },
      ],
      pitfallsHeading: "Pièges de config IA évités par Hearth",
      pitfalls: [
        {
          title: "Règles désynchronisées entre 4+ agents",
          desc: "Mettre à jour CLAUDE.md en oubliant Cursor ou Codex crée des écarts de style. Un seul AGENTS.md lié résout le souci.",
        },
        {
          title: "Serveurs MCP cassés en silence",
          desc: "Un mauvais chemin Node casse MCP avec des erreurs floues. Hearth sonde chaque serveur et montre la commande résolue.",
        },
        {
          title: "Clés exposées dans les dotfiles",
          desc: "Coller un token dans mcp.json expose vos secrets. Hearth masque les valeurs sensibles et alerte sur le texte clair.",
        },
      ],
    },
  },
};
