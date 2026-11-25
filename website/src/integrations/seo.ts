import {
  defaultLocale,
  type Locale,
  localePath,
  locales,
  openGraphLocales,
  siteRoutes,
} from "@/config/locales";
import { site } from "@/config/site";
import * as m from "@/integrations/i18n/paraglide/messages";

export interface FaqSchemaItem {
  question: string;
  answer: string;
}

export interface BreadcrumbSchemaItem {
  name: string;
  path: string;
}

export interface PageHeadOptions {
  path?: string;
  title?: string;
  description?: string;
  notFound?: boolean;
  includeSoftwareApp?: boolean;
  faq?: FaqSchemaItem[];
  breadcrumbs?: BreadcrumbSchemaItem[];
}

export function pageHead(locale: Locale, options: PageHeadOptions | boolean = {}) {
  const opts: PageHeadOptions = typeof options === "boolean" ? { notFound: options } : options;
  const notFound = Boolean(opts.notFound);
  const routePath = opts.path ?? "/";
  const title = notFound
    ? m.notFoundTitle({}, { locale })
    : (opts.title ?? m.metaTitle({}, { locale }));
  const description = notFound
    ? m.notFoundDescription({}, { locale })
    : (opts.description ?? m.metaDescription({}, { locale }));
  const url = `${site.origin}${localePath(locale, routePath)}`;
  const ogImage = `${site.origin}/og.png`;

  const scripts: Array<{ type: string; children: string }> = [];

  if (!notFound) {
    if (opts.includeSoftwareApp ?? routePath === "/") {
      scripts.push({
        type: "application/ld+json",
        children: JSON.stringify({
          "@context": "https://schema.org",
          "@type": "SoftwareApplication",
          name: site.name,
          url: `${site.origin}${localePath(locale, "/")}`,
          description: m.metaDescription({}, { locale }),
          inLanguage: locale,
          operatingSystem: "macOS, Windows, Linux",
          applicationCategory: "DeveloperApplication",
          offers: {
            "@type": "Offer",
            price: "0",
            priceCurrency: "USD",
          },
          license: "https://opensource.org/licenses/MIT",
          author: {
            "@type": "Person",
            name: site.author.name,
            sameAs: [site.author.x, "https://github.com/Blinks44"],
          },
        }),
      });
    }

    if (opts.faq && opts.faq.length > 0) {
      scripts.push({
        type: "application/ld+json",
        children: JSON.stringify({
          "@context": "https://schema.org",
          "@type": "FAQPage",
          inLanguage: locale,
          mainEntity: opts.faq.map((item) => ({
            "@type": "Question",
            name: item.question,
            acceptedAnswer: {
              "@type": "Answer",
              text: item.answer,
            },
          })),
        }),
      });
    }

    if (opts.breadcrumbs && opts.breadcrumbs.length > 0) {
      scripts.push({
        type: "application/ld+json",
        children: JSON.stringify({
          "@context": "https://schema.org",
          "@type": "BreadcrumbList",
          itemListElement: opts.breadcrumbs.map((crumb, index) => ({
            "@type": "ListItem",
            position: index + 1,
            name: crumb.name,
            item: `${site.origin}${localePath(locale, crumb.path)}`,
          })),
        }),
      });
    }
  }

  return {
    meta: [
      { title },
      { name: "description", content: description },
      {
        name: "robots",
        content: notFound || !site.indexingEnabled ? "noindex, follow" : "index, follow",
      },
      { property: "og:type", content: routePath === "/" ? "website" : "article" },
      { property: "og:site_name", content: site.name },
      { property: "og:title", content: title },
      { property: "og:description", content: description },
      { property: "og:locale", content: openGraphLocales[locale] },
      ...(!notFound ? [{ property: "og:url", content: url }] : []),
      { property: "og:image", content: ogImage },
      { property: "og:image:width", content: "1200" },
      { property: "og:image:height", content: "630" },
      {
        property: "og:image:alt",
        content: "Hearth · One calm home for your local dev environment",
      },
      { name: "twitter:card", content: "summary_large_image" },
      { name: "twitter:creator", content: site.author.xHandle },
      { name: "twitter:title", content: title },
      { name: "twitter:description", content: description },
      { name: "twitter:image", content: ogImage },
      {
        name: "twitter:image:alt",
        content: "Hearth · One calm home for your local dev environment",
      },
    ],
    links: notFound
      ? []
      : [
          { rel: "canonical", href: url },
          ...locales.map((value) => ({
            rel: "alternate",
            hrefLang: value,
            href: `${site.origin}${localePath(value, routePath)}`,
          })),
          {
            rel: "alternate",
            hrefLang: "x-default",
            href: `${site.origin}${localePath(defaultLocale, routePath)}`,
          },
        ],
    scripts,
  };
}

export function sitemap() {
  const urls = siteRoutes.flatMap((routePath) => {
    const alternates = locales
      .map(
        (locale) =>
          `<xhtml:link rel="alternate" hreflang="${locale}" href="${site.origin}${localePath(locale, routePath)}"/>`,
      )
      .join("");
    const xDefault = `<xhtml:link rel="alternate" hreflang="x-default" href="${site.origin}${localePath(defaultLocale, routePath)}"/>`;
    return locales.map(
      (locale) =>
        `<url><loc>${site.origin}${localePath(locale, routePath)}</loc>${alternates}${xDefault}</url>`,
    );
  });
  return `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">${urls.join("")}</urlset>`;
}
