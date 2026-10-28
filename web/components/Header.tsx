import Link from "next/link";
import { Suspense } from "react";
import { getMe } from "@/lib/api";
import { GAMES, QUICK_LINKS, SITE_NAME, SPORTS, gameHref, sportHref } from "@/lib/format";
import SignOutButton from "./SignOutButton";
import SearchBar from "./SearchBar";

export default async function Header() {
  const me = await getMe();
  return (
    <header className="border-b border-line">
      {/* Utility row, like eBay's top strip */}
      <div className="mx-auto flex max-w-[1280px] items-center gap-4 px-4 pt-2 text-[13px]">
        {me ? (
          <span>
            Hi <strong>{me.username}</strong> <SignOutButton />
          </span>
        ) : (
          <span>
            Hi! <Link href="/signin" className="text-link underline">Sign in</Link> or{" "}
            <Link href="/register" className="text-link underline">register</Link>
          </span>
        )}
        <span className="ml-auto flex gap-4">
          <Link href="/sell" className="hover:underline">Sell</Link>
          {me && <Link href="/sell" className="hover:underline">My listings</Link>}
        </span>
      </div>

      {/* Logo + search */}
      <div className="mx-auto flex max-w-[1280px] flex-wrap items-center gap-x-5 gap-y-3 px-4 py-3">
        <Link href="/" className="flex items-center gap-2" aria-label={`${SITE_NAME} home`}>
          <Logo />
          <span className="text-[22px] font-extrabold leading-none tracking-tight" style={{ fontStretch: "92%" }}>
            {SITE_NAME}
          </span>
        </Link>
        <Suspense fallback={<div className="min-w-[280px] flex-1" />}>
          <SearchBar />
        </Suspense>
      </div>

      {/* Category menus + shortcuts. <details> menus work without JavaScript. */}
      <nav aria-label="Categories" className="mx-auto max-w-[1280px] px-4">
        <ul className="flex flex-wrap items-center gap-x-5 gap-y-1 pb-2 text-[13px] text-muted">
          <li><Menu label="Sports cards" allHref="/search?category=sports" items={SPORTS.map((s) => ({ label: s.label, href: sportHref(s.value) }))} /></li>
          <li><Menu label="Trading card games" allHref="/search?category=tcg" items={GAMES.map((g) => ({ label: g.label, href: gameHref(g.value) }))} /></li>
          {QUICK_LINKS.map((c) => (
            <li key={c.label}>
              <Link href={c.href} className="hover:text-ink hover:underline">{c.label}</Link>
            </li>
          ))}
        </ul>
      </nav>
    </header>
  );
}

/** A graded slab seen from the front: label band on top, card window below. */
function Logo() {
  return (
    <svg width="26" height="34" viewBox="0 0 26 34" aria-hidden="true">
      <rect x="1" y="1" width="24" height="32" rx="3" fill="#fff" stroke="#18202b" strokeWidth="2" />
      <rect x="1" y="1" width="24" height="8" rx="3" fill="#c8102e" />
      <rect x="1" y="6" width="24" height="3" fill="#c8102e" />
      <rect x="5" y="12" width="16" height="17" rx="1.5" fill="#1a4fd6" />
    </svg>
  );
}

function Menu({ label, allHref, items }: { label: string; allHref: string; items: { label: string; href: string }[] }) {
  return (
    <details className="group relative">
      <summary className="flex cursor-pointer list-none items-center gap-1 font-semibold text-ink hover:underline [&::-webkit-details-marker]:hidden">
        {label}
        <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true" className="transition-transform group-open:rotate-180">
          <path d="M1.5 3.5 5 7l3.5-3.5" stroke="currentColor" strokeWidth="1.6" fill="none" />
        </svg>
      </summary>
      <div className="absolute left-0 top-full z-20 mt-2 w-[min(440px,90vw)] rounded-lg border border-line bg-paper p-4 shadow-lg">
        <ul className="grid grid-cols-2 gap-x-6 gap-y-1.5 text-[14px]">
          {items.map((i) => (
            <li key={i.href}><Link href={i.href} className="text-ink hover:text-link hover:underline">{i.label}</Link></li>
          ))}
        </ul>
        <Link href={allHref} className="mt-3 block border-t border-line pt-2 text-[14px] font-semibold text-link hover:underline">
          Shop all {label.toLowerCase()}
        </Link>
      </div>
    </details>
  );
}
