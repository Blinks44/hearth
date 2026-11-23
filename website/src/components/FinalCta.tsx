import { ArrowUpRight, Download, Star } from "lucide-react";
import { site } from "@/config/site";
import * as m from "@/integrations/i18n/paraglide/messages";
import type { ReleaseAssets } from "@/integrations/releases";

interface FinalCtaProps {
  release: ReleaseAssets;
}

export function FinalCta({ release }: FinalCtaProps) {
  return (
    <section className="final-cta" data-reveal>
      <div className="container">
        <img
          src="/app-icon.png"
          alt="Hearth"
          width={56}
          height={56}
          className="final-cta-icon"
          loading="lazy"
        />
        <h2>{m.ctaTitle()}</h2>
        <div className="final-cta-actions">
          <a href="#install" className="btn-hearth-primary">
            <Download size={17} aria-hidden="true" />
            <span>{m.ctaDownload({ version: release.version })}</span>
          </a>
          <a
            href={site.repository}
            target="_blank"
            rel="noreferrer"
            className="btn-hearth-outline"
            style={{ height: "46px", paddingInline: "1.2rem" }}
          >
            <Star size={15} aria-hidden="true" />
            <span>{m.ctaStar()}</span>
          </a>
        </div>

        <div className="suite-band">
          <p className="suite-eyebrow">{m.suiteEyebrow()}</p>
          <div className="suite-grid">
            <a href={site.suite.monny} target="_blank" rel="noreferrer" className="suite-card">
              <img src="/suite/monny.png" alt="Monny" width={48} height={48} loading="lazy" />
              <div className="suite-card-body">
                <strong>Monny</strong>
                <p>{m.suiteMonnyDesc()}</p>
              </div>
              <span className="suite-card-arrow">
                {m.suiteExplore()}
                <ArrowUpRight size={14} aria-hidden="true" />
              </span>
            </a>

            <a href={site.suite.rebind} target="_blank" rel="noreferrer" className="suite-card">
              <img src="/suite/rebind.png" alt="Rebind" width={48} height={48} loading="lazy" />
              <div className="suite-card-body">
                <strong>Rebind</strong>
                <p>{m.suiteRebindDesc()}</p>
              </div>
              <span className="suite-card-arrow">
                {m.suiteExplore()}
                <ArrowUpRight size={14} aria-hidden="true" />
              </span>
            </a>
          </div>
        </div>
      </div>
    </section>
  );
}
