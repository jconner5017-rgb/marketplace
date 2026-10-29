import Link from "next/link";
import HeartButton from "./HeartButton";
import type { Listing } from "@/lib/types";
import { CONDITIONS, gradeLabel, money, timeLeft } from "@/lib/format";

/** "3 bids · 2d 4h left" for auctions; "Buy It Now" or "or Best Offer" otherwise. */
export function SaleLine({ l, className = "" }: { l: Listing; className?: string }) {
  if (l.format === "auction" && l.auction) {
    const a = l.auction;
    return (
      <p className={className}>
        {a.bid_count} {a.bid_count === 1 ? "bid" : "bids"} · <span className="num">{timeLeft(a.ends_at)}</span>
        {a.buy_now_cents != null && <span className="text-muted"> · or Buy It Now {money(a.buy_now_cents)}</span>}
      </p>
    );
  }
  return <p className={className}>{l.offers_enabled ? "Buy It Now or Best Offer" : "Buy It Now"}</p>;
}

/**
 * Graded cards: a tiny grading-slab label (red band, white body).
 * Raw cards: a plain outlined condition tag.
 */
export function GradeBadge({ l, size = "sm" }: { l: Listing; size?: "sm" | "lg" }) {
  const big = size === "lg";
  if (l.grader) {
    return (
      <span
        className={`inline-flex flex-col overflow-hidden rounded-[3px] border border-slab bg-white leading-none ${big ? "min-w-16" : "min-w-11"}`}
        title={`Graded ${l.grader} ${gradeLabel(l.grade)}`}
      >
        <span className={`bg-slab text-center font-bold text-white ${big ? "px-2 py-1 text-xs" : "px-1.5 py-[3px] text-[10px]"}`}>
          {l.grader}
        </span>
        <span className={`num text-center font-extrabold ${big ? "px-2 py-1 text-xl" : "px-1.5 py-[3px] text-sm"}`}>
          {gradeLabel(l.grade)}
        </span>
      </span>
    );
  }
  const code = l.condition ?? "";
  return (
    <span
      className={`inline-block rounded-[3px] border border-line text-muted ${big ? "px-2 py-1 text-sm" : "px-1.5 py-0.5 text-[11px]"}`}
      title={CONDITIONS[code] ?? code}
    >
      {big ? `Raw · ${CONDITIONS[code] ?? code}` : code}
    </span>
  );
}

export function Photo({ l, className = "" }: { l: Listing; className?: string }) {
  const src = l.photo_urls[0];
  return (
    <div className={`flex items-center justify-center overflow-hidden bg-wash ${className}`}>
      {src ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img src={src} alt={l.title} loading="lazy" className="h-full w-full object-contain" />
      ) : (
        <span className="text-xs text-muted">No photo</span>
      )}
    </div>
  );
}

function subtitle(l: Listing): string {
  return [l.year, l.set_name, l.card_number ? `#${l.card_number}` : null].filter(Boolean).join(" ");
}

/** Grid tile for browse rows and the grid view. */
export function ListingTile({ l }: { l: Listing }) {
  return (
    <div className="relative">
    <Link href={`/item/${l.id}`} className="group flex flex-col">
      <Photo l={l} className="aspect-[5/7] rounded-md" />
      <div className="mt-2 flex items-start gap-2">
        <h3 className="title-tight line-clamp-2 flex-1 text-[14px] leading-snug group-hover:underline">{l.title}</h3>
        <GradeBadge l={l} />
      </div>
      <p className="mt-0.5 line-clamp-1 text-[12px] text-muted">{subtitle(l)}</p>
      <p className="num mt-1 text-[18px] font-bold">{money(l.price_cents)}</p>
      <SaleLine l={l} className="text-[12px] text-muted" />
      <p className="text-[12px] text-ship">Free shipping</p>
    </Link>
    <HeartButton id={l.id} title={l.title} className="absolute right-2 top-2" />
    </div>
  );
}

/** Row for the list view — the eBay search-results layout. */
export function ListingRow({ l }: { l: Listing }) {
  return (
    <li className="flex gap-4 border-b border-line py-4">
      <div className="relative shrink-0">
        <Link href={`/item/${l.id}`}>
          <Photo l={l} className="h-[168px] w-[120px] rounded-md sm:h-[196px] sm:w-[140px]" />
        </Link>
        <HeartButton id={l.id} title={l.title} className="absolute right-1.5 top-1.5" />
      </div>
      <div className="min-w-0 flex-1">
        <Link href={`/item/${l.id}`} className="hover:underline">
          <h3 className="title-tight text-[17px] leading-snug">{l.title}</h3>
        </Link>
        <p className="mt-1 text-[13px] text-muted">
          {subtitle(l)} · {l.game_or_sport}
        </p>
        <div className="mt-2 flex items-center gap-2">
          <GradeBadge l={l} />
          {l.grader ? (
            <span className="text-[13px] text-muted">Graded</span>
          ) : (
            <span className="text-[13px] text-muted">{CONDITIONS[l.condition ?? ""] ?? "Raw"}</span>
          )}
        </div>
        <p className="num mt-3 text-[22px] font-bold leading-none">{money(l.price_cents)}</p>
        <SaleLine l={l} className="mt-1 text-[13px]" />
        <p className="text-[13px] text-ship">Free shipping</p>
        <p className="mt-2 text-[12px] text-muted">
          {l.quantity > 1 ? `${l.quantity} available · ` : ""}Seller: {l.seller_username}
        </p>
      </div>
    </li>
  );
}
