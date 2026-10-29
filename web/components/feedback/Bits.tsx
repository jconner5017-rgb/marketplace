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
      {s.badges.map((b) => (
        <li key={b.key} title={b.hint} className="rounded-full bg-ship/10 px-2.5 py-0.5 font-semibold text-ship">✓ {b.label}</li>
      ))}
      {s.outside.map((o) => (
        <li key={o.platform} title={`Checked by our team on ${new Date(o.verified_at).toLocaleDateString("en-US")}`}
          className="rounded-full border border-line px-2.5 py-0.5">
          Verified {PLATFORM[o.platform] ?? o.platform}: <span className="num">{o.feedback_count.toLocaleString()} ratings, {o.positive_percent}%</span>
        </li>
      ))}
    </ul>
  );
}
