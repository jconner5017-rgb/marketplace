"use client";

import { useEffect, useState } from "react";

/** Key the visitor's choice is saved under (read by the script in app/layout.tsx). */
export const THEME_KEY = "sns-theme";

/** Sun/moon switch in the header: flips the whole site between light and dark and remembers it. */
export default function ThemeToggle() {
  const [dark, setDark] = useState<boolean | null>(null);

  useEffect(() => {
    setDark(document.documentElement.dataset.theme === "dark");
  }, []);

  function flip() {
    const next = !(document.documentElement.dataset.theme === "dark");
    document.documentElement.dataset.theme = next ? "dark" : "light";
    try { localStorage.setItem(THEME_KEY, next ? "dark" : "light"); } catch { /* private mode: still switches for this visit */ }
    setDark(next);
  }

  const label = dark ? "Switch to light mode" : "Switch to dark mode";
  return (
    <button type="button" onClick={flip} aria-label={label} title={label} className="flex items-center hover:text-link">
      {dark ? (
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
          <circle cx="12" cy="12" r="4" />
          <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
        </svg>
      ) : (
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" aria-hidden="true">
          <path d="M20 14.5A8 8 0 1 1 9.5 4a6.5 6.5 0 0 0 10.5 10.5Z" />
        </svg>
      )}
    </button>
  );
}
