import { createFileRoute } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { BentoGrid } from '@/components/BentoGrid'
import { EcosystemStrip } from '@/components/EcosystemStrip'
import { FinalCta } from '@/components/FinalCta'
import { Footer } from '@/components/Footer'
import { Hero } from '@/components/Hero'
import { InstallSection } from '@/components/InstallSection'
import { McpSection } from '@/components/McpSection'
import { Navbar } from '@/components/Navbar'
import { NativeMetrics } from '@/components/NativeMetrics'
import { SafetySection } from '@/components/SafetySection'
import { WorkflowShowcase, type WorkflowTabId } from '@/components/WorkflowShowcase'
import { getLatestRelease } from '@/integrations/releases'
import { type DetectedPlatform, detectPlatform } from '@/lib/os'
import { useRevealObserver } from '@/lib/reveal'
import { useTheme } from '@/lib/theme'

export const Route = createFileRoute('/')({
  loader: () => getLatestRelease(),
  component: LandingPage,
})

function LandingPage() {
  const release = Route.useLoaderData()
  const { theme, toggleTheme } = useTheme()
  const [platform, setPlatform] = useState<DetectedPlatform>({ os: 'macos', arch: 'arm64' })
  const [workflowTab, setWorkflowTab] = useState<WorkflowTabId>('projects')

  useRevealObserver()

  useEffect(() => {
    setPlatform(detectPlatform())
  }, [])

  return (
    <>
      <Navbar theme={theme} onToggleTheme={toggleTheme} />
      <main>
        <Hero
          release={release}
          platform={platform}
          theme={theme}
          onToggleTheme={toggleTheme}
        />
        <EcosystemStrip theme={theme} />
        <WorkflowShowcase
          theme={theme}
          activeTab={workflowTab}
          onTabChange={setWorkflowTab}
        />
        <BentoGrid theme={theme} />
        <SafetySection />
        <McpSection />
        <NativeMetrics />
        <InstallSection release={release} platform={platform} />
        <FinalCta release={release} />
      </main>
      <Footer release={release} onSelectWorkflow={setWorkflowTab} />
    </>
  )
}
