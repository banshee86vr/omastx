export type Theme = "night" | "day";

const STORAGE_KEY = "omastx-theme";

export function getStoredTheme(): Theme {
  if (typeof document === "undefined") {
    return "night";
  }
  const stored = localStorage.getItem(STORAGE_KEY);
  return stored === "day" ? "day" : "night";
}

export function applyTheme(theme: Theme): void {
  document.documentElement.dataset.theme = theme;
  localStorage.setItem(STORAGE_KEY, theme);
  window.dispatchEvent(new CustomEvent("omastx-theme-change", { detail: theme }));
}

export function initTheme(): Theme {
  const theme = getStoredTheme();
  applyTheme(theme);
  return theme;
}

export function toggleTheme(current: Theme): Theme {
  const next: Theme = current === "night" ? "day" : "night";
  applyTheme(next);
  return next;
}
