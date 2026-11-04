import Link from "next/link";
import { Suspense } from "react";
import { getSummary } from "@/lib/api";
import { QUICK_LINKS, SITE_NAME, gameHref, nonsportHref, sportHref } from "@/lib/format";
import { getCatalog } from "@/lib/catalog";
import SignOutButton from "./SignOutButton";
import BellMenu from "./BellMenu";
import { BellIcon, CartIcon, ChevronIcon, HeartIcon } from "./Icons";
import SearchBar from "./SearchBar";
import MenuCloser from "./MenuCloser";
import ConfirmEmailBanner from "./ConfirmEmailBanner";

export default async function Header() {
  const [me, cat] = await Promise.all([getSummary(), getCatalog()]);
  const signIn = (next: string) => `/signin?next=${encodeURIComponent(next)}`;
  return (
    <header className="border-b border-line">
      <MenuCloser />
      {me?.must_confirm_email && <ConfirmEmailBanner email={me.user.email} />}
      {/* Utility row, eBay-style: greeting on the left, account shortcuts on the right */}
      <div className="border-b border-wash">
        <div className="mx-auto flex max-w-[1280px] items-center gap-5 px-4 py-1.5 text-[13px]">
          {me ? (
            <details data-menu className="group relative">
              <summary className="flex cursor-pointer list-none items-center gap-1 font-semibold hover:underline [&::-webkit-details-marker]:hidden">
                Hi {me.user.username}! <ChevronIcon />
              </summary>
              <div className="absolute left-0 top-full z-30 mt-2 w-60 rounded-lg border border-line bg-paper p-2 shadow-lg">
                <p className="px-3 py-2 text-[12px] text-muted">Signed in as {me.user.email}</p>
                <MenuLink href="/sell">{me.is_seller ? "Seller hub" : "Start selling"}</MenuLink>
                <MenuLink href="/purchases">Purchases</MenuLink>
                <MenuLink href="/offers">Offers</MenuLink>
                <MenuLink href="/watchlist">Watchlist</MenuLink>
                <MenuLink href="/saved-searches">Saved searches</MenuLink>
                <MenuLink href="/following">Followed stores</MenuLink>
                <MenuLink href={`/seller/${encodeURIComponent(me.user.username)}`}>My feedback</MenuLink>
                <MenuLink href="/account/standing">Account standing</MenuLink>
                <MenuLink href="/account/email">Email settings</MenuLink>
                <MenuLink href="/messages">Messages{me.unread_messages > 0 ? ` (${me.unread_messages})` : ""}</MenuLink>
                <MenuLink href="/cart">Cart</MenuLink>
                {(me.user.role === "admin" || me.user.staff_role) && <MenuLink href="/admin">Back office</MenuLink>}
                <div className="mt-1 border-t border-line px-3 pt-2"><SignOutButton /></div>
              </div>
            </details>
          ) : (
            <span>
              Hi! <Link href="/signin" className="text-link underline">Sign in</Link> or{" "}
              <Link href="/register" className="text-link underline">register</Link>
            </span>
          )}
          <Link href="/search?min_grade=1" className="hidden hover:underline sm:inline">Graded slabs</Link>
          <Link href="/search?max_price=5&sort=price_asc" className="hidden hover:underline sm:inline">Deals under $5</Link>

          <nav aria-label="Your account" className="ml-auto flex items-center gap-5">
            <Link href={me ? "/sell" : signIn("/sell")} className="hover:underline">Sell</Link>
            <Link href={me ? "/watchlist" : signIn("/watchlist")} className="flex items-center gap-1 hover:underline">
              <HeartIcon /> Watchlist{me && me.watching > 0 ? <span className="num text-muted">({me.watching})</span> : null}
            </Link>
            {me ? (
              <BellMenu unread={me.unread_notices} />
            ) : (
              <Link href={signIn("/messages")} aria-label="Notifications" className="hover:text-link"><BellIcon /></Link>
            )}
            <Link
              href={me ? "/cart" : signIn("/cart")}
              aria-label={me && me.cart_cards > 0 ? `Cart, ${me.cart_cards} cards` : "Cart"}
              className="relative hover:text-link"
            >
              <CartIcon />
              {me && me.cart_cards > 0 && (
                <span className="num absolute -right-2.5 -top-1.5 min-w-[18px] rounded-full bg-slab px-1 text-center text-[11px] font-bold leading-[18px] text-white">
                  {me.cart_cards > 99 ? "99+" : me.cart_cards}
                </span>
              )}
            </Link>
          </nav>
        </div>
      </div>

      {/* Logo + search */}
      <div className="mx-auto flex max-w-[1280px] flex-wrap items-center gap-x-5 gap-y-3 px-4 py-3">
        <Link href="/" className="flex items-center gap-2" aria-label={`${SITE_NAME} home`}>
          <Logo />
          <span aria-hidden="true" className="text-[25px] font-extrabold leading-none tracking-[-0.02em]">
            <span className="text-ink">Slabs</span>
            <span className="text-[#c9961f]">N</span>
            <span className="text-[#3f78c9]">Singl</span>
            <span className="text-[#c9961f]">es</span>
          </span>
        </Link>
        <Suspense fallback={<div className="min-w-[280px] flex-1" />}>
          <SearchBar />
        </Suspense>
      </div>

      {/* Category menus + shortcuts. <details> menus work without JavaScript. */}
      <nav aria-label="Categories" className="mx-auto max-w-[1280px] px-4">
        <ul className="flex flex-wrap items-center gap-x-5 gap-y-1 pb-2 text-[13px] text-muted">
          <li><Menu label="Sports cards" allHref="/sports" items={cat.sports.map((s) => ({ label: s.label, href: sportHref(s.value) }))} /></li>
          <li><Menu label="Trading card games" allHref="/tcg" items={cat.games.map((g) => ({ label: g.label, href: gameHref(g.value) }))} /></li>
          <li><Menu label="Non-sport cards" allHref="/nonsport" items={cat.nonsport.map((g) => ({ label: g.label, href: nonsportHref(g.value) }))} /></li>
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

/** Two cards fanned out: a gold-edged card behind, and in front a dark
 * card with a gold border holding a blue card (the SlabsNSingles mark). */
function Logo() {
  return (
    <svg width="40" height="40" viewBox="0 0 40 40" aria-hidden="true">
      <g transform="rotate(-16 12 19)">
        <rect x="3" y="4" width="19" height="27" rx="3" fill="#18202b" stroke="#c9961f" strokeWidth="2.2" />
      </g>
      <g transform="rotate(9 25 22)">
        <rect x="15" y="7" width="20" height="28" rx="3" fill="#18202b" stroke="#c9961f" strokeWidth="2.4" />
        <g transform="rotate(14 25 21)">
          <rect x="19.5" y="14" width="11" height="15" rx="1.6" fill="#3f78c9" stroke="#9cc0ee" strokeWidth="0.8" />
        </g>
      </g>
    </svg>
  );
}

function MenuLink({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <Link href={href} className="block rounded px-3 py-1.5 text-[14px] hover:bg-wash">
      {children}
    </Link>
  );
}

function Menu({ label, allHref, items }: { label: string; allHref: string; items: { label: string; href: string }[] }) {
  return (
    <details data-menu className="group relative">
      <summary className="flex cursor-pointer list-none items-center gap-1 font-semibold text-ink hover:underline [&::-webkit-details-marker]:hidden">
        {label}
        <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true" className="transition-transform group-open:rotate-180">
          <path d="M1.5 3.5 5 7l3.5-3.5" stroke="currentColor" strokeWidth="1.6" fill="none" />
        </svg>
      </summary>
      <div className="fixed inset-x-4 z-20 mt-2 sm:absolute sm:inset-x-auto sm:left-0 sm:top-full sm:w-[min(440px,90vw)] rounded-lg border border-line bg-paper p-4 shadow-lg">
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
