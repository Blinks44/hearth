import { Link } from "@tanstack/react-router";
import { ArrowRight, Download } from "lucide-react";
import { Footer } from "@/components/Footer";
import { Navbar } from "@/components/Navbar";
import type { SiteRoutePath } from "@/config/locales";
import type { GuidePageContent, GuideSlug } from "@/content/seo";
import * as m from "@/integrations/i18n/paraglide/messages";
import type { ReleaseAssets } from "@/integrations/releases";
import { useRevealObserver } from "@/lib/reveal";
import { useTheme } from "@/lib/theme";

interface GuidePageProps {
  content: GuidePageContent;
  release: ReleaseAssets;
}

interface RelatedPageCard {
  path: Exclude<SiteRoutePath, "/">;
  title: string;
  desc: string;
}

const pillToneClasses = {
  accent: "pill pill-accent",
  info: "pill pill-info",
  success: "pill pill-success",
} as const;

export function GuidePage({ content, release }: GuidePageProps) {
  const { theme, toggleTheme } = useTheme();
  useRevealObserver();

  const guideTabs: Array<{
    slug: GuideSlug;
    to: "/guides/dev-environment-setup/" | "/guides/ai-agents-mcp/";
    label: string;
  }> = [
    {
      slug: "dev-environment-setup",
      to: "/guides/dev-environment-setup/",
      label: m.seoGuideTabSetup(),
    },
    { slug: "ai-agents-mcp", to: "/guides/ai-agents-mcp/", label: m.seoGuideTabAi() },
  ];

  const allRelated: RelatedPageCard[] = [
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
                <Link to="/" hash="faq">
                  {m.seoCrumbGuides()}
                </Link>
                <span aria-hidden="true">/</span>
                <span className="seo-breadcrumb-current">{content.crumbLabel}</span>
              </nav>

              <div className="seo-tabs" role="navigation" aria-label={m.seoCrumbGuides()}>
                {guideTabs.map((tab) => (
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

              <Link to={content.secondaryCtaTo} className="btn-hearth-outline seo-secondary-cta">
                <span>{content.secondaryCtaLabel}</span>
                <ArrowRight size={14} aria-hidden="true" />
              </Link>
            </div>
          </section>

          <section className="seo-section" data-reveal>
            <h2 className="seo-section-title">{content.presetsHeading}</h2>
            <div className="seo-presets-grid">
              {content.presets.map((preset) => (
                <article key={preset.title} className="seo-preset-card">
                  <div className="seo-preset-top">
                    <span className={pillToneClasses[preset.pillTone]}>{preset.pill}</span>
                    <h3>{preset.title}</h3>
                    <p>{preset.desc}</p>
                  </div>

                  <div className="seo-preset-stack mono">
                    {preset.stackLines.map((line) => (
                      <div key={line}>{line}</div>
                    ))}
                  </div>

                  <div className="seo-preset-foot mono">{preset.foot}</div>
                </article>
              ))}
            </div>
          </section>

          <section className="seo-section" data-reveal>
            <h2 className="seo-section-title">{content.stepsHeading}</h2>
            <div className="seo-steps-grid seo-steps-4">
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
            <h2 className="seo-section-title">{content.pitfallsHeading}</h2>
            <div className="seo-pitfalls-grid">
              {content.pitfalls.map((pitfall) => (
                <article key={pitfall.title} className="seo-pitfall-card">
                  <h3>{pitfall.title}</h3>
                  <p>{pitfall.desc}</p>
                </article>
              ))}
            </div>
          </section>

          <section className="seo-section" data-reveal>
            <h2 className="seo-related-title">{m.seoGuideRelatedHeading()}</h2>
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
                <h3>{m.seoGuideCtaTitle()}</h3>
                <p>{m.seoGuideCtaSub()}</p>
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
