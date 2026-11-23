import { Download } from 'lucide-react'
import { BrandLogo } from '@/components/BrandLogo'
import { CopyButton } from '@/components/CopyButton'
import { HeroAppWindow } from '@/components/HeroAppWindow'
import { site } from '@/config/site'
import * as m from '@/integrations/i18n/paraglide/messages'
import type { ReleaseAssets } from '@/integrations/releases'
import type { DetectedPlatform } from '@/lib/os'
import type { ColorScheme } from '@/lib/theme'

interface HeroProps {
  release: ReleaseAssets
  platform: DetectedPlatform
  theme: ColorScheme
  onToggleTheme: () => void
}

export function Hero({ release, platform, theme, onToggleTheme }: HeroProps) {
  const primaryCta = (() => {
    if (platform.os === 'windows') {
      return {
        label: m.heroDownloadWin(),
        href: platform.arch === 'arm64' ? release.windowsArm64 : release.windowsAmd64,
      }
    }
    if (platform.os === 'linux') {
      return {
        label: m.heroDownloadLinux(),
        href: platform.arch === 'arm64' ? release.linuxDebArm64 : release.linuxDebAmd64,
      }
    }
    return {
      label: m.heroDownloadMac(),
      href: release.macosDmg,
    }
  })()

  return (
    <section id="top" className="hero">
      <div className="container">
        <div className="hero-eyebrow">
          <span className="hero-eyebrow-dot" aria-hidden="true" />
          <span>{m.heroEyebrow({ version: release.version })}</span>
        </div>

        <h1 className="hero-title">
          {m.heroTitleLine1()}
          <br />
          {m.heroTitleLine2()}
        </h1>

        <p className="hero-subtitle">{m.heroSubtitle()}</p>

        <div className="hero-actions">
          <a href={primaryCta.href} className="btn-hearth-primary">
            <Download size={17} aria-hidden="true" />
            <span>{primaryCta.label}</span>
          </a>

          <div className="hero-brew-box">
            <span className="hero-brew-prompt mono">$</span>
            <code className="hero-brew-cmd">{site.brewCommand}</code>
            <CopyButton text={site.brewCommand} label={m.heroCopyCommand()} />
          </div>
        </div>

        <p className="hero-meta hero-meta-desktop">{m.heroMeta()}</p>
        <p className="hero-meta hero-meta-mobile">{m.heroMetaMobile()}</p>

        {/* Desktop Interactive Hearth App Window */}
        <div className="hero-window-wrap">
          <HeroAppWindow theme={theme} onToggleTheme={onToggleTheme} />
        </div>

        {/* Mobile compact 2x2 live summary matching Sketch W03 */}
        <div className="hero-mobile-summary">
          <div className="mobile-summary-banner">
            <span className="status-chip status-warning">
              <span className="status-chip-dot" />
              {m.tabOverviewIssuesBadge()}
            </span>
            <h3>{m.tabOverviewStatus()}</h3>
            <p>{m.tabOverviewSub()}</p>
          </div>
          <div className="mobile-summary-grid">
            <div className="mobile-summary-tile">
              <div className="mobile-summary-tile-head">
                <BrandLogo name="nodedotjs" alt="Node.js" theme={theme} size={16} />
                <span>Node.js</span>
              </div>
              <div className="mobile-summary-tile-val mono">v22.14.0</div>
              <div className="mobile-summary-tile-sub">fnm · 2 projects</div>
            </div>
            <div className="mobile-summary-tile">
              <div className="mobile-summary-tile-head">
                <BrandLogo name="docker" alt="Docker" theme={theme} size={16} />
                <span>Docker</span>
              </div>
              <div className="mobile-summary-tile-val mono">v27.5.1</div>
              <div className="mobile-summary-tile-sub">Colima · 4 running</div>
            </div>
            <div className="mobile-summary-tile">
              <div className="mobile-summary-tile-head">
                <BrandLogo name="kubernetes" alt="Kubernetes" theme={theme} size={16} />
                <span>Kubernetes</span>
              </div>
              <div className="mobile-summary-tile-val mono">kind-dev</div>
              <div className="mobile-summary-tile-sub">default · 2 ctx</div>
            </div>
            <div className="mobile-summary-tile">
              <div className="mobile-summary-tile-head">
                <BrandLogo name="claude" alt="AI & MCP" theme={theme} size={16} />
                <span>AI &amp; MCP</span>
              </div>
              <div className="mobile-summary-tile-val mono">≈ $7.29</div>
              <div className="mobile-summary-tile-sub">3 agents · 4 MCP</div>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
