import Link from "next/link";
import type { FeedbackSummary } from "@/lib/types";

const PLATFORM: Record<string, string> = { ebay: "eBay", etsy: "Etsy", discogs: "Discogs", tcgplayer: "TCGplayer", comc: "COMC", other: "Elsewhere" };

/** Five small stars for a 1–5 rating (or average). */
export function Stars({ value, label }: { value: number | null; label?: string }) {
  if (value == null) return <span className="text-muted">—</span>;
  const full = Math.round(value);
  return (
    <span className="whitespace-nowrap" aria-label={`${label ? label + ": " : ""}${value} out of 5`}>
      <span aria-hidden className="tracking-[1px] text-[#b7791f]">{"★".repeat(full)}<span className="text-line">{"★".repeat(5 - full)}</span></span>
      <span className="num ml-1 text-[13px] text-muted">{value.toFixed(1)}</span>
    </span>
  );
}

/** "jo_cards (142) 99.3% positive", linking to the profile. */
export function SellerLine({ s, link = true }: { s: FeedbackSummary; link?: boolean }) {
  const name = link
    ? <Link href={`/seller/${encodeURIComponent(s.username)}`} className="font-semibold text-link hover:underline">{s.username}</Link>
    : <span className="font-semibold">{s.username}</span>;
  return (
    <span>
      {name} <span className="num text-muted">({s.score})</span>{" "}
      {s.positive_percent != null ? <span className="num">{s.positive_percent.toFixed(1)}% positive</span> : <span className="text-muted">No feedback yet</span>}
    </span>
  );
}

export function BadgeList({ s }: { s: FeedbackSummary }) {
  if (s.badges.length === 0 && s.outside.length === 0) return null;
  return (
    <ul className="mt-2 flex flex-wrap gap-2 text-[12px]">
      {s.badges.map((b) => <li key={b.key}><SellerBadge b={b} /></li>)}
      {s.outside.map((o) => (
        <li key={o.platform} title={`Checked by our team on ${new Date(o.verified_at).toLocaleDateString("en-US")}`}
          className="rounded-full border border-line px-2.5 py-0.5">
          Verified {PLATFORM[o.platform] ?? o.platform}: <span className="num">{o.feedback_count.toLocaleString()} ratings, {o.positive_percent}%</span>
        </li>
      ))}
    </ul>
  );
}

/** Colors for the sales badge ladder (the medal dot only; text stays ink). */
export const BADGE_COLOR: Record<string, string> = {
  bronze: "#b06a3b", silver: "#8a94a6", gold: "#c9971c", platinum: "#5b8a9a", diamond: "#3f7fe0",
};

/** One seller badge: a medal for sales milestones, a star for Top Rated, a check otherwise. */
export function SellerBadge({ b, size = "sm" }: { b: { key: string; label: string; hint: string }; size?: "sm" | "lg" }) {
  const big = size === "lg";
  const color = BADGE_COLOR[b.key];
  const pad = big ? "px-3 py-1 text-[14px]" : "px-2.5 py-0.5 text-[12px]";
  if (color) {
    return (
      <span title={b.hint} className={`inline-flex items-center gap-1.5 rounded-full border border-line bg-white font-semibold text-ink ${pad}`}>
        <span aria-hidden className={`inline-block rounded-full ${big ? "h-3.5 w-3.5" : "h-2.5 w-2.5"}`}
          style={{ background: color, boxShadow: `0 0 0 2px #fff, 0 0 0 3px ${color}` }} />
        {b.label}
      </span>
    );
  }
  if (b.key === "top_rated") {
    return (
      <span title={b.hint} className={`inline-flex items-center gap-1 rounded-full bg-ink font-semibold text-white ${pad}`}>
        <span aria-hidden>★</span> {b.label}
      </span>
    );
  }
  return <span title={b.hint} className={`inline-block rounded-full bg-ship/10 font-semibold text-ship ${pad}`}>✓ {b.label}</span>;
}
