import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { BentoGrid } from "@/components/BentoGrid";
import { EcosystemStrip } from "@/components/EcosystemStrip";
import { FaqSection, getLandingFaqItems } from "@/components/FaqSection";
import { FinalCta } from "@/components/FinalCta";
import { Footer } from "@/components/Footer";
import { Hero } from "@/components/Hero";
import { InstallSection } from "@/components/InstallSection";
import { McpSection } from "@/components/McpSection";
import { Navbar } from "@/components/Navbar";
import { NativeMetrics } from "@/components/NativeMetrics";
import { ReplacesSection } from "@/components/ReplacesSection";
import { SafetySection } from "@/components/SafetySection";
import { WhoItIsForSection } from "@/components/WhoItIsForSection";
import { WorkflowShowcase, type WorkflowTabId } from "@/components/WorkflowShowcase";
import type { Locale } from "@/config/locales";
import { getLocale } from "@/integrations/i18n/paraglide/runtime";
import { getLatestRelease } from "@/integrations/releases";
import { pageHead } from "@/integrations/seo";
import { type DetectedPlatform, detectPlatform } from "@/lib/os";
import { useRevealObserver } from "@/lib/reveal";
import { useTheme } from "@/lib/theme";

export const Route = createFileRoute("/")({
  loader: () => getLatestRelease(),
  head: () => {
    const locale = getLocale() as Locale;
    return pageHead(locale, {
      path: "/",
      includeSoftwareApp: true,
      faq: getLandingFaqItems(locale),
    });
  },
  component: LandingPage,
});

function LandingPage() {
  const release = Route.useLoaderData();
  const { theme, toggleTheme } = useTheme();
  const [platform, setPlatform] = useState<DetectedPlatform>({ os: "macos", arch: "arm64" });
  const [workflowTab, setWorkflowTab] = useState<WorkflowTabId>("projects");

  useRevealObserver();

  useEffect(() => {
    setPlatform(detectPlatform());
  }, []);

  return (
    <>
      <Navbar theme={theme} onToggleTheme={toggleTheme} />
      <main>
        <Hero release={release} platform={platform} theme={theme} onToggleTheme={toggleTheme} />
        <EcosystemStrip theme={theme} />
        <WhoItIsForSection />
        <WorkflowShowcase theme={theme} activeTab={workflowTab} onTabChange={setWorkflowTab} />
        <BentoGrid theme={theme} />
        <ReplacesSection onSelectWorkflow={setWorkflowTab} />
        <SafetySection />
        <McpSection />
        <NativeMetrics />
        <InstallSection release={release} platform={platform} />
        <FaqSection />
        <FinalCta release={release} />
      </main>
      <Footer release={release} onSelectWorkflow={setWorkflowTab} />
    </>
  );
}
