import Link from "next/link";
import { money } from "@/lib/format";
import type { Order } from "@/lib/types";

export const STATUS_LABEL: Record<Order["status"], string> = {
  paid: "Paid, waiting to ship",
  shipped: "Shipped",
  delivered: "Delivered",
  cancelled: "Cancelled",
  refunded: "Refunded",
  partially_refunded: "Partly refunded",
};

const dateFmt = new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", year: "numeric", timeZone: "America/New_York" });
export const day = (iso: string | null) => (iso ? dateFmt.format(new Date(iso)) : "");

export function trackingUrl(carrier: string | null, n: string | null): string | null {
  if (!n) return null;
  const t = encodeURIComponent(n);
  const c = (carrier ?? "").toUpperCase();
  switch (c === "FEDEX" ? "FedEx" : c) {
    case "USPS": return `https://tools.usps.com/go/TrackConfirmAction?tLabels=${t}`;
    case "UPS": return `https://www.ups.com/track?tracknum=${t}`;
    case "FedEx": return `https://www.fedex.com/fedextrack/?trknbr=${t}`;
    case "DHL": return `https://www.dhl.com/us-en/home/tracking.html?tracking-id=${t}`;
  }
  return null;
}

export function StatusPill({ o }: { o: Order }) {
  const late = o.status === "paid" && o.ship_by && new Date(o.ship_by).getTime() < Date.now();
  const tone = late ? "bg-slab/10 text-slab" : o.status === "paid" ? "bg-ink text-white" : o.status === "cancelled" || o.status.includes("refund") ? "bg-wash text-muted" : "bg-ship/10 text-ship";
  return <span className={`rounded-full px-2.5 py-0.5 text-[12px] font-semibold ${tone}`}>{late ? "Late to ship" : STATUS_LABEL[o.status]}</span>;
}

/** Cards, totals and shipping for one order, from either side. */
export default function OrderView({ o }: { o: Order }) {
  const selling = o.side === "selling";
  const track = o.shipment && (o.shipment.tracking_url || trackingUrl(o.shipment.carrier, o.shipment.tracking_number));
  return (
    <div className="grid gap-6 md:grid-cols-[1fr_300px]">
      <div>
        <ul className="rounded-lg border border-line">
          {o.lines.map((l, i) => (
            <li key={i} className="flex gap-4 border-b border-wash p-4 last:border-0 text-[14px]">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              {l.photo ? <img src={l.photo} alt="" className="h-[84px] w-[60px] shrink-0 rounded bg-wash object-contain" /> : <div className="h-[84px] w-[60px] shrink-0 rounded bg-wash" />}
              <div className="min-w-0 flex-1">
                {l.listing_id ? <Link href={`/item/${l.listing_id}`} className="title-tight text-[15px] hover:underline">{l.title}</Link> : <span className="title-tight text-[15px]">{l.title}</span>}
                <p className="text-muted">{l.source === "auction" ? "Won auction" : l.source === "offer" ? "Accepted offer" : "Bought now"}{l.quantity > 1 ? ` · ${l.quantity} × ${money(l.unit_cents)}` : ""}</p>
              </div>
              <p className="num shrink-0 font-semibold">{money(l.unit_cents * l.quantity)}</p>
            </li>
          ))}
        </ul>
        <dl className="num mt-3 space-y-1 px-1 text-[14px]">
          <div className="flex justify-between"><dt>Cards</dt><dd>{money(o.subtotal_cents)}</dd></div>
          {o.discount_cents > 0 && <div className="flex justify-between text-ship"><dt>Multi-card discount</dt><dd>−{money(o.discount_cents)}</dd></div>}
          <div className="flex justify-between"><dt>Shipping</dt><dd className="text-ship">Free</dd></div>
          {o.tax_cents > 0 && <div className="flex justify-between"><dt>Sales tax</dt><dd>{money(o.tax_cents)}</dd></div>}
          <div className="flex justify-between border-t border-line pt-2 font-bold"><dt>{selling ? "Buyer paid" : "Total paid"}</dt><dd>{money(o.total_cents + o.tax_cents)}</dd></div>
          {o.refunded_cents > 0 && <div className="flex justify-between text-slab"><dt>Refunded</dt><dd>−{money(o.refunded_cents)}</dd></div>}
          {selling && o.fee_cents != null && (
            <>
              <div className="flex justify-between text-muted"><dt>Marketplace fee</dt><dd>−{money(o.fee_cents)}</dd></div>
              <div className="flex justify-between font-semibold"><dt>You receive (before Stripe&apos;s processing fee{o.tax_cents > 0 ? " and sales tax you remit" : ""})</dt><dd>{money(o.total_cents - o.fee_cents - o.refunded_cents)}</dd></div>
            </>
          )}
        </dl>
      </div>

      <aside className="space-y-4 text-[14px]">
        <div className="rounded-lg border border-line p-4">
          <h2 className="font-semibold">{selling ? "Ship to" : "Shipping to"}</h2>
          {o.ship_to?.line1 ? (
            <address className="mt-1 not-italic leading-snug">
              {o.ship_to.name}<br />{o.ship_to.line1}{o.ship_to.line2 ? <><br />{o.ship_to.line2}</> : null}<br />
              {o.ship_to.city}, {o.ship_to.state} {o.ship_to.zip}
            </address>
          ) : <p className="mt-1 text-muted">Address on file with Stripe.</p>}
          <p className="mt-2 text-muted">{o.ship_method === "envelope" ? "Stamped envelope" : "Package with tracking"}</p>
          {o.status === "paid" && o.ship_by && <p className="mt-1">Ships by <strong>{day(o.ship_by)}</strong></p>}
        </div>
        {o.shipment && (
          <div className="rounded-lg border border-line p-4">
            <h2 className="font-semibold">Shipment</h2>
            <p className="mt-1">Sent {day(o.shipment.created_at)}{o.shipment.carrier ? ` with ${o.shipment.carrier}` : ""}</p>
            {o.shipment.tracking_number ? (
              track ? <a href={track} target="_blank" rel="noopener noreferrer" className="num mt-1 inline-block break-all text-link underline">{o.shipment.tracking_number}</a>
                : <p className="num mt-1 break-all">{o.shipment.tracking_number}</p>
            ) : <p className="mt-1 text-muted">No tracking (stamped envelope)</p>}
            {o.shipment.last_event && !o.delivered_at && <p className="mt-1 text-muted">Latest: {o.shipment.last_event}</p>}
            {o.delivered_at && <p className="mt-1 text-ship">Delivered {day(o.delivered_at)}</p>}
          </div>
        )}
        {o.cases.length > 0 && (
          <div className="rounded-lg border border-line p-4">
            <h2 className="font-semibold">Cases</h2>
            {o.cases.map((c) => <p key={c.number} className="mt-1">#{c.number} · {c.status.replace(/_/g, " ")}</p>)}
          </div>
        )}
        {(o.my_feedback || o.their_feedback) && (
          <div className="rounded-lg border border-line p-4">
            <h2 className="font-semibold">Feedback</h2>
            {o.my_feedback && <p className="mt-1">You: {o.my_feedback.rating > 0 ? "Positive" : o.my_feedback.rating < 0 ? "Negative" : "Neutral"}{o.my_feedback.comment ? ` · “${o.my_feedback.comment}”` : ""}</p>}
            {o.their_feedback && <p className="mt-1">{selling ? o.buyer : o.seller}: {o.their_feedback.rating > 0 ? "Positive" : o.their_feedback.rating < 0 ? "Negative" : "Neutral"}{o.their_feedback.comment ? ` · “${o.their_feedback.comment}”` : ""}</p>}
          </div>
        )}
      </aside>
    </div>
  );
}
