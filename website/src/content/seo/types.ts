import type { SiteRoutePath } from "@/config/locales";

export type AlternativeSlug = "docker-desktop" | "lens-freelens" | "dbngin";
export type GuideSlug = "dev-environment-setup" | "ai-agents-mcp";

export interface MetricItem {
  value: string;
  label: string;
}

export interface ComparisonRow {
  criterion: string;
  hearth: string;
  other: string;
}

export interface TradeoffCard {
  pill: string;
  title: string;
  points: string[];
}

export interface StepItem {
  step: string;
  title: string;
  desc: string;
}

export interface AlternativePageContent {
  slug: AlternativeSlug;
  path: SiteRoutePath;
  crumbLabel: string;
  metaTitle: string;
  metaDescription: string;
  heroTitle: string;
  heroSubtitle: string;
  metrics: [MetricItem, MetricItem, MetricItem];
  hearthColumnLabel: string;
  otherColumnLabel: string;
  rows: ComparisonRow[];
  chooseHearth: TradeoffCard;
  keepOther: TradeoffCard;
  steps: [StepItem, StepItem, StepItem];
}

export interface PresetCard {
  pill: string;
  pillTone: "accent" | "info" | "success";
  title: string;
  desc: string;
  stackLines: string[];
  foot: string;
}

export interface PitfallCard {
  title: string;
  desc: string;
}

export interface GuidePageContent {
  slug: GuideSlug;
  path: SiteRoutePath;
  crumbLabel: string;
  metaTitle: string;
  metaDescription: string;
  heroTitle: string;
  heroSubtitle: string;
  secondaryCtaLabel: string;
  secondaryCtaTo: SiteRoutePath;
  presetsHeading: string;
  presets: [PresetCard, PresetCard, PresetCard];
  stepsHeading: string;
  steps: [StepItem, StepItem, StepItem, StepItem];
  pitfallsHeading: string;
  pitfalls: [PitfallCard, PitfallCard, PitfallCard];
}

export interface SeoContentBundle {
  alternatives: Record<AlternativeSlug, AlternativePageContent>;
  guides: Record<GuideSlug, GuidePageContent>;
}
