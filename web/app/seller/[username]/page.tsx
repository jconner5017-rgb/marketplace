import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGet } from "@/lib/api";
import { SITE_URL } from "@/lib/seo";
import type { FeedbackEntry, FeedbackSummary } from "@/lib/types";
import { BadgeList, Stars } from "@/components/feedback/Bits";
import ReplyForm from "@/components/feedback/ReplyForm";

export const dynamic = "force-dynamic";

type Props = { params: Promise<{ username: string }>; searchParams: Promise<{ filter?: string; as?: string }> };
type Data = { summary: FeedbackSummary; feedback: FeedbackEntry[] };

async function load(username: string, qs: string) {
  return (await apiGet<Data>(`/v1/members/${encodeURIComponent(username)}/feedback?${qs}`)).data;
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { username } = await params;
  const d = await load(username, "role=seller&limit=1");
  if (!d) return { title: "Seller not found" };
  const s = d.summary;
  const pct = s.positive_percent != null ? `${s.positive_percent.toFixed(1)}% positive` : "new seller";
  return {
    title: `${s.username}: seller feedback`,
    description: `${s.username} on Slabs & Singles: feedback score ${s.score}, ${pct} over the last 12 months, ${s.sales} sales.`,
    alternates: { canonical: `${SITE_URL}/seller/${encodeURIComponent(s.username)}` },
  };
}

const FILTERS = [["", "All"], ["positive", "Positive"], ["neutral", "Neutral"], ["negative", "Negative"]] as const;
const RATING: Record<number, [string, string, string]> = { 1: ["Positive", "+", "text-ship"], 0: ["Neutral", "○", "text-muted"], [-1]: ["Negative", "−", "text-slab"] };

export default async function SellerProfile({ params, searchParams }: Props) {
  const { username } = await params;
  const sp = await searchParams;
  const role = sp.as === "buyer" ? "buyer" : "seller";
  const filter = FILTERS.some(([v]) => v === sp.filter) ? sp.filter ?? "" : "";
  const d = await load(username, new URLSearchParams({ role, filter, limit: "100" }).toString());
  if (!d) notFound();
  const s = d.summary;
  const since = new Date(s.member_since).toLocaleDateString("en-US", { month: "long", year: "numeric" });
  const href = (f: string, as = role) => `/seller/${encodeURIComponent(s.username)}?${new URLSearchParams({ ...(f ? { filter: f } : {}), ...(as === "buyer" ? { as } : {}) })}`;

  return (
    <div className="max-w-[900px] pt-6">
      <div className="flex flex-wrap items-baseline justify-between gap-3">
        <div>
          <h1 className="text-[26px] font-bold">{s.username}</h1>
          <p className="text-muted">Member since {since}{role === "seller" ? ` · ${s.sales} sales` : ""}</p>
          {role === "seller" && <BadgeList s={s} />}
        </div>
        {role === "seller" && (
          <Link href={`/search?seller=${encodeURIComponent(s.username)}`} className="rounded-full bg-link px-5 py-2 font-semibold text-white hover:bg-link-dark">See their cards</Link>
        )}
      </div>

      <section className="mt-6 grid gap-4 md:grid-cols-3">
        <div className="rounded-lg border border-line p-4">
          <p className="text-[13px] text-muted">Feedback score</p>
          <p className="num text-[30px] font-bold leading-tight">{s.score}</p>
          <p className="text-[12px] text-muted">Positive minus negative, all time</p>
        </div>
        <div className="rounded-lg border border-line p-4">
          <p className="text-[13px] text-muted">Positive, last 12 months</p>
          <p className="num text-[30px] font-bold leading-tight">{s.positive_percent != null ? `${s.positive_percent.toFixed(1)}%` : "—"}</p>
          <p className="num text-[12px] text-muted">
            <span className="text-ship">{s.positive_12m} positive</span> · {s.neutral_12m} neutral · <span className="text-slab">{s.negative_12m} negative</span>
          </p>
        </div>
        {role === "seller" && (
          <div className="rounded-lg border border-line p-4 text-[14px]">
            <p className="text-[13px] text-muted">Ratings, last 12 months</p>
            <dl className="mt-1 space-y-1">
              <div className="flex justify-between gap-2"><dt>Item as described</dt><dd><Stars value={s.item_avg} label="Item as described" /></dd></div>
              <div className="flex justify-between gap-2"><dt>Shipping</dt><dd><Stars value={s.shipping_avg} label="Shipping" /></dd></div>
              <div className="flex justify-between gap-2"><dt>Communication</dt><dd><Stars value={s.communication_avg} label="Communication" /></dd></div>
            </dl>
          </div>
        )}
      </section>
      {s.under_review > 0 && (
        <p className="mt-3 text-[13px] text-muted">{s.under_review === 1 ? "One rating is" : `${s.under_review} ratings are`} being reviewed by our support team and isn&apos;t counted yet.</p>
      )}

      <section className="mt-8">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line pb-2">
          <nav aria-label="Feedback as" className="flex gap-1 text-[14px]">
            <Link href={href(filter, "seller")} aria-current={role === "seller" ? "page" : undefined}
              className={`border-b-2 px-2 py-1 font-semibold ${role === "seller" ? "border-ink" : "border-transparent text-muted"}`}>As a seller</Link>
            <Link href={href(filter, "buyer")} aria-current={role === "buyer" ? "page" : undefined}
              className={`border-b-2 px-2 py-1 font-semibold ${role === "buyer" ? "border-ink" : "border-transparent text-muted"}`}>As a buyer</Link>
          </nav>
          <nav aria-label="Filter feedback" className="flex flex-wrap gap-1 text-[13px]">
            {FILTERS.map(([v, l]) => (
              <Link key={v || "all"} href={href(v)} className={`rounded-full border px-3 py-0.5 ${filter === v ? "border-ink bg-ink text-white" : "border-line hover:border-ink"}`}>{l}</Link>
            ))}
          </nav>
        </div>
        {d.feedback.length === 0 ? (
          <p className="py-10 text-center text-muted">No feedback here yet.</p>
        ) : (
          <ul>
            {d.feedback.map((f) => {
              const [label, sym, tone] = RATING[f.rating];
              return (
                <li key={f.id} className="border-b border-wash py-4 text-[14px]">
                  <div className="flex gap-3">
                    <span aria-label={label} title={label} className={`mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full border text-[15px] font-bold ${tone}`}>{sym}</span>
                    <div className="min-w-0 flex-1">
                      <p>{f.comment || <span className="text-muted">{label}</span>}</p>
                      <p className="mt-0.5 text-[12px] text-muted">
                        {f.from} · {new Date(f.created_at).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" })}
                        {f.verified_purchase && " · Verified purchase"}{f.item_title ? ` · ${f.item_title}` : ""}
                      </p>
                      {f.item_rating != null && (
                        <p className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-[12px]">
                          <span>Item <Stars value={f.item_rating} label="Item" /></span>
                          <span>Shipping <Stars value={f.shipping_rating} label="Shipping" /></span>
                          <span>Communication <Stars value={f.communication_rating} label="Communication" /></span>
                        </p>
                      )}
                      {f.reply && (
                        <p className="mt-2 border-l-2 border-line pl-3 text-[13px]"><span className="font-semibold">Seller&apos;s reply:</span> {f.reply}</p>
                      )}
                      {f.can_reply && <ReplyForm id={f.id} />}
                    </div>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </section>
    </div>
  );
}
