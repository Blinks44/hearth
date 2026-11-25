import { Link } from "@tanstack/react-router";
import { ArrowRight, Check } from "lucide-react";
import * as m from "@/integrations/i18n/paraglide/messages";

interface AudienceCard {
  pill: string;
  pillClass: string;
  title: string;
  desc: string;
  bullets: [string, string, string];
  ctaLabel: string;
  to: "/" | "/guides/dev-environment-setup/" | "/guides/ai-agents-mcp/";
  hash?: string;
}

export function WhoItIsForSection() {
  const cards: AudienceCard[] = [
    {
      pill: m.whoCard1Pill(),
      pillClass: "pill pill-accent",
      title: m.whoCard1Title(),
      desc: m.whoCard1Desc(),
      bullets: [m.whoCard1Bullet1(), m.whoCard1Bullet2(), m.whoCard1Bullet3()],
      ctaLabel: m.whoCard1Cta(),
      to: "/guides/dev-environment-setup/",
    },
    {
      pill: m.whoCard2Pill(),
      pillClass: "pill pill-info",
      title: m.whoCard2Title(),
      desc: m.whoCard2Desc(),
      bullets: [m.whoCard2Bullet1(), m.whoCard2Bullet2(), m.whoCard2Bullet3()],
      ctaLabel: m.whoCard2Cta(),
      to: "/guides/ai-agents-mcp/",
    },
    {
      pill: m.whoCard3Pill(),
      pillClass: "pill pill-success",
      title: m.whoCard3Title(),
      desc: m.whoCard3Desc(),
      bullets: [m.whoCard3Bullet1(), m.whoCard3Bullet2(), m.whoCard3Bullet3()],
      ctaLabel: m.whoCard3Cta(),
      to: "/",
      hash: "alternatives",
    },
  ];

  return (
    <section id="who-its-for" className="who-section" data-reveal>
      <div className="container">
        <div className="section-header">
          <span className="eyebrow-label">{m.whoEyebrow()}</span>
          <h2 className="section-title">{m.whoTitle()}</h2>
          <p className="section-subtitle">{m.whoSubtitle()}</p>
        </div>

        <div className="who-grid">
          {cards.map((card) => (
            <article key={card.pill} className="who-card">
              <div className="who-card-body">
                <span className={card.pillClass}>{card.pill}</span>
                <h3>{card.title}</h3>
                <p>{card.desc}</p>
                <ul className="who-bullets">
                  {card.bullets.map((bullet) => (
                    <li key={bullet}>
                      <Check size={14} strokeWidth={2.5} aria-hidden="true" className="who-check" />
                      <span>{bullet}</span>
                    </li>
                  ))}
                </ul>
              </div>

              <Link to={card.to} hash={card.hash} className="seo-inline-link">
                <span>{card.ctaLabel}</span>
                <ArrowRight size={14} aria-hidden="true" />
              </Link>
            </article>
          ))}
        </div>
      </div>
    </section>
  );
}
