"use client";

import { usePathname } from "next/navigation";
import { useEffect } from "react";

const open = () => Array.from(document.querySelectorAll<HTMLDetailsElement>("details[data-menu][open]"));

/**
 * Header drop-downs are <details data-menu> so they work without JavaScript.
 * With it, they close when a link inside is chosen, on a click elsewhere,
 * on Escape, when another menu opens, and after every page change.
 */
export default function MenuCloser() {
  const path = usePathname();
  useEffect(() => {
    open().forEach((d) => (d.open = false));
  }, [path]);

  useEffect(() => {
    function click(e: MouseEvent) {
      const t = e.target as Element | null;
      const menu = t?.closest("details[data-menu]");
      for (const d of open()) {
        if (d !== menu) d.open = false; // clicked outside, or opened another menu
        else if (t?.closest("a, button[type=submit]") && !t.closest("summary")) d.open = false; // chose an item
      }
    }
    function key(e: KeyboardEvent) {
      if (e.key !== "Escape") return;
      const ds = open();
      ds.forEach((d) => (d.open = false));
      ds[0]?.querySelector<HTMLElement>("summary")?.focus();
    }
    document.addEventListener("click", click);
    document.addEventListener("keydown", key);
    return () => {
      document.removeEventListener("click", click);
      document.removeEventListener("keydown", key);
    };
  }, []);
  return null;
}
