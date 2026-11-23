import {
  AlertTriangle,
  ArrowUpCircle,
  BarChart3,
  Cpu,
  Database,
  Download,
  FolderGit2,
  LayoutGrid,
  Moon,
  Network,
  Puzzle,
  RefreshCw,
  Search,
  Settings,
  Sun,
  Terminal,
  Trash2,
} from 'lucide-react'
import type { ReactNode } from 'react'
import { useCallback, useMemo, useRef, useState } from 'react'
import { BrandLogo } from '@/components/BrandLogo'
import type { ColorScheme } from '@/lib/theme'

interface HeroAppWindowProps {
  theme: ColorScheme
  onToggleTheme: () => void
}

type PanelId =
  | 'overview'
  | 'projects-list'
  | 'issues'
  | 'updates'
  | 'cleanup'
  | 'setup'
  | 'bun'
  | 'go'
  | 'node'
  | 'python'
  | 'rust'
  | 'docker'
  | 'kubernetes'
  | 'databases'
  | 'ports'
  | 'ai-usage'
  | 'ai-skills'
  | 'ai-mcp'
  | 'path'
  | 'settings'

interface SidebarItem {
  id: PanelId
  label: string
  /** Optional full-window screenshot basename in /screenshots/ if one exists; falls back to panels/<id>. */
  fullShot?: string
  keywords: string
  badge?: string
  badgeTone?: 'warn' | 'muted'
  dot?: 'ok' | 'warn'
  icon: (theme: ColorScheme) => ReactNode
}

interface SidebarSection {
  title?: string
  items: SidebarItem[]
}

const SIDEBAR_SECTIONS: SidebarSection[] = [
  {
    items: [
      {
        id: 'overview',
        label: 'Overview',
        fullShot: 'overview',
        keywords: 'overview home dashboard summary status',
        icon: () => <LayoutGrid size={14} strokeWidth={1.9} />,
      },
      {
        id: 'projects-list',
        label: 'Projects',
        fullShot: 'projects',
        keywords: 'projects repos folders workspaces billing-api acme-web',
        badge: '4',
        badgeTone: 'muted',
        icon: () => <FolderGit2 size={14} strokeWidth={1.9} />,
      },
    ],
  },
  {
    title: 'Maintenance',
    items: [
      {
        id: 'issues',
        label: 'Issues',
        keywords: 'issues problems errors warnings port conflict fix',
        badge: '3',
        badgeTone: 'warn',
        icon: () => <AlertTriangle size={14} strokeWidth={1.9} />,
      },
      {
        id: 'updates',
        label: 'Updates',
        keywords: 'updates upgrade outdated versions packages',
        badge: '3',
        badgeTone: 'muted',
        icon: () => <ArrowUpCircle size={14} strokeWidth={1.9} />,
      },
      {
        id: 'cleanup',
        label: 'Cleanup',
        fullShot: 'cleanup',
        keywords: 'cleanup disk space cache node_modules docker prune',
        badge: '14.5 GB',
        badgeTone: 'muted',
        icon: () => <Trash2 size={14} strokeWidth={1.9} />,
      },
      {
        id: 'setup',
        label: 'Install',
        fullShot: 'setup',
        keywords: 'install setup new machine onboarding bootstrap',
        icon: () => <Download size={14} strokeWidth={1.9} />,
      },
    ],
  },
  {
    title: 'Languages & runtimes',
    items: [
      {
        id: 'bun',
        label: 'Bun',
        keywords: 'bun javascript typescript runtime',
        badge: '1.2',
        badgeTone: 'muted',
        icon: (theme) => <BrandLogo name="bun" alt="" theme={theme} size={14} />,
      },
      {
        id: 'go',
        label: 'Go',
        keywords: 'go golang mise runtime',
        badge: '1.24',
        badgeTone: 'muted',
        icon: (theme) => <BrandLogo name="go" alt="" theme={theme} size={14} />,
      },
      {
        id: 'node',
        label: 'Node.js',
        keywords: 'node nodejs javascript fnm npm pnpm corepack',
        badge: '22.14',
        badgeTone: 'muted',
        icon: (theme) => <BrandLogo name="nodedotjs" alt="" theme={theme} size={14} />,
      },
      {
        id: 'python',
        label: 'Python',
        keywords: 'python uv pip virtualenv',
        badge: '3.13',
        badgeTone: 'muted',
        icon: (theme) => <BrandLogo name="python" alt="" theme={theme} size={14} />,
      },
      {
        id: 'rust',
        label: 'Rust',
        keywords: 'rust rustup cargo toolchain',
        badge: '1.85',
        badgeTone: 'muted',
        icon: (theme) => <BrandLogo name="rust" alt="" theme={theme} hasDark size={14} />,
      },
    ],
  },
  {
    title: 'Infrastructure',
    items: [
      {
        id: 'docker',
        label: 'Docker',
        fullShot: 'docker',
        keywords: 'docker colima containers compose images volumes',
        dot: 'ok',
        icon: (theme) => <BrandLogo name="docker" alt="" theme={theme} size={14} />,
      },
      {
        id: 'kubernetes',
        label: 'Kubernetes',
        fullShot: 'kubernetes',
        keywords: 'kubernetes k8s kind-dev pods helm clusters',
        badge: 'kind-dev',
        badgeTone: 'muted',
        icon: (theme) => <BrandLogo name="kubernetes" alt="" theme={theme} size={14} />,
      },
      {
        id: 'databases',
        label: 'Databases',
        keywords: 'databases postgresql postgres redis services',
        dot: 'warn',
        icon: () => <Database size={14} strokeWidth={1.9} />,
      },
      {
        id: 'ports',
        label: 'Ports',
        keywords: 'ports tcp listeners 5432 6379 lsof',
        icon: () => <Network size={14} strokeWidth={1.9} />,
      },
    ],
  },
  {
    title: 'AI',
    items: [
      {
        id: 'ai-usage',
        label: 'Usage',
        fullShot: 'ai-usage',
        keywords: 'ai usage spend tokens claude codex cost',
        badge: '$7.29',
        badgeTone: 'muted',
        icon: () => <BarChart3 size={14} strokeWidth={1.9} />,
      },
      {
        id: 'ai-skills',
        label: 'Skills',
        keywords: 'ai skills agents claude codex cursor symlinks',
        badge: '3',
        badgeTone: 'muted',
        icon: () => <Puzzle size={14} strokeWidth={1.9} />,
      },
      {
        id: 'ai-mcp',
        label: 'MCP',
        fullShot: 'ai-mcp',
        keywords: 'mcp servers tools model context protocol',
        badge: '4',
        badgeTone: 'muted',
        icon: () => <Cpu size={14} strokeWidth={1.9} />,
      },
    ],
  },
  {
    title: 'System',
    items: [
      {
        id: 'path',
        label: 'PATH & variables',
        keywords: 'path shell zshrc env variables duplicates',
        badge: '1',
        badgeTone: 'warn',
        icon: () => <Terminal size={14} strokeWidth={1.9} />,
      },
    ],
  },
]

const SETTINGS_ITEM: SidebarItem = {
  id: 'settings',
  label: 'Settings',
  keywords: 'settings preferences theme language startup',
  icon: () => <Settings size={14} strokeWidth={1.9} />,
}

const ALL_ITEMS: SidebarItem[] = [
  ...SIDEBAR_SECTIONS.flatMap((section) => section.items),
  SETTINGS_ITEM,
]

function panelSrc(id: PanelId, theme: ColorScheme): string {
  return `/screenshots/panels/${id}-${theme}.webp`
}

export function HeroAppWindow({ theme, onToggleTheme }: HeroAppWindowProps) {
  const [active, setActive] = useState<PanelId>('overview')
  const [displayed, setDisplayed] = useState<PanelId>('overview')
  const [query, setQuery] = useState('')
  const [refreshing, setRefreshing] = useState(false)
  const decodedCache = useRef<Map<string, HTMLImageElement>>(new Map())
  const pendingLoads = useRef<Map<string, Promise<void>>>(new Map())
  const wantedId = useRef<PanelId>('overview')

  const ensureDecoded = useCallback((src: string): Promise<void> => {
    if (decodedCache.current.has(src)) return Promise.resolve()
    const inflight = pendingLoads.current.get(src)
    if (inflight) return inflight

    const img = new Image()
    img.decoding = 'sync'
    img.src = src

    const promise = (typeof img.decode === 'function'
      ? img.decode()
      : new Promise<void>((resolve, reject) => {
          img.onload = () => resolve()
          img.onerror = () => reject(new Error(`failed to load ${src}`))
        })
    )
      .then(() => {
        decodedCache.current.set(src, img)
      })
      .catch(() => {
        // Fallback to letting the visible <img> element load it directly.
      })
      .finally(() => {
        pendingLoads.current.delete(src)
      })

    pendingLoads.current.set(src, promise)
    return promise
  }, [])

  const preloadPanel = useCallback(
    (id: PanelId) => {
      void ensureDecoded(panelSrc(id, theme))
    },
    [ensureDecoded, theme],
  )

  const selectPanel = useCallback(
    (id: PanelId) => {
      wantedId.current = id
      setActive(id)
      const src = panelSrc(id, theme)
      if (decodedCache.current.has(src)) {
        setDisplayed(id)
        return
      }
      void ensureDecoded(src).then(() => {
        if (wantedId.current === id) {
          setDisplayed(id)
        }
      })
    },
    [ensureDecoded, theme],
  )

  const filteredSections = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return SIDEBAR_SECTIONS
    return SIDEBAR_SECTIONS.map((section) => ({
      ...section,
      items: section.items.filter(
        (item) =>
          item.label.toLowerCase().includes(q) || item.keywords.toLowerCase().includes(q),
      ),
    })).filter((section) => section.items.length > 0)
  }, [query])

  const activeItem: SidebarItem = ALL_ITEMS.find((item) => item.id === active) ?? SETTINGS_ITEM

  const triggerRefresh = () => {
    setRefreshing(true)
    window.setTimeout(() => setRefreshing(false), 620)
  }

  return (
    <div className="hero-app-shell" data-theme={theme}>
      <aside className="ha-sidebar" aria-label="Hearth sections">
        <div className="ha-traffic-lights" aria-hidden="true">
          <span className="ha-tl ha-tl-red" />
          <span className="ha-tl ha-tl-yellow" />
          <span className="ha-tl ha-tl-green" />
        </div>

        <div className="ha-brand">
          <img
            src="/app-icon.png"
            alt=""
            width={24}
            height={24}
            className="ha-brand-icon"
          />
          <div className="ha-brand-text">
            <strong>Hearth</strong>
            <span>alex-macbook</span>
          </div>
        </div>

        <label className="ha-search-box">
          <Search size={12} className="ha-search-ico" aria-hidden="true" />
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && filteredSections[0]?.items[0]) {
                selectPanel(filteredSections[0].items[0].id)
              }
            }}
            placeholder="Find a tool..."
            aria-label="Find a tool"
          />
          <kbd>⌘K</kbd>
        </label>

        <nav className="ha-nav-scroll" role="tablist" aria-orientation="vertical">
          {filteredSections.map((section, idx) => (
            <div key={section.title ?? `top-${idx}`} className="ha-nav-group">
              {section.title ? <div className="ha-section-label">{section.title}</div> : null}
              {section.items.map((item) => {
                const isSelected = active === item.id
                return (
                  <button
                    key={item.id}
                    type="button"
                    role="tab"
                    aria-selected={isSelected}
                    className={`ha-nav-item ${isSelected ? 'is-active' : ''}`}
                    onMouseEnter={() => preloadPanel(item.id)}
                    onFocus={() => preloadPanel(item.id)}
                    onClick={() => selectPanel(item.id)}
                  >
                    <span className="ha-nav-ico" aria-hidden="true">
                      {item.icon(theme)}
                    </span>
                    <span className="ha-nav-label">{item.label}</span>
                    {item.badge ? (
                      <span
                        className={`ha-nav-end ${item.badgeTone === 'warn' ? 'is-warn' : ''}`}
                      >
                        {item.badge}
                      </span>
                    ) : null}
                    {item.dot ? (
                      <span
                        className={`ha-nav-dot ${item.dot === 'ok' ? 'is-ok' : 'is-warn'}`}
                        aria-hidden="true"
                      />
                    ) : null}
                  </button>
                )
              })}
            </div>
          ))}
          {filteredSections.length === 0 ? (
            <div className="ha-nav-empty">No matching sections</div>
          ) : null}
        </nav>

        <div className="ha-sidebar-footer">
          <button
            type="button"
            role="tab"
            aria-selected={active === 'settings'}
            className={`ha-nav-item ${active === 'settings' ? 'is-active' : ''}`}
            onMouseEnter={() => preloadPanel('settings')}
            onFocus={() => preloadPanel('settings')}
            onClick={() => selectPanel('settings')}
          >
            <span className="ha-nav-ico" aria-hidden="true">
              {SETTINGS_ITEM.icon(theme)}
            </span>
            <span className="ha-nav-label">Settings</span>
          </button>

          <div className="ha-status-row">
            <span className="ha-status-left">
              <span className={`ha-status-dot ${refreshing ? 'is-busy' : ''}`} />
              <span>{refreshing ? 'Checking...' : 'Checked 2 min ago'}</span>
            </span>
            <div className="ha-status-actions">
              <button
                type="button"
                className="ha-icon-btn"
                onClick={onToggleTheme}
                aria-label="Toggle theme"
                title="Toggle theme"
              >
                {theme === 'dark' ? <Sun size={12} /> : <Moon size={12} />}
              </button>
              <button
                type="button"
                className={`ha-icon-btn ${refreshing ? 'is-spin' : ''}`}
                onClick={triggerRefresh}
                aria-label="Refresh"
                title="Refresh"
              >
                <RefreshCw size={12} />
              </button>
            </div>
          </div>
        </div>
      </aside>

      <div className="ha-stage" role="tabpanel" aria-label={activeItem.label}>
        <img
          key={`${displayed}-${theme}`}
          src={panelSrc(displayed, theme)}
          alt={`Hearth ${activeItem.label} screen`}
          className="ha-stage-img"
          width={2070}
          height={1640}
          decoding="sync"
        />
      </div>
    </div>
  )
}
