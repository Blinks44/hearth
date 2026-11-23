import { CopyButton } from '@/components/CopyButton'
import * as m from '@/integrations/i18n/paraglide/messages'

const MCP_SNIPPET = `{
  "mcpServers": {
    "hearth": {
      "command": "/Applications/Hearth.app/Contents/MacOS/Hearth",
      "args": ["mcp"]
    }
  }
}`

const TOOLS = [
  { name: 'get_tools', accent: false },
  { name: 'get_projects', accent: false },
  { name: 'get_issues', accent: false },
  { name: 'get_agents', accent: false },
  { name: 'propose_fix', accent: true },
] as const

export function McpSection() {
  return (
    <section id="mcp" className="mcp-section" data-reveal>
      <div className="container">
        <div className="mcp-card">
          <div className="mcp-copy">
            <span className="pill mcp-pill">{m.mcpEyebrow()}</span>
            <h2>{m.mcpTitle()}</h2>
            <p>{m.mcpDesc()}</p>

            <div className="mcp-chips">
              {TOOLS.map((tool) => (
                <span
                  key={tool.name}
                  className={tool.accent ? 'mcp-chip mcp-chip-accent' : 'mcp-chip'}
                >
                  {tool.name}
                </span>
              ))}
            </div>

            <div className="mcp-footnote">{m.mcpFootnote()}</div>
          </div>

          <div className="mcp-code-wrap">
            <div className="mcp-code-box">
              <div className="mcp-code-bar">
                <span>mcp.json · stdio transport</span>
                <CopyButton
                  text={MCP_SNIPPET}
                  label={m.mcpCopy()}
                  className="mcp-copy-btn"
                  showLabelWhenIdle
                />
              </div>
              <pre className="mcp-pre">
                <code>{MCP_SNIPPET}</code>
              </pre>
            </div>

            <div className="mcp-approval-card">
              <div>
                <div className="mcp-approval-eyebrow">
                  <span className="hero-eyebrow-dot" aria-hidden="true" />
                  <span>{m.mcpRequestedBy()}</span>
                </div>
                <div className="mcp-approval-cmd">
                  propose_fix(&quot;path-node-shadow&quot;) → Plan ready
                </div>
                <div className="mcp-approval-hint">{m.mcpPlanHint()}</div>
              </div>
              <a href="#safety" className="preview-mini-btn preview-mini-btn-accent">
                {m.mcpReviewBtn()}
              </a>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
