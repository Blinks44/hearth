import { Check } from "lucide-react";
import { useState } from "react";
import { BrandLogo } from "@/components/BrandLogo";
import * as m from "@/integrations/i18n/paraglide/messages";
import type { ColorScheme } from "@/lib/theme";

interface BentoGridProps {
  theme: ColorScheme;
}

const BENTO_CLEANUP = [
  { id: "nm", gb: 11.2 },
  { id: "pnpm", gb: 6.4 },
  { id: "docker", gb: 4.8 },
  { id: "ai", gb: 2.2 },
] as const;

const BENTO_BARS = [28, 44, 58, 42, 74, 92, 66];

export function BentoGrid({ theme }: BentoGridProps) {
  const [checked, setChecked] = useState<Record<string, boolean>>({
    nm: true,
    pnpm: true,
    docker: true,
    ai: false,
  });

  const selectedGb = BENTO_CLEANUP.reduce((sum, item) => sum + (checked[item.id] ? item.gb : 0), 0);

  const toggle = (id: string) => {
    setChecked((prev) => ({ ...prev, [id]: !prev[id] }));
  };

  return (
    <section className="bento-section" data-reveal>
      <div className="container">
        <div className="section-header">
          <span className="eyebrow-label">{m.bentoEyebrow()}</span>
          <h2 className="section-title">{m.bentoTitle()}</h2>
          <p className="section-subtitle">{m.bentoSubtitle()}</p>
        </div>

        <div className="bento-grid">
          {/* 1. Docker & Colima */}
          <article className="bento-card">
            <div>
              <span className="pill pill-info">{m.bentoDockerPill()}</span>
            </div>
            <h3>{m.bentoDockerTitle()}</h3>
            <p>{m.bentoDockerDesc()}</p>
            <div className="bento-widget">
              <div className="preview-row" style={{ fontSize: "0.73rem", padding: "0 0.25rem" }}>
                <div style={{ display: "inline-flex", alignItems: "center", gap: "0.45rem" }}>
                  <strong className="mono">colima</strong>
                  <span
                    className="pill pill-success"
                    style={{ textTransform: "none", letterSpacing: 0 }}
                  >
                    {m.tabDockerManaged()}
                  </span>
                </div>
                <span className="widget-row-meta">RAM · 1.4 / 4.0 GB</span>
              </div>
              {[
                {
                  logo: "postgresql",
                  name: "shop-postgres",
                  img: "postgres:16-alpine",
                  port: ":5432",
                },
                { logo: "redis", name: "shop-redis", img: "redis:7-alpine", port: ":6379" },
                { logo: "nginx", name: "shop-gateway", img: "nginx:1.27", port: ":8080" },
              ].map((row) => (
                <div key={row.name} className="widget-row widget-row-4col">
                  <span className="tool-cell">
                    <BrandLogo name={row.logo} alt={row.name} theme={theme} size={15} />
                    <span>{row.name}</span>
                  </span>
                  <span className="widget-row-meta">{row.img}</span>
                  <span className="widget-row-meta">{row.port}</span>
                  <span className="status-chip status-running widget-row-status">
                    <span className="status-chip-dot" />
                    {m.tabDockerRunning()}
                  </span>
                </div>
              ))}
            </div>
          </article>

          {/* 2. Kubernetes */}
          <article className="bento-card">
            <div>
              <span className="pill pill-info">{m.bentoK8sPill()}</span>
            </div>
            <h3>{m.bentoK8sTitle()}</h3>
            <p>{m.bentoK8sDesc()}</p>
            <div className="bento-widget">
              <div className="preview-row" style={{ fontSize: "0.73rem", padding: "0 0.25rem" }}>
                <span className="mono" style={{ fontWeight: 600 }}>
                  kind-dev / default / Pods
                </span>
                <kbd
                  className="mono"
                  style={{
                    fontSize: "0.68rem",
                    padding: "0.1rem 0.35rem",
                    borderRadius: "4px",
                    background: "var(--hearth-bg-elevated)",
                    border: "1px solid var(--hearth-border)",
                    color: "var(--hearth-text-muted)",
                  }}
                >
                  ⌘J
                </kbd>
              </div>
              <div className="widget-row widget-row-3col">
                <span className="mono widget-row-primary">api-74f98c-xk2m</span>
                <span className="widget-row-meta">1/1 · 12m</span>
                <span className="status-chip status-running widget-row-status">
                  <span className="status-chip-dot" />
                  {m.tabDockerRunning()}
                </span>
              </div>
              <div className="widget-row widget-row-3col">
                <span className="mono widget-row-primary">worker-4b4a1-9q7p</span>
                <span className="widget-row-meta" style={{ color: "var(--hearth-danger)" }}>
                  0/1 · OOMKilled
                </span>
                <span className="status-chip status-danger widget-row-status">
                  <span className="status-chip-dot" />
                  {m.tabDockerCrash()}
                </span>
              </div>
              <div className="widget-row widget-row-3col">
                <span className="mono widget-row-primary">postgres-0</span>
                <span className="widget-row-meta">1/1 · 3d</span>
                <span className="status-chip status-running widget-row-status">
                  <span className="status-chip-dot" />
                  {m.tabDockerRunning()}
                </span>
              </div>
            </div>
          </article>

          {/* 3. AI Agents & Spend */}
          <article className="bento-card">
            <div>
              <span className="pill pill-accent">{m.bentoAiPill()}</span>
            </div>
            <h3>{m.bentoAiTitle()}</h3>
            <p>{m.bentoAiDesc()}</p>
            <div className="bento-widget">
              <div className="agent-icon-strip">
                <div className="agent-icons">
                  <BrandLogo name="claude" alt="Claude" theme={theme} size={18} />
                  <BrandLogo name="openai" alt="Codex" theme={theme} hasDark size={18} />
                  <BrandLogo name="cursor" alt="Cursor" theme={theme} hasDark size={18} />
                  <BrandLogo name="githubcopilot" alt="Copilot" theme={theme} hasDark size={18} />
                </div>
                <span className="status-chip status-running mono" style={{ fontSize: "0.68rem" }}>
                  ✓ ~/.agents/AGENTS.md linked
                </span>
              </div>
              <div className="spend-card" style={{ padding: "0.75rem 0.85rem" }}>
                <div>
                  <div
                    style={{
                      fontSize: "0.64rem",
                      fontWeight: 700,
                      letterSpacing: "0.08em",
                      textTransform: "uppercase",
                      color: "var(--hearth-text-muted)",
                    }}
                  >
                    {m.tabAiSpendLabel()}
                  </div>
                  <div className="spend-amount" style={{ fontSize: "1.15rem" }}>
                    $28.40
                  </div>
                  <div style={{ fontSize: "0.7rem", color: "var(--hearth-text-muted)" }}>
                    842k tokens · 4 agents
                  </div>
                </div>
                <div className="spend-bars" style={{ height: "38px" }} aria-hidden="true">
                  {BENTO_BARS.map((h, i) => (
                    <div
                      key={i}
                      className="spend-bar"
                      data-peak={i === 5}
                      style={{ width: "11px", height: `${h}%` }}
                    />
                  ))}
                </div>
              </div>
            </div>
          </article>

          {/* 4. Safe Cleanup (Interactive!) */}
          <article className="bento-card">
            <div className="preview-row">
              <span className="pill pill-success">{m.bentoCleanupPill()}</span>
              <span
                className="mono"
                style={{ fontSize: "0.75rem", fontWeight: 700, color: "var(--hearth-accent)" }}
              >
                {m.bentoCleanupSelectedNote({ selected: selectedGb.toFixed(1) })}
              </span>
            </div>
            <h3>{m.bentoCleanupTitle()}</h3>
            <p>{m.bentoCleanupDesc()}</p>
            <div className="bento-widget">
              {[
                {
                  id: "nm",
                  title: m.tabCleanupItem1Title(),
                  sub: m.tabCleanupItem1Sub(),
                  gb: 11.2,
                },
                {
                  id: "pnpm",
                  title: m.tabCleanupItem2Title(),
                  sub: m.tabCleanupItem2Sub(),
                  gb: 6.4,
                },
                {
                  id: "docker",
                  title: m.tabCleanupItem3Title(),
                  sub: m.tabCleanupItem3Sub(),
                  gb: 4.8,
                },
                { id: "ai", title: m.tabCleanupItem4Title(), sub: m.tabCleanupItem4Sub(), gb: 2.2 },
              ].map((item) => {
                const isChecked = Boolean(checked[item.id]);
                return (
                  <div
                    key={item.id}
                    role="checkbox"
                    aria-checked={isChecked}
                    tabIndex={0}
                    className="cleanup-row cleanup-row-bento"
                    onClick={() => toggle(item.id)}
                    onKeyDown={(e) => {
                      if (e.key === " " || e.key === "Enter") {
                        e.preventDefault();
                        toggle(item.id);
                      }
                    }}
                  >
                    <div className="cleanup-row-left">
                      <span className="cleanup-checkbox-box" data-checked={isChecked}>
                        {isChecked ? <Check size={11} strokeWidth={3} aria-hidden="true" /> : null}
                      </span>
                      <span className="cleanup-row-title" style={{ fontSize: "0.76rem" }}>
                        {item.title}
                      </span>
                    </div>
                    <span className="cleanup-bento-sub">{item.sub}</span>
                    <span className="cleanup-row-size" style={{ fontSize: "0.75rem" }}>
                      {item.gb.toFixed(1)} GB
                    </span>
                  </div>
                );
              })}
            </div>
          </article>

          {/* 5. Local Databases & Ports */}
          <article className="bento-card">
            <div>
              <span className="pill pill-info">{m.bentoDbPill()}</span>
            </div>
            <h3>{m.bentoDbTitle()}</h3>
            <p>{m.bentoDbDesc()}</p>
            <div className="bento-widget">
              <div className="preview-row" style={{ padding: "0 0.2rem" }}>
                <span
                  style={{
                    fontSize: "0.66rem",
                    fontWeight: 700,
                    letterSpacing: "0.07em",
                    textTransform: "uppercase",
                    color: "var(--hearth-text-muted)",
                  }}
                >
                  {m.bentoDbHeader()}
                </span>
                <span
                  style={{
                    fontSize: "0.7rem",
                    fontWeight: 600,
                    padding: "0.2rem 0.5rem",
                    borderRadius: "6px",
                    background: "var(--hearth-bg-elevated)",
                    border: "1px solid var(--hearth-border)",
                    color: "var(--hearth-text-secondary)",
                  }}
                >
                  {m.bentoDbInstallBtn()}
                </span>
              </div>
              <div className="widget-row widget-row-3col">
                <span className="tool-cell">
                  <BrandLogo name="postgresql" alt="PostgreSQL" theme={theme} size={15} />
                  <span>PostgreSQL 16</span>
                </span>
                <span className="widget-row-meta">brew · :5432</span>
                <span className="status-chip status-running widget-row-status">
                  <span className="status-chip-dot" />
                  {m.tabDockerRunning()}
                </span>
              </div>
              <div className="widget-row widget-row-3col">
                <span className="tool-cell">
                  <BrandLogo name="redis" alt="Redis" theme={theme} size={15} />
                  <span>Redis 7.4</span>
                </span>
                <span className="widget-row-meta">docker · :6379</span>
                <span className="status-chip status-running widget-row-status">
                  <span className="status-chip-dot" />
                  {m.tabDockerRunning()}
                </span>
              </div>
              <div className="widget-row widget-row-3col">
                <span className="tool-cell">
                  <BrandLogo name="mysql" alt="MySQL" theme={theme} hasDark size={15} />
                  <span>MySQL 8.4</span>
                </span>
                <span className="widget-row-meta">port :3306</span>
                <span className="status-chip status-warning widget-row-status">
                  <span className="status-chip-dot" />
                  {m.bentoDbConflict()}
                </span>
              </div>
            </div>
          </article>

          {/* 6. Runtimes & Shell Startup */}
          <article className="bento-card">
            <div>
              <span className="pill pill-warning">{m.bentoShellPill()}</span>
            </div>
            <h3>{m.bentoShellTitle()}</h3>
            <p>{m.bentoShellDesc()}</p>
            <div className="bento-widget">
              <div className="preview-row" style={{ padding: "0 0.2rem" }}>
                <span
                  style={{
                    fontSize: "0.66rem",
                    fontWeight: 700,
                    letterSpacing: "0.07em",
                    textTransform: "uppercase",
                    color: "var(--hearth-text-muted)",
                  }}
                >
                  {m.bentoShellHeader()}
                </span>
                <span className="status-chip status-running mono" style={{ fontSize: "0.68rem" }}>
                  1.4s → 0.2s
                </span>
              </div>
              <div className="widget-row widget-row-3col">
                <span className="mono widget-row-primary">nvm.sh (line 18)</span>
                <span className="widget-row-meta">840 ms · switch to mise</span>
                <span className="status-chip status-danger widget-row-status">
                  <span className="status-chip-dot" />
                  {m.bentoShellSlow()}
                </span>
              </div>
              <div className="widget-row widget-row-3col">
                <span className="mono widget-row-primary">which -a python3</span>
                <span className="widget-row-meta">v3.12 · brew shadowed</span>
                <span className="status-chip status-warning widget-row-status">
                  <span className="status-chip-dot" />
                  {m.bentoShellDuplicate()}
                </span>
              </div>
              <div className="widget-row widget-row-3col">
                <span className="mono widget-row-primary">which -a go</span>
                <span className="widget-row-meta">1.27.3 · GOPATH ok</span>
                <span className="status-chip status-running widget-row-status">
                  <span className="status-chip-dot" />
                  {m.bentoShellClean()}
                </span>
              </div>
            </div>
          </article>
        </div>
      </div>
    </section>
  );
}
