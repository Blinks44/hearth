import { Link } from "@tanstack/react-router";
import { ArrowRight } from "lucide-react";
import type { WorkflowTabId } from "@/components/WorkflowShowcase";
import * as m from "@/integrations/i18n/paraglide/messages";

interface ReplacesSectionProps {
  onSelectWorkflow?: (tab: WorkflowTabId) => void;
}

interface ReplaceRow {
  workflow: string;
  tools: string;
  hearth: string;
  ctaLabel: string;
  to?:
    | "/alternatives/docker-desktop/"
    | "/alternatives/lens-freelens/"
    | "/alternatives/dbngin/"
    | "/guides/ai-agents-mcp/";
  workflowTab?: WorkflowTabId;
}

export function ReplacesSection({ onSelectWorkflow }: ReplacesSectionProps) {
  const rows: ReplaceRow[] = [
    {
      workflow: m.replacesRow1Workflow(),
      tools: m.replacesRow1Tools(),
      hearth: m.replacesRow1Hearth(),
      ctaLabel: m.replacesRow1Cta(),
      to: "/alternatives/docker-desktop/",
    },
    {
      workflow: m.replacesRow2Workflow(),
      tools: m.replacesRow2Tools(),
      hearth: m.replacesRow2Hearth(),
      ctaLabel: m.replacesRow2Cta(),
      to: "/alternatives/lens-freelens/",
    },
    {
      workflow: m.replacesRow3Workflow(),
      tools: m.replacesRow3Tools(),
      hearth: m.replacesRow3Hearth(),
      ctaLabel: m.replacesRow3Cta(),
      to: "/alternatives/dbngin/",
    },
    {
      workflow: m.replacesRow4Workflow(),
      tools: m.replacesRow4Tools(),
      hearth: m.replacesRow4Hearth(),
      ctaLabel: m.replacesRow4Cta(),
      workflowTab: "overview",
    },
    {
      workflow: m.replacesRow5Workflow(),
      tools: m.replacesRow5Tools(),
      hearth: m.replacesRow5Hearth(),
      ctaLabel: m.replacesRow5Cta(),
      workflowTab: "cleanup",
    },
    {
      workflow: m.replacesRow6Workflow(),
      tools: m.replacesRow6Tools(),
      hearth: m.replacesRow6Hearth(),
      ctaLabel: m.replacesRow6Cta(),
      to: "/guides/ai-agents-mcp/",
    },
  ];

  return (
    <section id="alternatives" className="replaces-section" data-reveal>
      <div className="container">
        <div className="section-header">
          <span className="eyebrow-label">{m.replacesEyebrow()}</span>
          <h2 className="section-title">{m.replacesTitle()}</h2>
          <p className="section-subtitle">{m.replacesSubtitle()}</p>
        </div>

        <div className="replaces-card">
          <div className="replaces-head" aria-hidden="true">
            <span>{m.replacesColWorkflow()}</span>
            <span>{m.replacesColTools()}</span>
            <span>{m.replacesColHearth()}</span>
            <span className="replaces-col-right">{m.replacesColDetails()}</span>
          </div>

          <div className="replaces-body">
            {rows.map((row) => (
              <div key={row.workflow} className="replaces-row">
                <div className="replaces-workflow">{row.workflow}</div>
                <div className="replaces-tools mono">{row.tools}</div>
                <div className="replaces-hearth">{row.hearth}</div>
                <div className="replaces-action">
                  {row.to ? (
                    <Link to={row.to} className="seo-inline-link">
                      <span>{row.ctaLabel}</span>
                      <ArrowRight size={14} aria-hidden="true" />
                    </Link>
                  ) : (
                    <a
                      href="#features"
                      className="seo-inline-link"
                      onClick={() => {
                        if (row.workflowTab) {
                          onSelectWorkflow?.(row.workflowTab);
                        }
                      }}
                    >
                      <span>{row.ctaLabel}</span>
                      <ArrowRight size={14} aria-hidden="true" />
                    </a>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>

        <div className="honest-scope-bar">
          <p>{m.replacesHonestScope()}</p>
          <span className="honest-scope-badge mono">{m.replacesHonestBadge()}</span>
        </div>
      </div>
    </section>
  );
}
