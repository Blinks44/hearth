import { describe, expect, it } from "vitest";
import { localePath, locales, siteRoutes } from "@/config/locales";
import {
  type AlternativeSlug,
  type GuideSlug,
  getAlternativeContent,
  getGuideContent,
} from "@/content/seo";
import { pageHead, sitemap } from "@/integrations/seo";

describe("SEO helpers & localized content", () => {
  it("builds trailing-slash locale paths for root and subpages", () => {
    expect(localePath("en")).toBe("/");
    expect(localePath("en", "/")).toBe("/");
    expect(localePath("en", "/alternatives/docker-desktop")).toBe("/alternatives/docker-desktop/");
    expect(localePath("ru")).toBe("/ru/");
    expect(localePath("ru", "/alternatives/docker-desktop")).toBe(
      "/ru/alternatives/docker-desktop/",
    );
    expect(localePath("zh-Hans", "/guides/ai-agents-mcp/")).toBe("/zh-Hans/guides/ai-agents-mcp/");
  });

  it("includes every route across every locale with hreflang alternates in sitemap", () => {
    const xml = sitemap();
    const urlCount = (xml.match(/<url>/g) ?? []).length;
    expect(urlCount).toBe(siteRoutes.length * locales.length);

    for (const route of siteRoutes) {
      for (const locale of locales) {
        const expectedLoc = `https://hearth.moduix.dev${localePath(locale, route)}`;
        expect(xml).toContain(`<loc>${expectedLoc}</loc>`);
        expect(xml).toContain(`hreflang="${locale}" href="${expectedLoc}"`);
      }
      const xDefault = `https://hearth.moduix.dev${localePath("en", route)}`;
      expect(xml).toContain(`hreflang="x-default" href="${xDefault}"`);
    }
  });

  it("emits SoftwareApplication and FAQPage JSON-LD for landing head", () => {
    const head = pageHead("en", {
      path: "/",
      includeSoftwareApp: true,
      faq: [{ question: "Is Hearth free?", answer: "Yes, MIT open source." }],
    });

    expect(head.links.find((l) => l.rel === "canonical")?.href).toBe("https://hearth.moduix.dev/");
    const schemas = head.scripts.map((s) => JSON.parse(s.children) as { "@type": string });
    expect(schemas.map((s) => s["@type"])).toEqual(["SoftwareApplication", "FAQPage"]);
  });

  it("emits route canonical and BreadcrumbList JSON-LD for subpages and noindex for 404", () => {
    const altHead = pageHead("ru", {
      path: "/alternatives/docker-desktop",
      title: "Docker Alt",
      description: "Desc",
      breadcrumbs: [
        { name: "Hearth", path: "/" },
        { name: "Docker Desktop", path: "/alternatives/docker-desktop" },
      ],
    });
    expect(altHead.links.find((l) => l.rel === "canonical")?.href).toBe(
      "https://hearth.moduix.dev/ru/alternatives/docker-desktop/",
    );
    const schemas = altHead.scripts.map((s) => JSON.parse(s.children) as { "@type": string });
    expect(schemas.map((s) => s["@type"])).toEqual(["BreadcrumbList"]);

    const notFoundHead = pageHead("en", { notFound: true });
    expect(notFoundHead.links).toEqual([]);
    expect(notFoundHead.scripts).toEqual([]);
    expect(notFoundHead.meta.find((m) => "name" in m && m.name === "robots")?.content).toBe(
      "noindex, follow",
    );
  });

  it("provides complete localized content for every SEO page across all 6 locales", () => {
    const altSlugs: AlternativeSlug[] = ["docker-desktop", "lens-freelens", "dbngin"];
    const guideSlugs: GuideSlug[] = ["dev-environment-setup", "ai-agents-mcp"];

    for (const locale of locales) {
      for (const slug of altSlugs) {
        const page = getAlternativeContent(slug, locale);
        expect(page.metaTitle.length).toBeGreaterThan(10);
        expect(page.metaDescription.length).toBeGreaterThan(30);
        expect(page.rows.length).toBe(6);
        expect(page.steps.length).toBe(3);
      }
      for (const slug of guideSlugs) {
        const page = getGuideContent(slug, locale);
        expect(page.metaTitle.length).toBeGreaterThan(10);
        expect(page.metaDescription.length).toBeGreaterThan(30);
        expect(page.presets.length).toBe(3);
        expect(page.steps.length).toBe(4);
        expect(page.pitfalls.length).toBe(3);
      }
    }
  });
});
