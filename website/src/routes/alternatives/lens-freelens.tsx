import { createFileRoute } from "@tanstack/react-router";
import { AlternativePage } from "@/components/AlternativePage";
import type { Locale } from "@/config/locales";
import { getAlternativeContent } from "@/content/seo";
import * as m from "@/integrations/i18n/paraglide/messages";
import { getLocale } from "@/integrations/i18n/paraglide/runtime";
import { getLatestRelease } from "@/integrations/releases";
import { pageHead } from "@/integrations/seo";

export const Route = createFileRoute("/alternatives/lens-freelens")({
  loader: () => getLatestRelease(),
  head: () => {
    const locale = getLocale() as Locale;
    const content = getAlternativeContent("lens-freelens", locale);
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
  component: LensFreelensAlternativeRoute,
});

function LensFreelensAlternativeRoute() {
  const release = Route.useLoaderData();
  const locale = getLocale() as Locale;
  return (
    <AlternativePage content={getAlternativeContent("lens-freelens", locale)} release={release} />
  );
}
