import { ArrowRight, Download } from 'lucide-react'
import { CopyButton } from '@/components/CopyButton'
import { site } from '@/config/site'
import * as m from '@/integrations/i18n/paraglide/messages'
import type { ReleaseAssets } from '@/integrations/releases'
import type { DetectedPlatform } from '@/lib/os'

interface InstallSectionProps {
  release: ReleaseAssets
  platform: DetectedPlatform
}

export function InstallSection({ release, platform }: InstallSectionProps) {
  const winCmd = `hearth-setup-${release.version}-amd64.exe`
  const linuxCmd = `sh install.sh hearth-${release.version}-linux-amd64.tar.gz`

  const cards = [
    {
      os: 'macos' as const,
      title: m.installMacTitle(),
      sub: m.installMacSub(),
      btnLabel: m.installMacBtn(),
      href: release.macosDmg,
      cmd: site.brewCommand,
      meta: m.installMacMeta(),
      alts: [] as Array<{ label: string; href: string }>,
    },
    {
      os: 'windows' as const,
      title: m.installWinTitle(),
      sub: m.installWinSub(),
      btnLabel: m.installWinBtn(),
      href: platform.arch === 'arm64' ? release.windowsArm64 : release.windowsAmd64,
      cmd: winCmd,
      meta: m.installWinMeta(),
      alts: [
        { label: m.installAltX64(), href: release.windowsAmd64 },
        { label: m.installAltArm64(), href: release.windowsArm64 },
      ],
    },
    {
      os: 'linux' as const,
      title: m.installLinuxTitle(),
      sub: m.installLinuxSub(),
      btnLabel: m.installLinuxBtn(),
      href: platform.arch === 'arm64' ? release.linuxDebArm64 : release.linuxDebAmd64,
      cmd: linuxCmd,
      meta: m.installLinuxMeta(),
      alts: [
        { label: `.deb ${m.installAltX64()}`, href: release.linuxDebAmd64 },
        { label: `.deb ${m.installAltArm64()}`, href: release.linuxDebArm64 },
        { label: `${m.installAltTar()} ${m.installAltX64()}`, href: release.linuxTarAmd64 },
      ],
    },
  ]

  return (
    <section id="install" className="install-section" data-reveal>
      <div className="container">
        <div className="section-header">
          <span className="eyebrow-label">{m.installEyebrow()}</span>
          <h2 className="section-title">{m.installTitle({ version: release.version })}</h2>
          <p className="section-subtitle">{m.installSubtitle()}</p>
        </div>

        <div className="install-grid">
          {cards.map((card) => {
            const detected = platform.os === card.os
            return (
              <article key={card.os} className="install-card" data-detected={detected}>
                <div className="install-card-head">
                  <h3>{card.title}</h3>
                  {detected ? (
                    <span className="pill pill-accent" style={{ textTransform: 'none', letterSpacing: 0 }}>
                      {m.installDetected()}
                    </span>
                  ) : null}
                </div>
                <p className="install-card-sub">{card.sub}</p>

                <a
                  href={card.href}
                  className={`${detected ? 'btn-hearth-primary' : 'btn-hearth-dark'} install-cta-btn`}
                >
                  <Download size={16} aria-hidden="true" />
                  <span>{card.btnLabel}</span>
                </a>

                <div className="install-cmd-row">
                  <code className="install-cmd-text">{card.cmd}</code>
                  <CopyButton text={card.cmd} />
                </div>

                {card.alts.length > 0 ? (
                  <div className="install-alt-links">
                    {card.alts.map((alt) => (
                      <a key={alt.label} href={alt.href}>
                        {alt.label}
                      </a>
                    ))}
                  </div>
                ) : null}

                <div className="install-card-meta">{card.meta}</div>
              </article>
            )
          })}
        </div>

        <div className="trust-bar">
          <div>
            <h4>{m.installTrustTitle()}</h4>
            <p>{m.installTrustBody()}</p>
          </div>
          <a
            href={release.checksumsUrl}
            target="_blank"
            rel="noreferrer"
            className="btn-hearth-outline"
            style={{ flexShrink: 0 }}
          >
            <span>{m.installTrustLink()}</span>
            <ArrowRight size={14} aria-hidden="true" />
          </a>
        </div>
      </div>
    </section>
  )
}
