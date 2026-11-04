import Link from "next/link";
import { apiGet, getSeller } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import { money } from "@/lib/format";
import type { FeedbackSummary } from "@/lib/types";
import { TIERS } from "@/components/sell/HubBits";
import { SellerBadge } from "@/components/feedback/Bits";

export const dynamic = "force-dynamic";
export const metadata = { title: "Performance", robots: { index: false } };

type Perf = { orders_12m: number; late_12m: number; late_percent: number; tracked_percent: number; chargebacks_12m: number; open_problems: number };

const LADDER = [
  { key: "bronze", label: "Bronze seller", sales: 10 },
  { key: "silver", label: "Silver seller", sales: 50 },
  { key: "gold", label: "Gold seller", sales: 250 },
  { key: "platinum", label: "Platinum seller", sales: 1000 },
  { key: "diamond", label: "Diamond seller", sales: 5000 },
];

export default async function SellerPerformance() {
  const me = await requireUser("/sell/performance");
  const [{ seller }, ovR, repR] = await Promise.all([
    getSeller(),
    apiGet<{ performance: Perf }>("/v1/seller/overview"),
    apiGet<{ summary: FeedbackSummary }>(`/v1/members/${encodeURIComponent(me.username)}/feedback?role=seller&limit=1`),
  ]);
  const p = ovR.data?.performance;
  const rep = repR.data?.summary;
  const tier = seller?.tier ?? 1;
  const current = TIERS.find((t) => t.tier === tier) ?? TIERS[0];
  const sales = rep?.sales ?? 0;
  const pct = rep?.positive_percent;
  const topRated = rep?.badges.some((b) => b.key === "top_rated");
  const checks = [
    { label: "50 or more completed sales", ok: sales >= 50, now: sales.toLocaleString() },
    { label: "98% or more positive feedback (12 months)", ok: pct == null || pct >= 98, now: pct != null ? `${pct.toFixed(1)}%` : "No feedback yet" },
    { label: "No more than 5% of orders shipped late", ok: (p?.late_percent ?? 0) <= 5, now: `${(p?.late_percent ?? 0).toFixed(1)}%` },
    { label: "No more than 1% of orders lost to a chargeback", ok: !p || p.orders_12m === 0 || p.chargebacks_12m * 100 <= p.orders_12m, now: String(p?.chargebacks_12m ?? 0) },
  ];

  return (
    <div className="pb-10 pt-6">
      <h1 className="text-[26px] font-bold">Performance</h1>
      <div className="mt-5 grid gap-4 md:grid-cols-2">
        <section className="rounded-xl border border-line p-4">
          <h2 className="font-bold">Seller level: {money(current.cap)} per card</h2>
          <ol className="mt-3 space-y-2 text-[14px]">
            {TIERS.map((t) => (
              <li key={t.tier} className={`rounded-lg px-3 py-2 ${t.tier === tier ? "bg-ink text-white" : t.tier < tier ? "bg-wash" : "border border-line"}`}>
                <span className="font-semibold">Up to {money(t.cap).replace(".00", "")}</span>
                <span className={t.tier === tier ? "text-white/80" : "text-muted"}> · {t.need}</span>
              </li>
            ))}
          </ol>
          <p className="mt-2 text-[13px] text-muted">Checked daily. <Link href="/help/seller-levels" className="text-link underline">How levels work</Link></p>
        </section>

        <section className="rounded-xl border border-line p-4">
          <h2 className="font-bold">Service, last 12 months</h2>
          <dl className="mt-2 text-[14px]">
            {[
              ["Orders", (p?.orders_12m ?? 0).toLocaleString()],
              ["Shipped late", `${p?.late_12m ?? 0} (${(p?.late_percent ?? 0).toFixed(1)}%)`],
              ["Label orders with tracking", `${(p?.tracked_percent ?? 100).toFixed(0)}%`],
              ["Chargebacks lost", String(p?.chargebacks_12m ?? 0)],
              ["Open problems now", String(p?.open_problems ?? 0)],
            ].map(([k, v]) => (
              <div key={k} className="flex justify-between border-b border-wash py-1.5 last:border-0"><dt>{k}</dt><dd className="num font-semibold">{v}</dd></div>
            ))}
          </dl>
        </section>

        <section className="rounded-xl border border-line p-4">
          <h2 className="font-bold">Sales badges</h2>
          <p className="mt-1 text-[14px] text-muted">{sales.toLocaleString()} completed {sales === 1 ? "sale" : "sales"}.</p>
          <ol className="mt-3 space-y-1.5 text-[14px]">
            {LADDER.map((b) => (
              <li key={b.key} className={`flex items-center justify-between gap-3 ${sales >= b.sales ? "" : "opacity-60"}`}>
                <SellerBadge b={{ key: b.key, label: b.label, hint: `${b.sales.toLocaleString()}+ completed sales` }} />
                <span className="num">{sales >= b.sales ? "Earned" : `${(b.sales - sales).toLocaleString()} to go`}</span>
              </li>
            ))}
          </ol>
        </section>

        <section className="rounded-xl border border-line p-4">
          <h2 className="font-bold">Top Rated {topRated ? <span className="ml-1 align-middle"><SellerBadge b={{ key: "top_rated", label: "Top Rated", hint: "" }} /></span> : null}</h2>
          <ul className="mt-2 text-[14px]">
            {checks.map((c) => (
              <li key={c.label} className="flex justify-between gap-3 border-b border-wash py-1.5 last:border-0">
                <span><span aria-hidden className={c.ok ? "text-ship" : "text-slab"}>{c.ok ? "✓" : "✗"}</span> {c.label}</span>
                <span className="num whitespace-nowrap font-semibold">{c.now}</span>
              </li>
            ))}
          </ul>
          <p className="mt-2 text-[13px] text-muted"><Link href="/help/seller-badges" className="text-link underline">About badges</Link></p>
        </section>
      </div>
    </div>
  );
}
