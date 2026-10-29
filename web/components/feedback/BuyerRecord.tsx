"use client";

import { useState } from "react";
import { send } from "@/lib/client";

type Rec = {
  username: string; member_since: string; orders_paid: number; wins_paid: number; wins_unpaid: number; pay_rate: number | null;
  unpaid_strikes: number; cases_opened: number; orders_refunded: number; refund_rate: number | null; chargebacks: number;
  from_sellers: { score: number; positive_percent: number | null; positive_12m: number; neutral_12m: number };
};

/** A seller's look at a buyer's track record (orders, payment, strikes, cases). */
export default function BuyerRecord({ username }: { username: string }) {
  const [rec, setRec] = useState<Rec | null>(null);
  const [error, setError] = useState("");
  const [open, setOpen] = useState(false);
  async function load() {
    setOpen(true);
    if (rec) return;
    const r = await send<{ buyer: Rec }>("GET", `/v1/members/${encodeURIComponent(username)}/buyer-record`);
    if (!r.ok) return setError(r.message);
    setRec(r.data.buyer);
  }
  if (!open) {
    return <button type="button" onClick={load} className="text-[13px] font-semibold text-link hover:underline">Buyer record</button>;
  }
  if (!rec && !error) return <p className="text-[13px] text-muted">Loading buyer record…</p>;
  if (error) return <p className="text-[13px] text-slab">{error}</p>;
  const r = rec!;
  const flag = r.unpaid_strikes > 0 || r.chargebacks > 0 || (r.pay_rate != null && r.pay_rate < 90);
  return (
    <div className={`mt-2 rounded-lg border p-3 text-[13px] ${flag ? "border-slab/50" : "border-line"}`}>
      <p className="font-semibold">{r.username}&apos;s buyer record · member since {new Date(r.member_since).toLocaleDateString("en-US", { month: "short", year: "numeric" })}</p>
      <dl className="num mt-1 grid grid-cols-2 gap-x-4 gap-y-0.5 sm:grid-cols-3">
        <div><dt className="inline text-muted">Orders paid </dt><dd className="inline font-semibold">{r.orders_paid}</dd></div>
        <div><dt className="inline text-muted">Pays wins/offers </dt><dd className="inline font-semibold">{r.pay_rate != null ? `${r.pay_rate}%` : "—"}</dd></div>
        <div><dt className="inline text-muted">Unpaid strikes </dt><dd className={`inline font-semibold ${r.unpaid_strikes ? "text-slab" : ""}`}>{r.unpaid_strikes}</dd></div>
        <div><dt className="inline text-muted">Cases opened </dt><dd className="inline font-semibold">{r.cases_opened}</dd></div>
        <div><dt className="inline text-muted">Refund rate </dt><dd className="inline font-semibold">{r.refund_rate != null ? `${r.refund_rate}%` : "—"}</dd></div>
        <div><dt className="inline text-muted">Chargebacks </dt><dd className={`inline font-semibold ${r.chargebacks ? "text-slab" : ""}`}>{r.chargebacks}</dd></div>
      </dl>
      <p className="mt-1 text-muted">From sellers: score {r.from_sellers.score}{r.from_sellers.positive_percent != null ? `, ${r.from_sellers.positive_percent}% positive` : ""}</p>
    </div>
  );
}
