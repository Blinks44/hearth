import { BrandLogo } from '@/components/BrandLogo'
import * as m from '@/integrations/i18n/paraglide/messages'
import type { ColorScheme } from '@/lib/theme'

interface EcosystemStripProps {
  theme: ColorScheme
}

const STACK = [
  { name: 'nodedotjs', label: 'Node.js', hasDark: false },
  { name: 'python', label: 'Python', hasDark: false },
  { name: 'go', label: 'Go', hasDark: false },
  { name: 'rust', label: 'Rust', hasDark: true },
  { name: 'bun', label: 'Bun', hasDark: false },
  { name: 'docker', label: 'Docker', hasDark: false },
  { name: 'kubernetes', label: 'Kubernetes', hasDark: false },
  { name: 'postgresql', label: 'PostgreSQL', hasDark: false },
  { name: 'claude', label: 'Claude', hasDark: false },
  { name: 'openai', label: 'Codex', hasDark: true },
  { name: 'cursor', label: 'Cursor', hasDark: true },
  { name: 'git', label: 'Git', hasDark: false },
] as const

export function EcosystemStrip({ theme }: EcosystemStripProps) {
  return (
    <section className="ecosystem" data-reveal>
      <div className="container">
        <p className="ecosystem-label">{m.ecosystemLabel()}</p>
        <div className="ecosystem-grid">
          {STACK.map((item) => (
            <div key={item.name} className="ecosystem-item">
              <BrandLogo name={item.name} alt={item.label} theme={theme} hasDark={item.hasDark} size={22} />
              <span>{item.label}</span>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
