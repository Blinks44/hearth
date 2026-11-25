import {
  Accordion,
  AccordionItem,
  AccordionItemBody,
  AccordionItemContent,
  AccordionItemTrigger,
} from "@moduix/react/accordion";
import { Link } from "@tanstack/react-router";
import { ArrowRight, Minus, Plus } from "lucide-react";
import { useState } from "react";
import type { Locale, SiteRoutePath } from "@/config/locales";
import * as m from "@/integrations/i18n/paraglide/messages";
import type { FaqSchemaItem } from "@/integrations/seo";

export function getLandingFaqItems(locale?: Locale): FaqSchemaItem[] {
  const opts = locale ? { locale } : undefined;
  return [
    { question: m.faqQ1({}, opts), answer: m.faqA1({}, opts) },
    { question: m.faqQ2({}, opts), answer: m.faqA2({}, opts) },
    { question: m.faqQ3({}, opts), answer: m.faqA3({}, opts) },
    { question: m.faqQ4({}, opts), answer: m.faqA4({}, opts) },
    { question: m.faqQ5({}, opts), answer: m.faqA5({}, opts) },
    { question: m.faqQ6({}, opts), answer: m.faqA6({}, opts) },
  ];
}

interface GuideLinkItem {
  to: Exclude<SiteRoutePath, "/">;
  label: string;
}

export function FaqSection() {
  const [openValues, setOpenValues] = useState<string[]>(["faq-0"]);
  const items = getLandingFaqItems();

  const guideLinks: GuideLinkItem[] = [
    { to: "/alternatives/docker-desktop/", label: m.faqLink1() },
    { to: "/alternatives/lens-freelens/", label: m.faqLink2() },
    { to: "/alternatives/dbngin/", label: m.faqLink3() },
    { to: "/guides/ai-agents-mcp/", label: m.faqLink4() },
    { to: "/guides/dev-environment-setup/", label: m.faqLink5() },
  ];

  return (
    <section id="faq" className="faq-section" data-reveal>
      <div className="container">
        <div className="faq-layout">
          <div className="faq-intro">
            <span className="eyebrow-label">{m.faqEyebrow()}</span>
            <h2 className="faq-title">{m.faqTitle()}</h2>
            <p className="faq-subtitle">{m.faqSubtitle()}</p>

            <div className="faq-links-card">
              <div className="faq-links-label">{m.faqLinksLabel()}</div>
              <ul className="faq-links-list">
                {guideLinks.map((link) => (
                  <li key={link.to}>
                    <Link to={link.to} className="seo-inline-link">
                      <span>{link.label}</span>
                      <ArrowRight size={14} aria-hidden="true" />
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          </div>

          <Accordion
            className="faq-accordion"
            collapsible
            value={openValues}
            onValueChange={(details) => setOpenValues(details.value)}
          >
            {items.map((item, idx) => {
              const id = `faq-${idx}`;
              const isOpen = openValues.includes(id);
              return (
                <AccordionItem key={id} value={id} className="faq-item">
                  <AccordionItemTrigger className="faq-trigger">
                    <span className="faq-question">{item.question}</span>
                    <span className="faq-toggle-icon" aria-hidden="true">
                      {isOpen ? <Minus size={16} /> : <Plus size={16} />}
                    </span>
                  </AccordionItemTrigger>
                  <AccordionItemContent className="faq-content">
                    <AccordionItemBody className="faq-answer">{item.answer}</AccordionItemBody>
                  </AccordionItemContent>
                </AccordionItem>
              );
            })}
          </Accordion>
        </div>
      </div>
    </section>
  );
}
