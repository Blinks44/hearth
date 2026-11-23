import type { WorkflowTabId } from '@/components/WorkflowShowcase'
import { site } from '@/config/site'
import * as m from '@/integrations/i18n/paraglide/messages'
import type { ReleaseAssets } from '@/integrations/releases'

interface FooterProps {
  release: ReleaseAssets
  onSelectWorkflow?: (tab: WorkflowTabId) => void
}

export function Footer({ release, onSelectWorkflow }: FooterProps) {
  return (
    <footer className="site-footer">
      <div className="container">
        <div className="footer-grid">
          <div>
            <a href="#top" className="brand-link">
              <img
                src="/app-icon.png"
                alt="Hearth"
                width={26}
                height={26}
                className="brand-icon"
                loading="lazy"
              />
              <span>{site.name}</span>
            </a>
            <p className="footer-tagline">{m.footerTagline()}</p>
          </div>

          <div className="footer-col">
            <h4>{m.footerColProduct()}</h4>
            <ul>
              <li>
                <a href="#features" onClick={() => onSelectWorkflow?.('overview')}>
                  {m.footerLinkOverview()}
                </a>
              </li>
              <li>
                <a href="#features" onClick={() => onSelectWorkflow?.('projects')}>
                  {m.footerLinkProjects()}
                </a>
              </li>
              <li>
                <a href="#features" onClick={() => onSelectWorkflow?.('docker')}>
                  {m.footerLinkDocker()}
                </a>
              </li>
              <li>
                <a href="#features" onClick={() => onSelectWorkflow?.('ai')}>
                  {m.footerLinkAi()}
                </a>
              </li>
              <li>
                <a href="#features" onClick={() => onSelectWorkflow?.('cleanup')}>
                  {m.footerLinkCleanup()}
                </a>
              </li>
            </ul>
          </div>

          <div className="footer-col">
            <h4>{m.footerColResources()}</h4>
            <ul>
              <li><a href="#install">{m.footerLinkInstall()}</a></li>
              <li><a href={site.security} target="_blank" rel="noreferrer">{m.footerLinkSecurity()}</a></li>
              <li><a href={site.changelog} target="_blank" rel="noreferrer">{m.footerLinkChangelog()}</a></li>
              <li><a href={release.checksumsUrl} target="_blank" rel="noreferrer">{m.footerLinkChecksums()}</a></li>
              <li><a href={site.docs} target="_blank" rel="noreferrer">{m.footerLinkAcknowledgements()}</a></li>
            </ul>
          </div>

          <div className="footer-col">
            <h4>{m.footerColCommunity()}</h4>
            <ul>
              <li><a href={site.repository} target="_blank" rel="noreferrer">{m.footerLinkGithub()}</a></li>
              <li><a href={site.discussions} target="_blank" rel="noreferrer">{m.footerLinkDiscussions()}</a></li>
              <li><a href={site.issues} target="_blank" rel="noreferrer">{m.footerLinkIssues()}</a></li>
              <li><a href={site.contributing} target="_blank" rel="noreferrer">{m.footerLinkContributing()}</a></li>
              <li><a href={site.license} target="_blank" rel="noreferrer">{m.footerLinkLicense()}</a></li>
            </ul>
          </div>
        </div>

        <div className="footer-bottom">{m.footerCopyright()}</div>
      </div>
    </footer>
  )
}
