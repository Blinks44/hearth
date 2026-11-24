import {
  Dialog,
  DialogBackdrop,
  DialogBody,
  DialogCloseIcon,
  DialogContent,
  DialogHeader,
  DialogPositioner,
  DialogTitle,
} from '@moduix/react/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@moduix/react/tabs'
import {
  AlertTriangle,
  ArrowRight,
  Check,
  FolderOpen,
  Image as ImageIcon,
  ShieldCheck,
} from 'lucide-react'
import { useState } from 'react'
import { BrandLogo } from '@/components/BrandLogo'
import { CopyButton } from '@/components/CopyButton'
import * as m from '@/integrations/i18n/paraglide/messages'
import type { ColorScheme } from '@/lib/theme'

export type WorkflowTabId = 'overview' | 'projects' | 'docker' | 'ai' | 'cleanup' | 'setup'

interface WorkflowShowcaseProps {
  theme: ColorScheme
  activeTab?: WorkflowTabId
  onTabChange?: (tab: WorkflowTabId) => void
}

const CLEANUP_GROUPS = [
  { id: 'nm', gb: 11.2, defaultChecked: true },
  { id: 'pnpm', gb: 6.4, defaultChecked: true },
  { id: 'docker', gb: 4.8, defaultChecked: true },
  { id: 'ai', gb: 2.2, defaultChecked: false },
] as const

const SETUP_GROUPS = [
  { id: 'foundation', steps: 2, defaultOn: true },
  { id: 'languages', steps: 2, defaultOn: true },
  { id: 'infra', steps: 2, defaultOn: true },
  { id: 'ai', steps: 1, defaultOn: true },
] as const

const SPEND_BARS = [
  { h: 28, peak: false },
  { h: 44, peak: false },
  { h: 58, peak: false },
  { h: 42, peak: false },
  { h: 74, peak: false },
  { h: 92, peak: true },
  { h: 66, peak: false },
] as const

const SCREENSHOT_BY_TAB: Record<WorkflowTabId, string> = {
  overview: 'overview',
  projects: 'projects',
  docker: 'docker',
  ai: 'ai-usage',
  cleanup: 'cleanup',
  setup: 'setup',
}

export function WorkflowShowcase({
  theme,
  activeTab: controlledTab,
  onTabChange,
}: WorkflowShowcaseProps) {
  const [internalTab, setInternalTab] = useState<WorkflowTabId>('projects')
  const activeTab = controlledTab ?? internalTab
  const setActiveTab = (next: WorkflowTabId) => {
    setInternalTab(next)
    onTabChange?.(next)
  }
  const [projectPlanned, setProjectPlanned] = useState(false)
  const [cleanupChecked, setCleanupChecked] = useState<Record<string, boolean>>({
    nm: true,
    pnpm: true,
    docker: true,
    ai: false,
  })
  const [cleanupDone, setCleanupDone] = useState(false)
  const [setupOn, setSetupOn] = useState<Record<string, boolean>>({
    foundation: true,
    languages: true,
    infra: true,
    ai: true,
  })
  const [setupPlanned, setSetupPlanned] = useState(false)
  const [lightboxTab, setLightboxTab] = useState<WorkflowTabId>('projects')
  const [lightboxOpen, setLightboxOpen] = useState(false)

  const totalGb = CLEANUP_GROUPS.reduce((sum, g) => sum + g.gb, 0)
  const selectedGb = CLEANUP_GROUPS.reduce(
    (sum, g) => sum + (cleanupChecked[g.id] ? g.gb : 0),
    0,
  )
  const checkedCount = CLEANUP_GROUPS.filter((g) => cleanupChecked[g.id]).length
  const uncheckedCount = CLEANUP_GROUPS.length - checkedCount

  const setupStepCount = SETUP_GROUPS.reduce(
    (sum, g) => sum + (setupOn[g.id] ? g.steps : 0),
    0,
  )

  const tabs: Array<{
    id: WorkflowTabId
    label: string
    pill: string
    title: string
    desc: string
    bullets: [string, string, string]
  }> = [
    {
      id: 'overview',
      label: m.tabOverviewLabel(),
      pill: m.tabOverviewPill(),
      title: m.tabOverviewTitle(),
      desc: m.tabOverviewDesc(),
      bullets: [m.tabOverviewBullet1(), m.tabOverviewBullet2(), m.tabOverviewBullet3()],
    },
    {
      id: 'projects',
      label: m.tabProjectsLabel(),
      pill: m.tabProjectsPill(),
      title: m.tabProjectsTitle(),
      desc: m.tabProjectsDesc(),
      bullets: [m.tabProjectsBullet1(), m.tabProjectsBullet2(), m.tabProjectsBullet3()],
    },
    {
      id: 'docker',
      label: m.tabDockerLabel(),
      pill: m.tabDockerPill(),
      title: m.tabDockerTitle(),
      desc: m.tabDockerDesc(),
      bullets: [m.tabDockerBullet1(), m.tabDockerBullet2(), m.tabDockerBullet3()],
    },
    {
      id: 'ai',
      label: m.tabAiLabel(),
      pill: m.tabAiPill(),
      title: m.tabAiTitle(),
      desc: m.tabAiDesc(),
      bullets: [m.tabAiBullet1(), m.tabAiBullet2(), m.tabAiBullet3()],
    },
    {
      id: 'cleanup',
      label: m.tabCleanupLabel(),
      pill: m.tabCleanupPill(),
      title: m.tabCleanupTitle(),
      desc: m.tabCleanupDesc(),
      bullets: [m.tabCleanupBullet1(), m.tabCleanupBullet2(), m.tabCleanupBullet3()],
    },
    {
      id: 'setup',
      label: m.tabSetupLabel(),
      pill: m.tabSetupPill(),
      title: m.tabSetupTitle(),
      desc: m.tabSetupDesc(),
      bullets: [m.tabSetupBullet1(), m.tabSetupBullet2(), m.tabSetupBullet3()],
    },
  ]

  const toggleCleanup = (id: string) => {
    setCleanupDone(false)
    setCleanupChecked((prev) => ({ ...prev, [id]: !prev[id] }))
  }

  const toggleSetup = (id: string) => {
    setSetupPlanned(false)
    setSetupOn((prev) => ({ ...prev, [id]: !prev[id] }))
  }

  const renderPreview = (tabId: WorkflowTabId) => {
    switch (tabId) {
      case 'overview':
        return (
          <div className="preview-surface">
            <div className="preview-box preview-box-warm">
              <div className="preview-row">
                <div>
                  <span className="status-chip status-warning">
                    <span className="status-chip-dot" />
                    {m.tabOverviewIssuesBadge()}
                  </span>
                  <div
                    style={{
                      display: 'flex',
                      flexWrap: 'wrap',
                      alignItems: 'baseline',
                      gap: '0.6rem',
                      marginTop: '0.45rem',
                    }}
                  >
                    <strong style={{ fontSize: '1.15rem' }}>{m.tabOverviewStatus()}</strong>
                    <span style={{ fontSize: '0.76rem', color: 'var(--hearth-text-secondary)' }}>
                      {m.tabOverviewSub()}
                    </span>
                  </div>
                </div>
                <CopyButton
                  text="Hearth Report: 14 tools · 5 AI agents · 2 issues · 24.6 GB reclaimable"
                  label={m.tabOverviewCopyReport()}
                  className="preview-mini-btn"
                  showLabelWhenIdle
                />
              </div>
            </div>

            <div className="preview-box">
              <div className="preview-row">
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.65rem', minWidth: 0 }}>
                  <AlertTriangle size={17} color="var(--hearth-warning)" aria-hidden="true" />
                  <div style={{ minWidth: 0 }}>
                    <div style={{ fontSize: '0.82rem', fontWeight: 650 }}>
                      {m.tabOverviewIssue1Title()}
                    </div>
                    <div style={{ fontSize: '0.74rem', color: 'var(--hearth-text-secondary)' }}>
                      {m.tabOverviewIssue1Desc()}
                    </div>
                  </div>
                </div>
                <a href="#safety" className="preview-mini-btn preview-mini-btn-dark">
                  {m.tabOverviewIssue1Btn()}
                </a>
              </div>
            </div>

            <div className="preview-box">
              <div className="preview-row">
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.65rem', minWidth: 0 }}>
                  <AlertTriangle size={17} color="var(--hearth-warning)" aria-hidden="true" />
                  <div style={{ minWidth: 0 }}>
                    <div style={{ fontSize: '0.82rem', fontWeight: 650 }}>
                      {m.tabOverviewIssue2Title()}
                    </div>
                    <div style={{ fontSize: '0.74rem', color: 'var(--hearth-text-secondary)' }}>
                      {m.tabOverviewIssue2Desc()}
                    </div>
                  </div>
                </div>
                <a href="#mcp" className="preview-mini-btn">
                  {m.tabOverviewIssue2Btn()}
                </a>
              </div>
            </div>

            <div className="preview-box" style={{ fontSize: '0.74rem', color: 'var(--hearth-text-secondary)' }}>
              <div
                style={{
                  fontSize: '0.66rem',
                  fontWeight: 700,
                  letterSpacing: '0.08em',
                  textTransform: 'uppercase',
                  color: 'var(--hearth-text-muted)',
                  marginBottom: '0.35rem',
                }}
              >
                {m.tabOverviewRecentTitle()}
              </div>
              <div>• {m.tabOverviewRecent1()}</div>
              <div style={{ marginTop: '0.2rem' }}>• {m.tabOverviewRecent2()}</div>
            </div>
          </div>
        )

      case 'projects':
        return (
          <div className="preview-surface">
            <div className="preview-box">
              <div className="preview-row">
                <div>
                  <div style={{ fontSize: '0.96rem', fontWeight: 700 }}>legacy-admin</div>
                  <div className="mono" style={{ fontSize: '0.72rem', color: 'var(--hearth-text-muted)', marginTop: '0.15rem' }}>
                    ~/work/legacy-admin · .nvmrc · compose.yaml
                  </div>
                </div>
                <button
                  type="button"
                  className={
                    projectPlanned
                      ? 'preview-mini-btn preview-mini-btn-dark'
                      : 'preview-mini-btn preview-mini-btn-accent'
                  }
                  onClick={() => setProjectPlanned((v) => !v)}
                >
                  {projectPlanned ? m.tabProjectsPreparedBtn() : m.tabProjectsPrepareBtn()}
                </button>
              </div>
            </div>

            <div className="preview-box preview-table-wrap">
              <table className="preview-table">
                <thead>
                  <tr>
                    <th>{m.tabProjectsColTool()}</th>
                    <th>{m.tabProjectsColNeeds()}</th>
                    <th>{m.tabProjectsColRuns()}</th>
                    <th style={{ textAlign: 'right' }}>{m.tabProjectsColStatus()}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td>
                      <span className="tool-cell">
                        <BrandLogo name="nodedotjs" alt="Node.js" theme={theme} size={17} />
                        Node.js
                      </span>
                    </td>
                    <td className="mono">18.20 (.nvmrc)</td>
                    <td className="mono" style={{ color: 'var(--hearth-text-secondary)' }}>
                      {projectPlanned ? '18.20 (local mise)' : '22.12 (global)'}
                    </td>
                    <td style={{ textAlign: 'right' }}>
                      <span
                        className={`status-chip ${projectPlanned ? 'status-running' : 'status-warning'}`}
                      >
                        <span className="status-chip-dot" />
                        {projectPlanned ? m.tabProjectsReady() : m.tabProjectsMissing()}
                      </span>
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <span className="tool-cell">
                        <BrandLogo name="pnpm" alt="pnpm" theme={theme} hasDark size={17} />
                        pnpm
                      </span>
                    </td>
                    <td className="mono">9.11</td>
                    <td className="mono" style={{ color: 'var(--hearth-text-secondary)' }}>
                      9.11.0 (Corepack)
                    </td>
                    <td style={{ textAlign: 'right' }}>
                      <span className="status-chip status-running">
                        <span className="status-chip-dot" />
                        {m.tabProjectsReady()}
                      </span>
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <span className="tool-cell">
                        <BrandLogo name="docker" alt="Docker" theme={theme} size={17} />
                        Docker
                      </span>
                    </td>
                    <td className="mono">postgres:16, redis:7</td>
                    <td className="mono" style={{ color: 'var(--hearth-text-secondary)' }}>
                      Colima · running
                    </td>
                    <td style={{ textAlign: 'right' }}>
                      <span className="status-chip status-running">
                        <span className="status-chip-dot" />
                        {m.tabProjectsReady()}
                      </span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>

            <div className="preview-plan-bar">
              <strong>{m.tabProjectsPlanSummary()}</strong>
              <span>{m.tabProjectsPlanNote()}</span>
            </div>
          </div>
        )

      case 'docker':
        return (
          <div className="preview-surface">
            <div className="preview-box" style={{ display: 'flex', flexDirection: 'column', gap: '0.45rem' }}>
              <div className="preview-row" style={{ paddingBottom: '0.4rem', borderBottom: '1px solid var(--hearth-border)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  <BrandLogo name="docker" alt="Colima" theme={theme} size={17} />
                  <strong className="mono" style={{ fontSize: '0.82rem' }}>colima</strong>
                  <span className="pill pill-success" style={{ textTransform: 'none', letterSpacing: 0 }}>
                    {m.tabDockerManaged()}
                  </span>
                </div>
                <span className="mono" style={{ fontSize: '0.72rem', color: 'var(--hearth-text-muted)' }}>
                  4 CPU · 1.4 / 4.0 GB RAM
                </span>
              </div>
              {[
                { logo: 'postgresql', name: 'shop-postgres', img: 'postgres:16-alpine', port: ':5432' },
                { logo: 'redis', name: 'shop-redis', img: 'redis:7-alpine', port: ':6379' },
                { logo: 'nginx', name: 'shop-gateway', img: 'nginx:1.27', port: ':8080' },
              ].map((c) => (
                <div key={c.name} className="preview-grid-4col">
                  <span className="tool-cell">
                    <BrandLogo name={c.logo} alt={c.name} theme={theme} size={16} />
                    <span>{c.name}</span>
                  </span>
                  <span className="mono widget-row-meta">{c.img}</span>
                  <span className="mono widget-row-meta" style={{ color: 'var(--hearth-text-secondary)' }}>
                    {c.port}
                  </span>
                  <span className="status-chip status-running widget-row-status">
                    <span className="status-chip-dot" />
                    {m.tabDockerRunning()}
                  </span>
                </div>
              ))}
            </div>

            <div className="preview-box" style={{ display: 'flex', flexDirection: 'column', gap: '0.45rem' }}>
              <div className="preview-row" style={{ paddingBottom: '0.4rem', borderBottom: '1px solid var(--hearth-border)' }}>
                <span className="tool-cell mono" style={{ fontSize: '0.78rem' }}>
                  <BrandLogo name="kubernetes" alt="Kubernetes" theme={theme} size={16} />
                  kind-dev / default / Pods
                </span>
                <kbd
                  className="mono"
                  style={{
                    fontSize: '0.68rem',
                    padding: '0.15rem 0.4rem',
                    borderRadius: '5px',
                    background: 'var(--hearth-surface)',
                    border: '1px solid var(--hearth-border)',
                    color: 'var(--hearth-text-muted)',
                  }}
                >
                  ⌘J
                </kbd>
              </div>
              <div className="preview-grid-3col">
                <span className="mono widget-row-primary">api-74f98c-xk2m</span>
                <span className="mono widget-row-meta">1/1 · 12m · 42 MB</span>
                <span className="status-chip status-running widget-row-status">
                  <span className="status-chip-dot" />
                  {m.tabDockerRunning()}
                </span>
              </div>
              <div className="preview-grid-3col">
                <span className="mono widget-row-primary">worker-4b4a1-9q7p</span>
                <span className="mono widget-row-meta" style={{ color: 'var(--hearth-danger)' }}>
                  0/1 · OOMKilled · exit 137
                </span>
                <span className="status-chip status-danger widget-row-status">
                  <span className="status-chip-dot" />
                  {m.tabDockerCrash()}
                </span>
              </div>
            </div>
          </div>
        )

      case 'ai':
        return (
          <div className="preview-surface">
            {[
              { logo: 'claude', name: 'Claude Code', ver: 'v1.0.24', meta: '12 skills · 4 MCP · AGENTS.md', conflict: false, hasDark: false },
              { logo: 'openai', name: 'Codex CLI', ver: 'v0.18.2', meta: '8 skills · 4 MCP · AGENTS.md', conflict: false, hasDark: true },
              { logo: 'cursor', name: 'Cursor', ver: '0.46.8', meta: '9 rules · 6 MCP · 1 conflict', conflict: true, hasDark: true },
              { logo: 'githubcopilot', name: 'Copilot CLI', ver: 'v1.1.0', meta: '4 skills · 2 MCP · AGENTS.md', conflict: false, hasDark: true },
            ].map((agent) => (
              <div
                key={agent.name}
                className="preview-box"
                style={
                  agent.conflict
                    ? { borderColor: 'var(--hearth-warning-border)', padding: '0.68rem 0.9rem' }
                    : { padding: '0.68rem 0.9rem' }
                }
              >
                <div className="preview-grid-ai">
                  <span className="tool-cell">
                    <BrandLogo name={agent.logo} alt={agent.name} theme={theme} hasDark={agent.hasDark} size={17} />
                    <span>{agent.name}</span>
                  </span>
                  <span className="mono widget-row-meta">{agent.ver}</span>
                  <span className="widget-row-meta" style={{ fontFamily: 'var(--moduix-font-sans)', color: 'var(--hearth-text-secondary)' }}>
                    {agent.meta}
                  </span>
                  <span className={`status-chip widget-row-status ${agent.conflict ? 'status-warning' : 'status-running'}`}>
                    <span className="status-chip-dot" />
                    {agent.conflict ? m.tabAiConflict() : m.tabAiSynced()}
                  </span>
                </div>
              </div>
            ))}

            <div className="spend-card">
              <div>
                <div style={{ fontSize: '0.66rem', fontWeight: 700, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--hearth-text-muted)' }}>
                  {m.tabAiSpendLabel()}
                </div>
                <div className="spend-amount">$28.40</div>
                <div style={{ fontSize: '0.72rem', color: 'var(--hearth-text-muted)', marginTop: '0.15rem' }}>
                  {m.tabAiSpendMeta()}
                </div>
              </div>
              <div className="spend-bars" aria-hidden="true">
                {SPEND_BARS.map((bar, idx) => (
                  <div
                    key={idx}
                    className="spend-bar"
                    data-peak={bar.peak}
                    style={{ height: `${bar.h}%` }}
                  />
                ))}
              </div>
            </div>
          </div>
        )

      case 'cleanup':
        return (
          <div className="preview-surface">
            <div className="preview-box">
              <div className="preview-row">
                <div>
                  <strong style={{ fontSize: '0.88rem' }}>
                    {m.tabCleanupSelected({
                      selected: selectedGb.toFixed(1),
                      total: totalGb.toFixed(1),
                    })}
                  </strong>
                  <div style={{ fontSize: '0.73rem', color: 'var(--hearth-text-muted)', marginTop: '0.15rem' }}>
                    {m.tabCleanupSelectionMeta({
                      checked: String(checkedCount),
                      unchecked: String(uncheckedCount),
                    })}
                  </div>
                </div>
                <button
                  type="button"
                  className={`preview-mini-btn ${cleanupDone ? 'preview-mini-btn-dark' : 'preview-mini-btn-accent'}`}
                  disabled={selectedGb === 0 && !cleanupDone}
                  onClick={() => setCleanupDone((prev) => !prev)}
                >
                  {cleanupDone
                    ? m.tabCleanupCleanedBtn({ selected: selectedGb.toFixed(1) })
                    : m.tabCleanupCleanBtn({ selected: selectedGb.toFixed(1) })}
                </button>
              </div>
            </div>

            {[
              { id: 'nm', title: m.tabCleanupItem1Title(), sub: m.tabCleanupItem1Sub(), gb: 11.2 },
              { id: 'pnpm', title: m.tabCleanupItem2Title(), sub: m.tabCleanupItem2Sub(), gb: 6.4 },
              { id: 'docker', title: m.tabCleanupItem3Title(), sub: m.tabCleanupItem3Sub(), gb: 4.8 },
              { id: 'ai', title: m.tabCleanupItem4Title(), sub: m.tabCleanupItem4Sub(), gb: 2.2 },
            ].map((item) => {
              const checked = Boolean(cleanupChecked[item.id])
              return (
                <div
                  key={item.id}
                  role="checkbox"
                  aria-checked={checked}
                  tabIndex={0}
                  className="cleanup-row"
                  onClick={() => toggleCleanup(item.id)}
                  onKeyDown={(e) => {
                    if (e.key === ' ' || e.key === 'Enter') {
                      e.preventDefault()
                      toggleCleanup(item.id)
                    }
                  }}
                >
                  <div className="cleanup-row-left">
                    <span className="cleanup-checkbox-box" data-checked={checked}>
                      {checked ? <Check size={12} strokeWidth={3} aria-hidden="true" /> : null}
                    </span>
                    <div style={{ minWidth: 0 }}>
                      <div className="cleanup-row-title">{item.title}</div>
                      <div className="cleanup-row-sub">{item.sub}</div>
                    </div>
                  </div>
                  <div style={{ display: 'inline-flex', alignItems: 'center', gap: '0.45rem' }}>
                    <span className="cleanup-row-size">{item.gb.toFixed(1)} GB</span>
                    <FolderOpen size={14} color="var(--hearth-text-muted)" aria-hidden="true" />
                  </div>
                </div>
              )
            })}

            <div className="protected-bar">
              <ShieldCheck size={15} aria-hidden="true" style={{ flexShrink: 0 }} />
              <span>{m.tabCleanupProtected()}</span>
            </div>
          </div>
        )

      case 'setup':
        return (
          <div className="preview-surface">
            <div className="setup-steps">
              <div className="setup-step" data-active={!setupPlanned}>{m.tabSetupStep1()}</div>
              <div className="setup-step" data-active={setupPlanned}>{m.tabSetupStep2()}</div>
              <div className="setup-step">{m.tabSetupStep3()}</div>
              <div className="setup-step">{m.tabSetupStep4()}</div>
            </div>

            <div className="setup-switch-group">
              {[
                { id: 'foundation', eyebrow: m.tabSetupGroup1Eyebrow(), items: m.tabSetupGroup1Items() },
                { id: 'languages', eyebrow: m.tabSetupGroup2Eyebrow(), items: m.tabSetupGroup2Items() },
                { id: 'infra', eyebrow: m.tabSetupGroup3Eyebrow(), items: m.tabSetupGroup3Items() },
                { id: 'ai', eyebrow: m.tabSetupGroup4Eyebrow(), items: m.tabSetupGroup4Items() },
              ].map((group) => {
                const on = Boolean(setupOn[group.id])
                return (
                  <div key={group.id} className="setup-switch-item">
                    <div>
                      <div style={{ fontSize: '0.64rem', fontWeight: 700, letterSpacing: '0.08em', textTransform: 'uppercase', color: 'var(--hearth-text-muted)' }}>
                        {group.eyebrow}
                      </div>
                      <div style={{ fontSize: '0.8rem', fontWeight: 600, marginTop: '0.18rem' }}>
                        {group.items}
                      </div>
                    </div>
                    <button
                      type="button"
                      role="switch"
                      aria-checked={on}
                      aria-label={group.eyebrow}
                      className="switch-track"
                      data-checked={on}
                      onClick={() => toggleSetup(group.id)}
                    >
                      <span className="switch-thumb" />
                    </button>
                  </div>
                )
              })}
            </div>

            <div className="preview-box">
              <div className="preview-row">
                <div className="mono" style={{ fontSize: '0.73rem', color: 'var(--hearth-text-secondary)' }}>
                  {m.tabSetupStepsReady({ count: String(setupStepCount) })} · Homebrew → mise → runtimes → Colima → AI
                </div>
                <button
                  type="button"
                  className={`preview-mini-btn ${setupPlanned ? 'preview-mini-btn-dark' : 'preview-mini-btn-accent'}`}
                  disabled={setupStepCount === 0}
                  onClick={() => setSetupPlanned((v) => !v)}
                >
                  <span>{setupPlanned ? m.tabSetupPlannedBtn() : m.tabSetupNextBtn()}</span>
                  {!setupPlanned ? <ArrowRight size={13} aria-hidden="true" /> : null}
                </button>
              </div>
            </div>
          </div>
        )
    }
  }

  const lightboxShot = `/screenshots/${SCREENSHOT_BY_TAB[lightboxTab]}-${theme}.webp`
  const lightboxTitle = tabs.find((t) => t.id === lightboxTab)?.label ?? ''

  return (
    <section id="features" className="workflows-section" data-reveal>
      <div className="container">
        <div className="section-header">
          <span className="eyebrow-label">{m.workflowsEyebrow()}</span>
          <h2 className="section-title">{m.workflowsTitle()}</h2>
          <p className="section-subtitle">{m.workflowsSubtitle()}</p>
        </div>

        <Tabs
          value={activeTab}
          onValueChange={(details) => setActiveTab(details.value as WorkflowTabId)}
        >
          <TabsList className="workflow-tabs-list">
            {tabs.map((tab) => (
              <TabsTrigger key={tab.id} value={tab.id} className="workflow-tab-trigger">
                {tab.label}
              </TabsTrigger>
            ))}
          </TabsList>

          {tabs.map((tab) => (
            <TabsContent key={tab.id} value={tab.id}>
              <div className="workflow-card">
                <div className="workflow-copy">
                  <span className="pill pill-accent">{tab.pill}</span>
                  <h3>{tab.title}</h3>
                  <p>{tab.desc}</p>
                  <ul className="workflow-bullets">
                    {tab.bullets.map((bullet, idx) => (
                      <li key={idx}>
                        <span className="check-dot" aria-hidden="true">
                          <Check size={12} strokeWidth={2.8} />
                        </span>
                        <span>{bullet}</span>
                      </li>
                    ))}
                  </ul>
                  <button
                    type="button"
                    className="workflow-screen-link"
                    onClick={() => {
                      setLightboxTab(tab.id)
                      setLightboxOpen(true)
                    }}
                  >
                    <ImageIcon size={14} aria-hidden="true" />
                    <span>{m.workflowsViewScreenshot()}</span>
                  </button>
                </div>

                {renderPreview(tab.id)}
              </div>
            </TabsContent>
          ))}
        </Tabs>
      </div>

      <Dialog open={lightboxOpen} onOpenChange={(details) => setLightboxOpen(details.open)}>
        <DialogBackdrop className="lightbox-backdrop" />
        <DialogPositioner className="lightbox-positioner">
          <DialogContent className="lightbox-content">
            <DialogHeader className="lightbox-header">
              <DialogTitle>{m.workflowsRealScreen()} · {lightboxTitle}</DialogTitle>
              <DialogCloseIcon />
            </DialogHeader>
            <DialogBody className="lightbox-body">
              <img src={lightboxShot} alt={lightboxTitle} />
            </DialogBody>
          </DialogContent>
        </DialogPositioner>
      </Dialog>
    </section>
  )
}
