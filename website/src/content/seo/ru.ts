import type { SeoContentBundle } from "./types";

export const ruSeoContent: SeoContentBundle = {
  alternatives: {
    "docker-desktop": {
      slug: "docker-desktop",
      path: "/alternatives/docker-desktop/",
      crumbLabel: "Docker Desktop",
      metaTitle: "Бесплатная альтернатива Docker Desktop (Colima и Go) · Hearth",
      metaDescription:
        "Замените Docker Desktop на открытый Colima на macOS или нативный Docker Engine на Linux в Hearth - нативном Go-приложении на ~18 МБ без Electron.",
      heroTitle: "Бесплатная нативная альтернатива Docker Desktop без Electron.",
      heroSubtitle:
        "Запускайте открытый Colima на macOS или нативный Docker Engine на Linux. Управляйте Compose-стеками, контейнерами, образами, томами и логами в одном приложении на ~18 МБ без Electron.",
      metrics: [
        {
          value: "~18 МБ бинарник",
          label: "Нативный Go и mygo UI · старт <80 мс · без Electron",
        },
        {
          value: "Бесплатно · MIT",
          label: "Без платных лицензий, без входа в аккаунт и без телеметрии",
        },
        {
          value: "Colima + Linux",
          label: "Открытая VM на macOS, нативный Engine на Linux, плюс K8s и БД",
        },
      ],
      hearthColumnLabel: "Hearth (Colima / Linux Engine)",
      otherColumnLabel: "Docker Desktop",
      rows: [
        {
          criterion: "Лицензия и цена",
          hearth: "Бесплатно и MIT Open Source для личной и коммерческой работы",
          other: "Платные лицензии для компаний выше порога",
        },
        {
          criterion: "Размер и скорость UI",
          hearth: "~18 МБ нативный Go-бинарник (mygo), холодный старт <80 мс",
          other: "Electron UI, часто 1 ГБ+ памяти в простое",
        },
        {
          criterion: "VM контейнеров на macOS",
          hearth: "Открытый Colima в один клик с настройкой CPU/RAM/диска",
          other: "Проприетарная VM внутри Docker Desktop",
        },
        {
          criterion: "Работа на Linux",
          hearth: "Подключается напрямую к нативному Docker Engine без лишней VM",
          other: "Запускает дополнительный слой VM на Linux",
        },
        {
          criterion: "Помимо Docker",
          hearth: "Встроенные Kubernetes UI, локальные БД, рантаймы, AI/MCP и очистка",
          other: "Только Docker и встроенный одноузловой K8s",
        },
        {
          criterion: "Приватность и телеметрия",
          hearth: "Ноль телеметрии, без аккаунта, на 100% локально",
          other: "Запросы входа в аккаунт и телеметрия по умолчанию",
        },
      ],
      chooseHearth: {
        pill: "Оптимально для локальной разработки",
        title: "Выбирайте Hearth, если…",
        points: [
          "Нужна бесплатная MIT-альтернатива без корпоративных ограничений по местам",
          "Хотите Colima на macOS или нативный Docker Engine на Linux с быстрым интерфейсом",
          "Хотите контейнеры, Kubernetes, локальные БД и AI-агентов в одном приложении на ~18 МБ",
        ],
      },
      keepOther: {
        pill: "Когда Docker Desktop всё ещё нужен",
        title: "Оставляйте Docker Desktop, если…",
        points: [
          "В компании обязательны Docker Business SSO и централизованные политики образов",
          "Вы активно пользуетесь плагинами из Docker Extensions Marketplace",
          "Нужна тесная встроенная интеграция с Docker Scout или Docker Build Cloud",
        ],
      },
      steps: [
        {
          step: "Шаг 01",
          title: "Установите Hearth (без фоновых правок)",
          desc: "Скачайте нативный бинарник ~18 МБ через DMG, Homebrew или .deb. Первое сканирование полностью read-only и не трогает контейнеры.",
        },
        {
          step: "Шаг 02",
          title: "Настройте Colima или Linux Engine",
          desc: "Откройте раздел Docker в Hearth и проверьте явный план установки: Colima, Docker CLI, Compose и Buildx с лимитами CPU/RAM.",
        },
        {
          step: "Шаг 03",
          title: "Остановите Docker Desktop и освободите гигабайты",
          desc: "Убедитесь, что Compose-стеки и контексты работают в Hearth, затем безопасно очистите старые кэши и диск прежней VM.",
        },
      ],
    },
    "lens-freelens": {
      slug: "lens-freelens",
      path: "/alternatives/lens-freelens/",
      crumbLabel: "Lens и Freelens",
      metaTitle: "Быстрая нативная альтернатива Lens, Freelens и Headlamp · Hearth",
      metaDescription:
        "Поды Kubernetes, логи, диагностика крэшей, port-forward, Secrets, Helm и локальные кластеры kind в нативном Go-приложении на ~18 МБ без Electron.",
      heroTitle: "Быстрый нативный Kubernetes UI без тормозов Electron.",
      heroSubtitle:
        "Написан напрямую поверх Go client-go. Смотрите поды, причины падений, живые логи, port-forward, релизы Helm, Secrets и локальные кластеры kind в одном приложении на ~18 МБ вместо гигабайтного окна Electron.",
      metrics: [
        {
          value: "Нативный client-go",
          label: "Читает kubeconfig напрямую · холодный старт <80 мс",
        },
        {
          value: "Без аккаунта",
          label: "Без Lens ID, без облачной синхронизации и телеметрии · MIT",
        },
        {
          value: "Весь локальный стек",
          label: "Kubernetes рядом с Docker, локальными БД, рантаймами и AI",
        },
      ],
      hearthColumnLabel: "Hearth (Нативный client-go)",
      otherColumnLabel: "Lens / Freelens / Headlamp",
      rows: [
        {
          criterion: "Движок и память",
          hearth: "~18 МБ нативный Go-бинарник (mygo), минимальный расход RAM",
          other: "Electron / Chromium приложение (Lens, Freelens) или браузерный UI",
        },
        {
          criterion: "Аккаунт и лицензия",
          hearth: "MIT Open Source, без регистрации и пейволлов",
          other: "Lens требует аккаунт и платные тарифы; Freelens/Headlamp - OSS",
        },
        {
          criterion: "Отладка подов и крэшей",
          hearth: "Диагностика крэшей, события, живые логи подов/контроллеров и shell",
          other: "Логи подов, события, консоль и многопанельные экраны",
        },
        {
          criterion: "Helm, Secrets и port-forward",
          hearth: "Встроенный менеджер port-forward, просмотр Secrets и список Helm",
          other: "Доступно через встроенные экраны или сторонние расширения",
        },
        {
          criterion: "Локальные кластеры kind",
          hearth: "Создание, запуск, остановка и переключение кластеров kind прямо в UI",
          other: "Только просмотр существующих кластеров; создание через CLI",
        },
        {
          criterion: "Безопасность действий",
          hearth: "Явный предпросмотр плана перед перезапуском, удалением или масштабированием",
          other: "Мгновенное действие или обычное окно подтверждения",
        },
      ],
      chooseHearth: {
        pill: "Оптимально для ежедневной разработки",
        title: "Выбирайте Hearth, если…",
        points: [
          "Нужен мгновенный старт и малый расход памяти при переключении между локальными и staging-кластерами",
          "Важны поды, диагностика CrashLoop, живые логи, port-forward, Helm и kind в одном окне",
          "Хотите убрать отдельные UI для Docker, Kubernetes и локальных баз данных",
        ],
      },
      keepOther: {
        pill: "Когда Lens / Headlamp всё ещё лучше",
        title: "Оставляйте Lens или Headlamp, если…",
        points: [
          "Вы завязаны на конкретные плагины экосистемы Lens или Headlamp",
          "Нужны встроенные графики Prometheus по большим production-кластерам",
          "Нужен веб-дашборд внутри самого кластера для платформенной команды",
        ],
      },
      steps: [
        {
          step: "Шаг 01",
          title: "Откройте Hearth и просканируйте ~/.kube/config",
          desc: "Hearth находит все контексты kubeconfig в режиме read-only, не меняя активный контекст и никуда ничего не отправляя.",
        },
        {
          step: "Шаг 02",
          title: "Проверяйте поды, логи и port-forward",
          desc: "Переключайте namespace, сразу открывайте диагностику CrashLoop, стримьте логи и пробрасывайте локальные порты в один клик.",
        },
        {
          step: "Шаг 03",
          title: "Поднимайте локальные кластеры kind по требованию",
          desc: "Создавайте и удаляйте локальные кластеры kind рядом с контейнерами Colima / Docker Engine, не выходя из Hearth.",
        },
      ],
    },
    dbngin: {
      slug: "dbngin",
      path: "/alternatives/dbngin/",
      crumbLabel: "DBngin",
      metaTitle: "Бесплатная кроссплатформенная альтернатива DBngin · Hearth",
      metaDescription:
        "Запускайте локальные PostgreSQL, Redis, MySQL, MariaDB и MongoDB на macOS, Windows и Linux с проверкой конфликтов портов и автозапуском в Hearth.",
      heroTitle: "Локальные базы данных и порты на macOS, Windows и Linux.",
      heroSubtitle:
        "Устанавливайте и управляйте PostgreSQL, Redis, MySQL, MariaDB и MongoDB локально или в Docker. Находите конфликты портов, управляйте автозапуском и открывайте psql или redis-cli без привязки только к macOS.",
      metrics: [
        {
          value: "3 платформы",
          label: "macOS, Windows и Linux (в отличие от DBngin только под macOS)",
        },
        {
          value: "5 движков БД",
          label: "PostgreSQL, Redis, MySQL, MariaDB и MongoDB локально или в Docker",
        },
        {
          value: "Контроль портов",
          label: "Поиск занятых портов с процессом и быстрый запуск CLI",
        },
      ],
      hearthColumnLabel: "Hearth (БД и порты)",
      otherColumnLabel: "DBngin / Postgres.app",
      rows: [
        {
          criterion: "Операционные системы",
          hearth: "macOS, Windows и Linux в едином интерфейсе",
          other: "Только macOS (DBngin и Postgres.app)",
        },
        {
          criterion: "Поддерживаемые движки",
          hearth: "PostgreSQL, Redis, MySQL, MariaDB и MongoDB",
          other: "PostgreSQL, MySQL и Redis (DBngin) или только Postgres",
        },
        {
          criterion: "Нативные и Docker-сервисы",
          hearth: "Управляет сервисами Homebrew/ОС и БД в Docker в одном списке",
          other: "Только собственные изолированные бинарники",
        },
        {
          criterion: "Конфликты портов",
          hearth: "Показывает, какой процесс занял 5432, 6379 или 3306 до ошибки запуска",
          other: "Ручной выбор порта без общего обзора занятых портов",
        },
        {
          criterion: "Лицензия и исходники",
          hearth: "100% бесплатно и MIT Open Source",
          other: "Бесплатно, но закрытый код (DBngin)",
        },
        {
          criterion: "SQL-редактор",
          hearth: "Не встроен - отлично работает в паре с TablePlus, DataGrip и psql",
          other: "Не встроен - работает в паре с TablePlus или psql",
        },
      ],
      chooseHearth: {
        pill: "Оптимально для локальных БД",
        title: "Выбирайте Hearth, если…",
        points: [
          "Вы работаете на macOS, Windows или Linux и хотите один открытый инструмент",
          "Используете одновременно локальные сервисы и базы данных в Docker Compose",
          "Нужны предупреждения о конфликтах портов и быстрый запуск CLI рядом с остальным стеком",
        ],
      },
      keepOther: {
        pill: "Когда DBngin и SQL-клиенты всё ещё нужны",
        title: "Оставляйте DBngin или TablePlus, если…",
        points: [
          "Вам нужны изолированные параллельные минорные сборки БД на macOS без Docker",
          "Нужен визуальный редактор SQL-запросов и таблиц (оставьте TablePlus или DataGrip)",
          "У вас уже настроены каталоги данных DBngin, которые вы пока не хотите переносить",
        ],
      },
      steps: [
        {
          step: "Шаг 01",
          title: "Просканируйте запущенные сервисы и порты",
          desc: "Hearth автоматически находит существующие сервисы Homebrew, ОС и Docker, не перезапуская их и не трогая данные.",
        },
        {
          step: "Шаг 02",
          title: "Установите БД или настройте автозапуск",
          desc: "Добавьте PostgreSQL, Redis, MySQL, MariaDB или MongoDB через явный план установки и управляйте автозапуском каждого сервиса.",
        },
        {
          step: "Шаг 03",
          title: "Подключите SQL-клиент или CLI",
          desc: "Скопируйте параметры подключения, откройте psql/redis-cli/mongosh и продолжайте писать запросы в TablePlus или DataGrip.",
        },
      ],
    },
  },
  guides: {
    "dev-environment-setup": {
      slug: "dev-environment-setup",
      path: "/guides/dev-environment-setup/",
      crumbLabel: "Настройка среды разработки",
      metaTitle: "Гайд по настройке среды разработки для новичков и вайбкодинга · Hearth",
      metaDescription:
        "Установите Node.js, Python, Git, Docker, PostgreSQL и Claude Code или Cursor на macOS, Windows и Linux за минуты, не сломав PATH.",
      heroTitle: "Настройте чистую среду разработки за минуты, не сломав PATH.",
      heroSubtitle:
        "Запускаете первый проект с Claude Code или Cursor, или настраиваете новый Mac, Windows или Linux? Hearth установит языки, Docker, базы данных и AI-инструменты по одному прозрачному плану.",
      secondaryCtaLabel: "Читайте также: гайд по AI-агентам и MCP",
      secondaryCtaTo: "/guides/ai-agents-mcp/",
      presetsHeading: "Выберите готовый пресет или соберите свой",
      presets: [
        {
          pill: "Новичкам и вайбкодинг",
          pillTone: "accent",
          title: "Старт для вайбкодинга",
          desc: "От чистого ноутбука до первых проектов с AI-агентами за 10 минут.",
          stackLines: [
            "Git + GitHub CLI · mise",
            "Node.js 22 LTS · Python 3.13 (uv)",
            "Claude Code · Cursor · единый AGENTS.md",
          ],
          foot: "Без ручной правки ~/.zshrc",
        },
        {
          pill: "Веб и продукт",
          pillTone: "info",
          title: "Full-Stack Web",
          desc: "Всё необходимое для современных репозиториев на TypeScript и Python.",
          stackLines: [
            "Homebrew/WinGet · mise",
            "Node 22 · Bun · pnpm · Python 3.13",
            "Docker + Colima · PostgreSQL · Redis",
          ],
          foot: "Учитывает .nvmrc и mise.toml",
        },
        {
          pill: "Бэкенд и инфра",
          pillTone: "success",
          title: "Backend & Cloud",
          desc: "Для API, контейнеров и локальных сценариев Kubernetes.",
          stackLines: [
            "mise · Go 1.27 · Rust",
            "Docker + Colima · kubectl · kind",
            "PostgreSQL 16 · Redis · Terraform",
          ],
          foot: "Экспорт в hearth-setup.json",
        },
      ],
      stepsHeading: "Как работает пошаговая настройка",
      steps: [
        {
          step: "01 · Выбор",
          title: "Выберите свой стек",
          desc: "Возьмите пресет Vibe Coding, Web или Backend либо импортируйте JSON-пресет от команды.",
        },
        {
          step: "02 · План",
          title: "Проверьте точный diff",
          desc: "Посмотрите каждую команду и изменение блока шелла до запуска. Автоматический бэкап .bak включен.",
        },
        {
          step: "03 · Установка",
          title: "Запуск с живыми логами",
          desc: "Установка идет через Homebrew, WinGet, mise и Colima в верном порядке зависимостей с откатом при ошибке.",
        },
        {
          step: "04 · Проверка",
          title: "Тест в чистом шелле",
          desc: "Hearth открывает чистый login shell и проверяет, что каждый бинарник находится в нужном порядке PATH.",
        },
      ],
      pitfallsHeading: "Частые ошибки настройки, которых Hearth помогает избежать",
      pitfalls: [
        {
          title: "Сломанный PATH и медленный zsh",
          desc: "Смешивание Homebrew node, nvm и pyenv вызывает конфликты версий и задержки терминала. Hearth ставит быстрый mise и проверяет which -a.",
        },
        {
          title: "API-ключи открытым текстом в MCP",
          desc: "При подключении AI-агентов ключи часто попадают в JSON открытым текстом. Hearth находит их и предлагает безопасный перенос в переменные среды.",
        },
        {
          title: "Незаметная перезапись конфигов",
          desc: "Случайные скрипты дописывают строки в ~/.zshrc без бэкапов. Hearth правит только свой маркерный блок и сохраняет копии .bak.",
        },
      ],
    },
    "ai-agents-mcp": {
      slug: "ai-agents-mcp",
      path: "/guides/ai-agents-mcp/",
      crumbLabel: "AI-агенты и MCP",
      metaTitle: "Управление Claude Code, Codex, Cursor, AGENTS.md и MCP в Hearth",
      metaDescription:
        "Объедините AGENTS.md для всех AI-агентов, делитесь скиллами, проверяйте MCP-серверы, находите утечки ключей и отслеживайте локальный расход токенов.",
      heroTitle: "Единый AGENTS.md, скиллы, MCP-серверы и локальный расход AI.",
      heroSubtitle:
        "Хватит вручную синхронизировать пять скрытых файлов настроек между Claude Code, Codex CLI, Cursor, GitHub Copilot и Gemini CLI. Hearth держит инструкции общими, проверяет MCP вживую и считает расход токенов локально.",
      secondaryCtaLabel: "Читайте также: гайд по настройке среды",
      secondaryCtaTo: "/guides/dev-environment-setup/",
      presetsHeading: "Три AI-сценария, которые упрощает Hearth",
      presets: [
        {
          pill: "Общие инструкции",
          pillTone: "accent",
          title: "Один ~/.agents/AGENTS.md",
          desc: "Редактируйте глобальные правила один раз и безопасно линковать их во все установленные агенты.",
          stackLines: [
            "~/.agents/AGENTS.md (единый источник)",
            "CLAUDE.md · CODEX · Gemini · Cursor",
            "Сравнение diff перед созданием симлинка",
          ],
          foot: "Автоматический бэкап .bak перед заменой",
        },
        {
          pill: "Контроль MCP",
          pillTone: "info",
          title: "Живая проверка и встроенный MCP",
          desc: "Тестируйте каждый stdio/HTTP MCP-сервер и подключите диагностику Hearth прямо к вашему агенту.",
          stackLines: [
            "hearth mcp (локальный stdio-сервер)",
            "Живой handshake и подсчет инструментов",
            "Предупреждения об открытых API-ключах",
          ],
          foot: "Любые правки требуют вашего подтверждения",
        },
        {
          pill: "Скиллы и расход",
          pillTone: "success",
          title: "Общие скиллы и учет токенов",
          desc: "Ставьте скиллы из GitHub-каталогов и смотрите локальную статистику токенов и стоимости за 7/30/90 дней.",
          stackLines: [
            "Общий каталог ~/.agents/skills",
            "Diff и синхронизация расходящихся копий",
            "Локальный разбор JSONL-логов и затрат",
          ],
          foot: "100% локально · без отправки в облако",
        },
      ],
      stepsHeading: "Как Hearth наводит порядок в AI-настройках",
      steps: [
        {
          step: "01 · Поиск",
          title: "Находит всех агентов и конфиги",
          desc: "Read-only сканирование автоматически находит настройки Claude Code, Codex, Cursor, Copilot, Gemini и OpenCode.",
        },
        {
          step: "02 · Единство",
          title: "Синхронизирует AGENTS.md и скиллы",
          desc: "Сравнивайте расходящиеся инструкции и скиллы бок о бок и переводите их на единый источник ~/.agents/.",
        },
        {
          step: "03 · Аудит",
          title: "Проверяет MCP и ищет секреты",
          desc: "Запускайте живые проверки MCP, находите конфликты команд между агентами и выносите ключи в переменные окружения.",
        },
        {
          step: "04 · Учет",
          title: "Показывает локальный расход",
          desc: "Смотрите расход токенов и оценку стоимости за 7, 30 и 90 дней напрямую по локальным логам сессий.",
        },
      ],
      pitfallsHeading: "Проблемы с конфигами AI, от которых защищает Hearth",
      pitfalls: [
        {
          title: "Разъехавшиеся правила в 4+ агентах",
          desc: "Правите CLAUDE.md, но забываете обновить правила для Cursor или Codex. Один симлинкованный AGENTS.md держит всё в синхроне.",
        },
        {
          title: "Молча падающие MCP-серверы",
          desc: "Неверный путь к Node или отсутствующий бинарник ломают MCP с неясными ошибками. Hearth проверяет каждый сервер и показывает точную команду.",
        },
        {
          title: "Ключи открытым текстом в dotfiles",
          desc: "Вставка токенов напрямую в mcp.json создает риск случайного коммита. Hearth маскирует секреты в отчетах и подсвечивает их в конфигах.",
        },
      ],
    },
  },
};
