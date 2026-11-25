import { Link } from "@tanstack/react-router";
import { ArrowRight, Check, Download } from "lucide-react";
import { CopyButton } from "@/components/CopyButton";
import { Footer } from "@/components/Footer";
import { Navbar } from "@/components/Navbar";
import type { SiteRoutePath } from "@/config/locales";
import { site } from "@/config/site";
import type { AlternativePageContent, AlternativeSlug } from "@/content/seo";
import * as m from "@/integrations/i18n/paraglide/messages";
import type { ReleaseAssets } from "@/integrations/releases";
import { useRevealObserver } from "@/lib/reveal";
import { useTheme } from "@/lib/theme";

interface AlternativePageProps {
  content: AlternativePageContent;
  release: ReleaseAssets;
}

interface RelatedPageCard {
  path: Exclude<SiteRoutePath, "/">;
  title: string;
  desc: string;
}

export function AlternativePage({ content, release }: AlternativePageProps) {
  const { theme, toggleTheme } = useTheme();
  useRevealObserver();

  const altTabs: Array<{
    slug: AlternativeSlug;
    to: "/alternatives/docker-desktop/" | "/alternatives/lens-freelens/" | "/alternatives/dbngin/";
    label: string;
  }> = [
    { slug: "docker-desktop", to: "/alternatives/docker-desktop/", label: m.seoAltTabDocker() },
    { slug: "lens-freelens", to: "/alternatives/lens-freelens/", label: m.seoAltTabLens() },
    { slug: "dbngin", to: "/alternatives/dbngin/", label: m.seoAltTabDbngin() },
  ];

  const allRelated: RelatedPageCard[] = [
    {
      path: "/alternatives/docker-desktop/",
      title: m.seoRelatedDockerTitle(),
      desc: m.seoRelatedDockerDesc(),
    },
    {
      path: "/alternatives/lens-freelens/",
      title: m.seoRelatedLensTitle(),
      desc: m.seoRelatedLensDesc(),
    },
    {
      path: "/alternatives/dbngin/",
      title: m.seoRelatedDbnginTitle(),
      desc: m.seoRelatedDbnginDesc(),
    },
    {
      path: "/guides/ai-agents-mcp/",
      title: m.seoRelatedAiTitle(),
      desc: m.seoRelatedAiDesc(),
    },
    {
      path: "/guides/dev-environment-setup/",
      title: m.seoRelatedSetupTitle(),
      desc: m.seoRelatedSetupDesc(),
    },
  ];

  const related = allRelated.filter((item) => item.path !== content.path);

  return (
    <>
      <Navbar theme={theme} onToggleTheme={toggleTheme} />
      <main id="top" className="seo-page-main">
        <div className="container seo-page-stack">
          <section className="seo-hero">
            <div className="seo-topbar">
              <nav className="seo-breadcrumb" aria-label="Breadcrumb">
                <Link to="/">{m.seoCrumbHome()}</Link>
                <span aria-hidden="true">/</span>
                <Link to="/" hash="alternatives">
                  {m.seoCrumbAlternatives()}
                </Link>
                <span aria-hidden="true">/</span>
                <span className="seo-breadcrumb-current">{content.crumbLabel}</span>
              </nav>

              <div className="seo-tabs" role="navigation" aria-label={m.seoCrumbAlternatives()}>
                {altTabs.map((tab) => (
                  <Link
                    key={tab.slug}
                    to={tab.to}
                    className="seo-tab"
                    data-active={tab.slug === content.slug ? "true" : undefined}
                  >
                    {tab.label}
                  </Link>
                ))}
              </div>
            </div>

            <h1 className="seo-h1">{content.heroTitle}</h1>
            <p className="seo-hero-sub">{content.heroSubtitle}</p>

            <div className="seo-hero-actions">
              <Link to="/" hash="install" className="btn-hearth-primary">
                <Download size={16} aria-hidden="true" />
                <span>{m.seoDownloadFree()}</span>
              </Link>

              <div className="seo-cmd-pill">
                <code className="mono">{site.brewCommand}</code>
                <CopyButton text={site.brewCommand} />
              </div>
            </div>

            <div className="seo-metrics-grid">
              {content.metrics.map((metric) => (
                <div key={metric.value} className="seo-metric-card">
                  <div className="seo-metric-value">{metric.value}</div>
                  <p className="seo-metric-label">{metric.label}</p>
                </div>
              ))}
            </div>
          </section>

          <section className="seo-section" data-reveal>
            <h2 className="seo-section-title">{m.seoComparisonHeading()}</h2>
            <div className="seo-compare-card">
              <div className="seo-compare-head" aria-hidden="true">
                <span>{m.seoColCriterion()}</span>
                <span className="seo-compare-col-hearth">{content.hearthColumnLabel}</span>
                <span>{content.otherColumnLabel}</span>
              </div>

              <div className="seo-compare-body">
                {content.rows.map((row) => (
                  <div key={row.criterion} className="seo-compare-row">
                    <div className="seo-compare-criterion">{row.criterion}</div>
                    <div className="seo-compare-hearth">
                      <Check
                        size={15}
                        strokeWidth={2.5}
                        aria-hidden="true"
                        className="seo-compare-check"
                      />
                      <span>{row.hearth}</span>
                    </div>
                    <div className="seo-compare-other">{row.other}</div>
                  </div>
                ))}
              </div>
            </div>
          </section>

          <section className="seo-section" data-reveal>
            <div className="seo-tradeoffs-grid">
              <article className="seo-tradeoff-card">
                <span className="pill pill-success">{content.chooseHearth.pill}</span>
                <h3>{content.chooseHearth.title}</h3>
                <ul className="seo-tradeoff-list">
                  {content.chooseHearth.points.map((point) => (
                    <li key={point}>
                      <Check
                        size={14}
                        strokeWidth={2.5}
                        aria-hidden="true"
                        className="seo-tradeoff-check"
                      />
                      <span>{point}</span>
                    </li>
                  ))}
                </ul>
              </article>

              <article className="seo-tradeoff-card seo-tradeoff-neutral">
                <span className="pill pill-neutral">{content.keepOther.pill}</span>
                <h3>{content.keepOther.title}</h3>
                <ul className="seo-tradeoff-list">
                  {content.keepOther.points.map((point) => (
                    <li key={point}>
                      <span className="seo-tradeoff-dot" aria-hidden="true">
                        •
                      </span>
                      <span>{point}</span>
                    </li>
                  ))}
                </ul>
              </article>
            </div>
          </section>

          <section className="seo-section" data-reveal>
            <h2 className="seo-section-title">{m.seoSwitchHeading()}</h2>
            <div className="seo-steps-grid seo-steps-3">
              {content.steps.map((step) => (
                <article key={step.step} className="seo-step-card">
                  <div className="seo-step-kicker mono">{step.step}</div>
                  <h3>{step.title}</h3>
                  <p>{step.desc}</p>
                </article>
              ))}
            </div>
          </section>

          <section className="seo-section" data-reveal>
            <h2 className="seo-related-title">{m.seoRelatedHeading()}</h2>
            <div className="seo-related-grid">
              {related.map((card) => (
                <Link key={card.path} to={card.path} className="seo-related-card">
                  <div className="seo-related-card-title">
                    <span>{card.title}</span>
                    <ArrowRight size={14} aria-hidden="true" />
                  </div>
                  <p>{card.desc}</p>
                </Link>
              ))}
            </div>

            <div className="seo-cta-band">
              <div>
                <h3>{m.seoAltCtaTitle()}</h3>
                <p>{m.seoAltCtaSub()}</p>
              </div>
              <Link to="/" hash="install" className="btn-hearth-primary">
                <span>{m.seoDownloadFree()}</span>
              </Link>
            </div>
          </section>
        </div>
      </main>
      <Footer release={release} />
    </>
  );
}
