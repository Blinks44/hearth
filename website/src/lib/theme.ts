import { useCallback, useEffect, useState } from "react";

export type ColorScheme = "light" | "dark";

const STORAGE_KEY = "hearth-theme";

export const themeInitScript = `(() => {
  try {
    const param = new URLSearchParams(window.location.search).get('theme');
    const saved = localStorage.getItem('${STORAGE_KEY}');
    const preferred = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    const scheme =
      param === 'dark' || param === 'light'
        ? param
        : saved === 'dark' || saved === 'light'
          ? saved
          : preferred;
    document.documentElement.dataset.moduixColorScheme = scheme;
    document.documentElement.dataset.theme = scheme;
    document.documentElement.style.colorScheme = scheme;
  } catch {}
})();`;

function readDomTheme(): ColorScheme {
  if (typeof document === "undefined") return "light";
  return document.documentElement.dataset.theme === "dark" ? "dark" : "light";
}

function applyTheme(scheme: ColorScheme): void {
  if (typeof document === "undefined") return;
  document.documentElement.dataset.moduixColorScheme = scheme;
  document.documentElement.dataset.theme = scheme;
  document.documentElement.style.colorScheme = scheme;
}

export function useTheme() {
  const [theme, setThemeState] = useState<ColorScheme>("light");

  useEffect(() => {
    const current = readDomTheme();
    setThemeState(current);
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const onSystemChange = (event: MediaQueryListEvent) => {
      try {
        if (!localStorage.getItem(STORAGE_KEY)) {
          const next: ColorScheme = event.matches ? "dark" : "light";
          applyTheme(next);
          setThemeState(next);
        }
      } catch {
        // Ignore storage errors in private mode.
      }
    };
    media.addEventListener("change", onSystemChange);
    return () => media.removeEventListener("change", onSystemChange);
  }, []);

  const toggleTheme = useCallback(() => {
    setThemeState((prev) => {
      const next: ColorScheme = prev === "dark" ? "light" : "dark";
      applyTheme(next);
      try {
        localStorage.setItem(STORAGE_KEY, next);
      } catch {
        // Ignore storage errors in private mode.
      }
      return next;
    });
  }, []);

  return { theme, toggleTheme };
}
