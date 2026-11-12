"use client";

import { useEffect, useRef, useState } from "react";

/** Share a listing: the phone's own share sheet where there is one, otherwise
 *  a small menu (copy link, Facebook, X, Pinterest, email). */
export default function ShareButton({ url, title, image }: { url: string; title: string; image?: string }) {
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent) => { if (!box.current?.contains(e.target as Node)) setOpen(false); };
    const esc = (e: KeyboardEvent) => { if (e.key === "Escape") setOpen(false); };
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", esc);
    return () => { document.removeEventListener("mousedown", close); document.removeEventListener("keydown", esc); };
  }, [open]);

  async function share() {
    const nav = navigator as Navigator & { share?: (d: ShareData) => Promise<void>; canShare?: (d: ShareData) => boolean };
    // Phones and tablets: the system share sheet (Messages, Facebook, X, AirDrop...).
    if (nav.share && window.matchMedia("(pointer: coarse)").matches) {
      try { await nav.share({ title, url }); } catch { /* cancelled */ }
      return;
    }
    setOpen((o) => !o);
  }

  async function copy() {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch { /* clipboard blocked: the link is still in the address bar */ }
  }

  const u = encodeURIComponent(url);
  const t = encodeURIComponent(title);
  const links = [
    { label: "Facebook", href: `https://www.facebook.com/sharer/sharer.php?u=${u}` },
    { label: "X (Twitter)", href: `https://twitter.com/intent/tweet?url=${u}&text=${t}` },
    { label: "Pinterest", href: `https://pinterest.com/pin/create/button/?url=${u}&description=${t}${image ? `&media=${encodeURIComponent(image)}` : ""}` },
    { label: "Email", href: `mailto:?subject=${t}&body=${u}` },
  ];

  return (
    <div ref={box} className="relative inline-block">
      <button type="button" onClick={share} aria-haspopup="menu" aria-expanded={open}
        className="inline-flex items-center gap-2 rounded-full border border-line px-4 py-2 text-[14px] font-semibold hover:bg-wash">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="M4 12v7a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-7" /><path d="M16 6l-4-4-4 4" /><path d="M12 2v13" />
        </svg>
        Share
      </button>
      {open && (
        <div role="menu" className="absolute right-0 z-20 mt-2 w-56 rounded-lg border border-line bg-paper p-1.5 shadow-lg">
          <button type="button" role="menuitem" onClick={copy} className="block w-full rounded px-3 py-2 text-left text-[14px] hover:bg-wash">
            {copied ? "Link copied ✓" : "Copy link"}
          </button>
          {links.map((l) => (
            <a key={l.label} role="menuitem" href={l.href} target="_blank" rel="noopener noreferrer"
              className="block rounded px-3 py-2 text-[14px] hover:bg-wash" onClick={() => setOpen(false)}>
              {l.label}
            </a>
          ))}
        </div>
      )}
    </div>
  );
}
