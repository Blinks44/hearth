import type { Locale } from "@/config/locales";
import { deSeoContent } from "./de";
import { enSeoContent } from "./en";
import { esSeoContent } from "./es";
import { frSeoContent } from "./fr";
import { ruSeoContent } from "./ru";
import type {
  AlternativePageContent,
  AlternativeSlug,
  GuidePageContent,
  GuideSlug,
  SeoContentBundle,
} from "./types";
import { zhHansSeoContent } from "./zh-Hans";

const bundles: Record<Locale, SeoContentBundle> = {
  en: enSeoContent,
  ru: ruSeoContent,
  de: deSeoContent,
  es: esSeoContent,
  fr: frSeoContent,
  "zh-Hans": zhHansSeoContent,
};

export function getAlternativeContent(
  slug: AlternativeSlug,
  locale: Locale,
): AlternativePageContent {
  return bundles[locale].alternatives[slug];
}

export function getGuideContent(slug: GuideSlug, locale: Locale): GuidePageContent {
  return bundles[locale].guides[slug];
}

export type {
  AlternativePageContent,
  AlternativeSlug,
  ComparisonRow,
  GuidePageContent,
  GuideSlug,
  MetricItem,
  PitfallCard,
  PresetCard,
  SeoContentBundle,
  StepItem,
  TradeoffCard,
} from "./types";
