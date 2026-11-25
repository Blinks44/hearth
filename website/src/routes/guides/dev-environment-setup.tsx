import { createFileRoute } from "@tanstack/react-router";
import { GuidePage } from "@/components/GuidePage";
import type { Locale } from "@/config/locales";
import { getGuideContent } from "@/content/seo";
import * as m from "@/integrations/i18n/paraglide/messages";
import { getLocale } from "@/integrations/i18n/paraglide/runtime";
import { getLatestRelease } from "@/integrations/releases";
import { pageHead } from "@/integrations/seo";

export const Route = createFileRoute("/guides/dev-environment-setup")({
  loader: () => getLatestRelease(),
  head: () => {
    const locale = getLocale() as Locale;
    const content = getGuideContent("dev-environment-setup", locale);
    return pageHead(locale, {
      path: content.path,
      title: content.metaTitle,
      description: content.metaDescription,
      breadcrumbs: [
        { name: m.seoCrumbHome({}, { locale }), path: "/" },
        { name: content.crumbLabel, path: content.path },
      ],
    });
  },
  component: DevEnvironmentSetupGuideRoute,
});

function DevEnvironmentSetupGuideRoute() {
  const release = Route.useLoaderData();
  const locale = getLocale() as Locale;
  return <GuidePage content={getGuideContent("dev-environment-setup", locale)} release={release} />;
}
