import type { SeoContentBundle } from "./types";

export const zhHansSeoContent: SeoContentBundle = {
  alternatives: {
    "docker-desktop": {
      slug: "docker-desktop",
      path: "/alternatives/docker-desktop/",
      crumbLabel: "Docker Desktop",
      metaTitle: "免费原生 Docker Desktop 替代方案（Colima 与 Go）· Hearth",
      metaDescription:
        "使用 Hearth 在 macOS 上一键管理开源 Colima，或在 Linux 上直连原生 Docker Engine。轻量原生 Go 应用，MIT 开源，零 Electron 臃肿。",
      heroTitle: "免费、原生的 Docker Desktop 替代方案，彻底告别 Electron。",
      heroSubtitle:
        "在 macOS 上运行开源 Colima，或在 Linux 上直连原生 Docker Engine。在一个原生 Go 应用中管理 Compose 栈、容器、镜像、卷与实时日志。",
      metrics: [
        { value: "~13–19 MB 安装包", label: "原生 Go 与 mygo 界面 · <80 ms 冷启动 · 零 Electron" },
        { value: "MIT 免费开源", label: "无商业席位限制，无需登录账号，零遥测" },
        {
          value: "Colima + Linux",
          label: "macOS 开源虚拟机、Linux 原生 Engine，并内置 K8s 与数据库",
        },
      ],
      hearthColumnLabel: "Hearth（Colima / Linux Engine）",
      otherColumnLabel: "Docker Desktop",
      rows: [
        {
          criterion: "协议与定价",
          hearth: "个人与商业团队均免费，MIT 开源协议",
          other: "超过企业规模门槛需要购买付费席位",
        },
        {
          criterion: "体积与启动速度",
          hearth: "单一原生 Go 二进制（mygo），约 13–19 MB 下载包",
          other: "Electron 桌面界面，空载内存常超过 1 GB",
        },
        {
          criterion: "macOS 容器虚拟机",
          hearth: "一键配置开源 Colima，支持 CPU/内存/磁盘调节",
          other: "内置于 Docker Desktop 的闭源虚拟机",
        },
        {
          criterion: "Linux 工作流",
          hearth: "直连原生 Docker Engine，无需额外虚拟机层",
          other: "在 Linux 上仍运行额外虚拟机层",
        },
        {
          criterion: "容器之外的能力",
          hearth: "自带 Kubernetes 界面、本地数据库、运行时、AI/MCP 与磁盘清理",
          other: "仅提供 Docker 与内置单节点 K8s",
        },
        {
          criterion: "隐私与遥测",
          hearth: "零遥测、免账号登录、100% 本地运行",
          other: "默认提示登录并启用使用遥测",
        },
      ],
      chooseHearth: {
        pill: "适合日常本地开发",
        title: "选择 Hearth，如果你…",
        points: [
          "需要免费且无商业席位限制的 MIT 开源替代方案",
          "希望在 macOS 上使用 Colima 或在 Linux 上直连原生 Docker Engine 并拥有极速界面",
          "希望用单一原生 Go 应用统一管理容器、Kubernetes、本地数据库与 AI 代理",
        ],
      },
      keepOther: {
        pill: "何时仍适合 Docker Desktop",
        title: "继续使用 Docker Desktop，如果你…",
        points: [
          "企业 IT 强制要求 Docker Business SSO 与镜像合规策略",
          "深度依赖 Docker Extensions 商店中的特定插件",
          "需要深度集成的 Docker Scout 或 Docker Build Cloud",
        ],
      },
      steps: [
        {
          step: "步骤 01",
          title: "安装 Hearth（零后台修改）",
          desc: "通过 DMG、Homebrew 或 .deb 下载 Hearth（各架构约 13–19 MB）。首次扫描完全只读，不会改动现有容器。",
        },
        {
          step: "步骤 02",
          title: "配置 Colima 或连接 Linux Engine",
          desc: "打开 Hearth 的 Docker 页面并审阅明确计划：Colima、Docker CLI、Compose 与 Buildx 及 CPU/内存设置。",
        },
        {
          step: "步骤 03",
          title: "停用 Docker Desktop 并释放空间",
          desc: "确认 Compose 栈与上下文运行正常后，安全清理陈旧构建缓存与旧虚拟机磁盘文件。",
        },
      ],
    },
    "lens-freelens": {
      slug: "lens-freelens",
      path: "/alternatives/lens-freelens/",
      crumbLabel: "Lens 与 Freelens",
      metaTitle: "高速原生 Lens、Freelens 与 Headlamp 替代方案 · Hearth",
      metaDescription:
        "基于原生 Go client-go 查看 Kubernetes Pod、崩溃诊断、实时日志、端口转发、Secrets、Helm 与 kind 集群，原生轻量，告别 Electron 卡顿。",
      heroTitle: "高速原生 Kubernetes 界面，彻底告别 Electron 卡顿。",
      heroSubtitle:
        "直接构建于 Go client-go 之上。在一个原生 Go 应用中查看 Pod、崩溃原因、实时日志、端口转发、Helm、Secrets 与本地 kind 集群，无需开启 1 GB 内存的 Electron 窗口。",
      metrics: [
        { value: "原生 client-go", label: "直接读取 kubeconfig，冷启动 <80 ms" },
        { value: "免登录", label: "无需 Lens ID、无云同步、无遥测，MIT 开源" },
        { value: "一体化本地栈", label: "Kubernetes 与 Docker、本地数据库、运行时及 AI 共处一窗" },
      ],
      hearthColumnLabel: "Hearth（原生 client-go）",
      otherColumnLabel: "Lens / Freelens / Headlamp",
      rows: [
        {
          criterion: "运行架构与内存",
          hearth: "单一原生 Go 二进制（mygo），零 Electron",
          other: "Electron / Chromium 应用（Lens、Freelens）或浏览器界面",
        },
        {
          criterion: "账号与协议",
          hearth: "MIT 开源，无账号墙与付费墙",
          other: "Lens 需账号与付费套餐；Freelens/Headlamp 为开源项目",
        },
        {
          criterion: "Pod 与崩溃排查",
          hearth: "内置崩溃诊断、事件、实时 Pod/控制器日志与 exec 终端",
          other: "提供 Pod 日志、事件、终端与多面板视图",
        },
        {
          criterion: "Helm、Secrets 与端口转发",
          hearth: "内置端口转发管理、Secrets 查看与 Helm 列表",
          other: "通过内置视图或社区插件提供",
        },
        {
          criterion: "本地 kind 集群",
          hearth: "应用内直接创建、启停与切换本地 kind 集群",
          other: "主要查看已有集群，创建需借助外部 CLI",
        },
        {
          criterion: "变更安全性",
          hearth: "重启、删除或扩缩容前展示明确计划预览",
          other: "直接执行或简单弹窗确认",
        },
      ],
      chooseHearth: {
        pill: "适合日常开发调试",
        title: "选择 Hearth，如果你…",
        points: [
          "希望在本地与测试集群之间秒级切换，且内存占用极低",
          "需要 Pod、CrashLoop 诊断、实时日志、端口转发、Helm 与 kind 集成在一处",
          "希望告别同时开启独立的 Docker、Kubernetes 和本地数据库客户端",
        ],
      },
      keepOther: {
        pill: "何时仍适合 Lens / Headlamp",
        title: "继续使用 Lens 或 Headlamp，如果你…",
        points: [
          "依赖特定 Lens 或 Headlamp 生态插件",
          "需要在超大规模生产集群上查看内置 Prometheus 指标图表",
          "需要面向平台团队的集群内 Web 控制台",
        ],
      },
      steps: [
        {
          step: "步骤 01",
          title: "打开 Hearth 自动识别 ~/.kube/config",
          desc: "以只读方式扫描所有 kubeconfig 上下文，不修改当前上下文，也不上传任何数据。",
        },
        {
          step: "步骤 02",
          title: "排查 Pod、日志与端口转发",
          desc: "切换命名空间、直达 CrashLoop 原因诊断、实时查看日志或一键建立本地端口转发。",
        },
        {
          step: "步骤 03",
          title: "按需拉起本地 kind 集群",
          desc: "在 Colima / Docker Engine 容器旁边直接创建或清理本地 kind 集群，无需切换工具。",
        },
      ],
    },
    dbngin: {
      slug: "dbngin",
      path: "/alternatives/dbngin/",
      crumbLabel: "DBngin",
      metaTitle: "免费跨平台 DBngin 替代方案（macOS / Windows / Linux）· Hearth",
      metaDescription:
        "在 macOS、Windows 与 Linux 上管理本地 PostgreSQL、Redis、MySQL、MariaDB 与 MongoDB，支持端口冲突检测与开机自启控制。",
      heroTitle: "跨 macOS、Windows 与 Linux 的本地数据库与端口管理。",
      heroSubtitle:
        "原生或通过 Docker 安装与启停 PostgreSQL、Redis、MySQL、MariaDB 与 MongoDB。实时发现端口冲突、管理自启并一键打开 psql 或 redis-cli，不再局限于 macOS。",
      metrics: [
        { value: "三大平台", label: "支持 macOS、Windows 与 Linux（告别仅限 macOS 的 DBngin）" },
        {
          value: "5 种数据库引擎",
          label: "PostgreSQL、Redis、MySQL、MariaDB 与 MongoDB 原生或 Docker",
        },
        { value: "端口冲突防护", label: "实时显示占用进程详情，并可一键打开 CLI" },
      ],
      hearthColumnLabel: "Hearth（数据库与端口）",
      otherColumnLabel: "DBngin / Postgres.app",
      rows: [
        {
          criterion: "支持系统",
          hearth: "macOS、Windows 与 Linux 统一界面",
          other: "仅限 macOS（DBngin 与 Postgres.app）",
        },
        {
          criterion: "支持引擎",
          hearth: "PostgreSQL、Redis、MySQL、MariaDB 与 MongoDB",
          other: "PostgreSQL、MySQL 与 Redis（DBngin）或仅 Postgres",
        },
        {
          criterion: "原生与 Docker 服务",
          hearth: "统一管理 Homebrew/系统服务与 Docker 数据库容器",
          other: "仅管理自带独立二进制服务",
        },
        {
          criterion: "端口冲突检测",
          hearth: "启动前即标出哪个进程占用了 5432、6379 或 3306",
          other: "需手动挑选端口，缺少全局冲突视图",
        },
        {
          criterion: "协议与源码",
          hearth: "100% 免费且 MIT 开源",
          other: "免费闭源软件（DBngin）",
        },
        {
          criterion: "SQL 查询编辑器",
          hearth: "不内置 — 与 TablePlus、DataGrip 及 psql 完美配合",
          other: "不内置 — 与 TablePlus 或 psql 配合",
        },
      ],
      chooseHearth: {
        pill: "适合本地数据库管理",
        title: "选择 Hearth，如果你…",
        points: [
          "在 macOS、Windows 或 Linux 上工作，并希望使用开源统一工具",
          "同时运行 Homebrew/系统本地服务与 Docker Compose 数据库",
          "希望与容器、运行时一起查看端口冲突并快速启动 CLI",
        ],
      },
      keepOther: {
        pill: "何时仍适合 DBngin / SQL 客户端",
        title: "继续保留 DBngin 或 TablePlus，如果你…",
        points: [
          "需要在 macOS 上不借助 Docker 并行运行多个小版本隔离构建",
          "需要可视化 SQL 查询编辑器与数据表浏览（请继续使用 TablePlus 或 DataGrip）",
          "已有 DBngin 数据目录且暂不打算迁移",
        ],
      },
      steps: [
        {
          step: "步骤 01",
          title: "扫描运行中的服务与端口",
          desc: "Hearth 自动发现现有的 Homebrew、系统与 Docker 数据库，不重启服务也不触碰数据目录。",
        },
        {
          step: "步骤 02",
          title: "按计划安装或切换自启",
          desc: "通过明确安装计划添加 PostgreSQL、Redis、MySQL、MariaDB 或 MongoDB，并按需控制开机启动。",
        },
        {
          step: "步骤 03",
          title: "连接你的 SQL 客户端或 CLI",
          desc: "复制连接串或直接打开 psql/redis-cli/mongosh，查询与建表继续使用 TablePlus 或 DataGrip。",
        },
      ],
    },
  },
  guides: {
    "dev-environment-setup": {
      slug: "dev-environment-setup",
      path: "/guides/dev-environment-setup/",
      crumbLabel: "开发环境配置",
      metaTitle: "新手与 Vibe Coding 开发环境配置指南 · Hearth",
      metaDescription:
        "几分钟内在 macOS、Windows 或 Linux 上安装 Node.js、Python、Git、Docker、PostgreSQL 以及 Claude Code 或 Cursor，不再搞乱 PATH。",
      heroTitle: "几分钟配好干净的开发环境，不再把 PATH 搞乱。",
      heroSubtitle:
        "刚准备用 Claude Code 或 Cursor 写第一个项目，或是刚拿到新 Mac、Windows 或 Linux 电脑？Hearth 用一份透明计划帮你装好运行时、Docker、数据库与 AI 工具。",
      secondaryCtaLabel: "延伸阅读：AI 代理与 MCP 指南",
      secondaryCtaTo: "/guides/ai-agents-mcp/",
      presetsHeading: "选用现成预设，或按需自由组合",
      presets: [
        {
          pill: "新手与 Vibe Coding",
          pillTone: "accent",
          title: "Vibe Coding 起步包",
          desc: "10 分钟让全新电脑具备与 AI 代理协作写代码的完整环境。",
          stackLines: [
            "Git + GitHub CLI · mise",
            "Node.js 22 LTS · Python 3.13 (uv)",
            "Claude Code · Cursor · 共享 AGENTS.md",
          ],
          foot: "无需手改 ~/.zshrc",
        },
        {
          pill: "Web 与产品开发",
          pillTone: "info",
          title: "全栈 Web",
          desc: "克隆并运行现代 TypeScript 与 Python 仓库所需的全套工具。",
          stackLines: [
            "Homebrew/WinGet · mise",
            "Node 22 · Bun · pnpm · Python 3.13",
            "Docker + Colima · PostgreSQL · Redis",
          ],
          foot: "遵循 .nvmrc 与 mise.toml",
        },
        {
          pill: "后端与云原生",
          pillTone: "success",
          title: "后端与基础设施",
          desc: "面向 API 服务、容器与本地 Kubernetes 开发流。",
          stackLines: [
            "mise · Go 1.27 · Rust",
            "Docker + Colima · kubectl · kind",
            "PostgreSQL 16 · Redis · Terraform",
          ],
          foot: "可导出为 hearth-setup.json",
        },
      ],
      stepsHeading: "引导式配置的四个步骤",
      steps: [
        {
          step: "01 · 选择",
          title: "挑选技术栈",
          desc: "选择 Vibe Coding、Web 或后端预设，或导入团队分享的 JSON 预设。",
        },
        {
          step: "02 · 计划",
          title: "先审阅完整 diff",
          desc: "执行前查看每条命令与 Shell 标记块改动，自动包含 .bak 备份。",
        },
        {
          step: "03 · 安装",
          title: "带实时日志执行",
          desc: "按依赖顺序通过 Homebrew、WinGet、mise 与 Colima 安装，失败可安全回滚。",
        },
        {
          step: "04 · 验证",
          title: "在干净 Shell 中复核",
          desc: "Hearth 打开全新登录 Shell，确认每个工具都按正确 PATH 顺序解析。",
        },
      ],
      pitfallsHeading: "Hearth 帮你避开的常见踩坑",
      pitfalls: [
        {
          title: "PATH 混乱与 zsh 启动缓慢",
          desc: "混用 Homebrew Node、nvm 与 pyenv 常导致版本错乱和终端卡顿。Hearth 默认使用高速 mise 并追踪 which -a。",
        },
        {
          title: "MCP 配置中的明文 API Key",
          desc: "连接 AI 代理时密钥常被直接写进 JSON。Hearth 会主动预警并给出迁移至环境变量的安全计划。",
        },
        {
          title: "静默覆盖配置文件",
          desc: "来路不明的安装脚本常直接改写 ~/.zshrc 且无备份。Hearth 仅编辑自身标记块并保留时间戳 .bak 备份。",
        },
      ],
    },
    "ai-agents-mcp": {
      slug: "ai-agents-mcp",
      path: "/guides/ai-agents-mcp/",
      crumbLabel: "AI 代理与 MCP",
      metaTitle: "统一管理 Claude Code、Codex、Cursor、AGENTS.md 与 MCP · Hearth",
      metaDescription:
        "跨多个 AI 编程代理统一 AGENTS.md、共享技能、实时探测 MCP 服务器、识别明文密钥并统计本地 Token 消耗。",
      heroTitle: "统一管理 AGENTS.md、技能、MCP 服务器与本地 AI 消耗。",
      heroSubtitle:
        "不再在 Claude Code、Codex CLI、Cursor、GitHub Copilot 与 Gemini CLI 之间反复维护五个隐藏配置。Hearth 同步规则、实测 MCP 并私密统计本地 Token 支出。",
      secondaryCtaLabel: "延伸阅读：开发环境配置指南",
      secondaryCtaTo: "/guides/dev-environment-setup/",
      presetsHeading: "Hearth 简化的三大 AI 工作流",
      presets: [
        {
          pill: "共享指令",
          pillTone: "accent",
          title: "唯一 ~/.agents/AGENTS.md",
          desc: "只需编辑一次全局规则，即可安全软链接到每个已安装的 AI 代理。",
          stackLines: [
            "~/.agents/AGENTS.md（单一事实源）",
            "CLAUDE.md · CODEX · Gemini · Cursor",
            "软链接前展示逐行 diff",
          ],
          foot: "创建链接前自动保留 .bak 备份",
        },
        {
          pill: "MCP 控制面",
          pillTone: "info",
          title: "实时 MCP 探测与内置服务",
          desc: "验证每个 stdio/HTTP MCP 服务器，并将 Hearth 的只读诊断暴露给你的 AI 代理。",
          stackLines: [
            "hearth mcp（本地 stdio 服务）",
            "实时握手测试与工具数统计",
            "明文 API Key 安全提醒",
          ],
          foot: "任何修复都必须经你亲自确认",
        },
        {
          pill: "技能与用量",
          pillTone: "success",
          title: "共享技能与 Token 账本",
          desc: "从 GitHub 目录安装技能，并查看 7/30/90 天本地 Token 与估算费用。",
          stackLines: [
            "~/.agents/skills 共享技能库",
            "差异对比与一键同步",
            "本地 JSONL 会话日志解析与统计",
          ],
          foot: "100% 本地统计 · 绝不上传云端",
        },
      ],
      stepsHeading: "Hearth 如何梳理你的 AI 配置",
      steps: [
        {
          step: "01 · 发现",
          title: "定位所有代理与配置",
          desc: "只读扫描自动识别 Claude Code、Codex、Cursor、Copilot、Gemini 与 OpenCode 配置。",
        },
        {
          step: "02 · 统一",
          title: "同步 AGENTS.md 与技能",
          desc: "并排对比产生分歧的指令或技能文件，并链接至共享目录 ~/.agents/。",
        },
        {
          step: "03 · 审计",
          title: "探测 MCP 并排查密钥",
          desc: "对 MCP 执行健康检查、发现代理间的命令冲突，并将明文密钥迁移至环境变量。",
        },
        {
          step: "04 · 追踪",
          title: "掌握本地 Token 消耗",
          desc: "直接基于本地会话日志查看近 7、30 与 90 天的 Token 与 API 估算成本。",
        },
      ],
      pitfallsHeading: "Hearth 帮你避免的 AI 配置难题",
      pitfalls: [
        {
          title: "多个代理间规则逐渐跑偏",
          desc: "改了 CLAUDE.md 却忘了 Cursor 或 Codex，会导致代码风格分裂。一份软链接的 AGENTS.md 让它们始终一致。",
        },
        {
          title: "MCP 服务器静默失效",
          desc: "错误的 Node 路径或缺失命令会让 MCP 报出模糊错误。Hearth 实测每个服务并展示真实解析命令。",
        },
        {
          title: "配置文件中泄露明文密钥",
          desc: "将 Token 直接粘进 mcp.json 容易误提交。Hearth 导出报告自动脱敏并高亮明文密钥。",
        },
      ],
    },
  },
};
