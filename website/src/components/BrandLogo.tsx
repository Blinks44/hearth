import type { ColorScheme } from "@/lib/theme";

interface BrandLogoProps {
  name: string;
  alt: string;
  theme: ColorScheme;
  hasDark?: boolean;
  size?: number;
}

export function BrandLogo({ name, alt, theme, hasDark = false, size = 20 }: BrandLogoProps) {
  const src = theme === "dark" && hasDark ? `/logos/${name}-dark.svg` : `/logos/${name}.svg`;
  return <img src={src} alt={alt} width={size} height={size} decoding="async" />;
}
