import Link from "next/link";
import { apiGet, getSeller } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import { money } from "@/lib/format";
import type { FeedbackSummary } from "@/lib/types";
import OnboardButton from "@/components/OnboardButton";
import PromoBox from "@/components/PromoBox";
import type { Redemption } from "@/lib/promos";
import { getFees } from "@/lib/catalog";
import { CountRow, HubCard, SellingCosts, TIERS, stripeNeed } from "@/components/sell/HubBits";
import SalesChart from "@/components/sell/SalesChart";
import { SellerBadge } from "@/components/feedback/Bits";

export const dynamic = "force-dynamic";
export const metadata = { title: "Seller hub" };

type Overview = {
  listings: Record<string, number>;
  orders: Record<string, number>;
  unread_messages: number;
  offers_to_answer: number;
  sales: { days: { day: string; cents: number; orders: number }[]; periods: { key: string; cents: number; orders: number }[] };
  traffic: { views_90d: number; views_31d: number; orders_90d: number; conversion_90d: number };
  performance: { orders_12m: number; late_12m: number; late_percent: number; tracked_percent: number; chargebacks_12m: number; open_problems: number };
  feedback_30d: { positive: number; neutral: number; negative: number };
};

export default async function SellPage({ searchParams }: { searchParams: Promise<{ promo?: string }> }) {
  const { promo } = await searchParams;
  const code = typeof promo === "string" ? promo.slice(0, 32) : "";
  const me = await requireUser(code ? `/sell?promo=${encodeURIComponent(code)}` : "/sell");
  const [{ status, seller }, fees, promosR] = await Promise.all([
    getSeller(), getFees(), apiGet<{ fee_free_sales_left: number; redemptions: Redemption[] }>("/v1/seller/promos"),
  ]);
  const promoBox = (
    <PromoBox left={promosR.data?.fee_free_sales_left ?? 0} redemptions={promosR.data?.redemptions ?? []} initial={code} />
  );

  if (status === 404 || !seller) {
    return (
      <div className="mx-auto max-w-[640px] pt-12">
        <h1 className="text-[26px] font-bold">Start selling, {me.username}</h1>
        <p className="mt-2 text-muted">
          Sales are paid straight into your own Stripe account. Setting it up takes about five minutes and asks for
          your bank details and ID, handled by Stripe.
        </p>
        <ul className="mt-5 space-y-2 text-[15px]">
          <li>Every listing ships free, so build shipping into your price.</li>
          <li>New sellers can list cards up to $20 each, rising to $250 and then $1,000 as you build feedback.</li>
        </ul>
        <SellingCosts fees={fees} />
        <div className="mt-6">{promoBox}</div>
        <div className="mt-6"><OnboardButton label="Set up payments with Stripe" /></div>
      </div>
    );
  }

  if (!seller.charges_enabled) {
    return (
      <div className="mx-auto max-w-[640px] pt-12">
        <h1 className="text-[26px] font-bold">Finish your Stripe setup</h1>
        <p className="mt-2 text-muted">
          {seller.stripe_needs?.length
            ? "Stripe needs a little more before you can sell."
            : seller.details_submitted
              ? "Stripe is reviewing your details. Listing unlocks as soon as they approve, usually within minutes."
              : "Your Stripe account isn't finished yet. Pick up where you left off."}
        </p>
        {seller.stripe_needs && seller.stripe_needs.length > 0 && (
          <ul className="mt-3 list-disc space-y-1 pl-5 text-[14px]">
            {Array.from(new Set(seller.stripe_needs.map(stripeNeed))).map((n) => <li key={n}>{n}</li>)}
          </ul>
        )}
        {seller.stripe_verifying && seller.stripe_verifying.length > 0 && !seller.stripe_needs?.length && (
          <p className="mt-3 text-[14px] text-muted">Stripe is checking: {Array.from(new Set(seller.stripe_verifying.map(stripeNeed))).join(", ")}.</p>
        )}
        {(!seller.details_submitted || (seller.stripe_needs?.length ?? 0) > 0) && <div className="mt-6"><OnboardButton label="Continue Stripe setup" /></div>}
        <div className="mt-6">{promoBox}</div>
      </div>
    );
  }

  const [ovR, repR] = await Promise.all([
    apiGet<Overview>("/v1/seller/overview"),
    apiGet<{ summary: FeedbackSummary; feedback: { id: string; rating: number; comment: string; created_at: string; item_title: string }[] }>(
      `/v1/members/${encodeURIComponent(me.username)}/feedback?role=seller&limit=3`),
  ]);
  const ov = ovR.data;
  const rep = repR.data?.summary;
  const recent = repR.data?.feedback ?? [];
  const L = ov?.listings ?? {}, O = ov?.orders ?? {};
  const period = (k: string) => ov?.sales.periods.find((p) => p.key === k) ?? { cents: 0, orders: 0 };
  const current = TIERS.find((t) => t.tier === seller.tier) ?? TIERS[0];
  const nextTier = TIERS.find((t) => t.tier === seller.tier + 1);
  const leftFree = promosR.data?.fee_free_sales_left ?? 0;
  const todo = [
    O.to_ship ? { href: "/sell/orders", text: `${O.to_ship} ${O.to_ship === 1 ? "order" : "orders"} to ship${O.late ? ` (${O.late} late)` : ""}`, urgent: true } : null,
    ov?.offers_to_answer ? { href: "/offers?side=selling", text: `${ov.offers_to_answer} ${ov.offers_to_answer === 1 ? "offer needs" : "offers need"} a reply`, urgent: false } : null,
    ov?.unread_messages ? { href: "/messages", text: `${ov.unread_messages} unread ${ov.unread_messages === 1 ? "message" : "messages"}`, urgent: false } : null,
    O.open_problems ? { href: "/sell/orders?status=", text: `${O.open_problems} open ${O.open_problems === 1 ? "problem" : "problems"} with an order`, urgent: true } : null,
  ].filter(Boolean) as { href: string; text: string; urgent: boolean }[];
  const ms = rep?.milestone;

  return (
    <div className="pb-10 pt-4">
      {/* Header strip */}
      <section className="flex flex-wrap items-center justify-between gap-4 rounded-xl border border-line p-4">
        <div>
          <p className="text-[20px] font-bold">{me.username}</p>
          <p className="text-[14px] text-muted">
            {rep?.positive_percent != null ? `${rep.positive_percent.toFixed(1)}% positive` : "No feedback yet"}
            {" "}(<Link href={`/seller/${encodeURIComponent(me.username)}`} className="text-link hover:underline">{rep?.score ?? 0}</Link>)
          </p>
          {rep && rep.badges.length > 0 && (
            <ul className="mt-1.5 flex flex-wrap gap-1.5">{rep.badges.map((b) => <li key={b.key}><SellerBadge b={b} /></li>)}</ul>
          )}
        </div>
        <dl className="flex flex-wrap items-center gap-6 text-[14px]">
          <div><dt className="text-muted">Listing views (90d)</dt><dd className="num text-[20px] font-bold">{(ov?.traffic.views_90d ?? 0).toLocaleString()}</dd></div>
          <div><dt className="text-muted">Sales (90d)</dt><dd className="num text-[20px] font-bold">{money(period("90d").cents)}</dd></div>
          <div><dt className="text-muted">Orders (90d)</dt><dd className="num text-[20px] font-bold">{period("90d").orders.toLocaleString()}</dd></div>
          <Link href="/sell/new" className="rounded-full bg-link px-5 py-2.5 font-semibold text-white hover:bg-link-dark">List a card</Link>
        </dl>
      </section>

      {seller.review_status === "rejected" && (
        <p role="alert" className="mt-4 rounded-lg border border-slab/40 bg-slab/5 p-4 text-[14px] text-slab">
          Your seller account wasn&apos;t approved, so your listings are hidden and you can&apos;t list new cards.
          See <a href="/help/contact" className="underline">Contact us</a> if you think this is a mistake.
        </p>
      )}

      {/* To-do bar */}
      <section className="mt-4 rounded-xl bg-wash p-4 text-[14px]" aria-label="To do">
        {todo.length === 0 ? (
          <p><strong>✓ You&apos;re all caught up!</strong> New tasks, like orders to ship or offers to answer, will show up here.</p>
        ) : (
          <ul className="flex flex-wrap gap-2">
            {todo.map((t) => (
              <li key={t.href + t.text}>
                <Link href={t.href} className={`inline-block rounded-lg px-4 py-2 font-semibold ${t.urgent ? "bg-ink text-white hover:opacity-90" : "border border-ink bg-paper hover:bg-wash"}`}>{t.text}</Link>
              </li>
            ))}
          </ul>
        )}
      </section>

      {(code || leftFree > 0) && <div className="mt-4">{promoBox}</div>}

      <div className="mt-4 grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        <HubCard title="Listings" href="/sell/listings">
          <p className="mb-1"><Link href="/sell/new" className="font-semibold text-link hover:underline">List a card</Link>{" · "}
            <Link href="/sell/import" className="text-link hover:underline">Upload a CSV</Link></p>
          <ul className="text-[14px]">
            <CountRow href="/sell/listings?show=active" label="Active listings" n={L.active} />
            <CountRow href="/sell/listings?show=with_offers" label="With open offers from buyers" n={L.with_offers} />
            <CountRow href="/sell/listings?show=auctions" label="All auctions" n={L.auctions} />
            <CountRow href="/sell/listings?show=ending_today" label="Auctions ending today" n={L.ending_today} />
            <CountRow href="/sell/listings?show=auctions" label="With reserve met" n={L.reserve_met} />
            <CountRow href="/sell/listings?show=unsold" label="Unsold and not relisted" n={L.unsold} />
            <CountRow href="/sell/listings?show=hidden" label="Hidden" n={L.hidden} />
            <CountRow href="/sell/listings?show=sold_out" label="Sold out" n={L.sold_out} />
          </ul>
        </HubCard>

        <HubCard title="Orders" href="/sell/orders">
          <ul className="text-[14px]">
            <CountRow href="/sell/orders" label="Awaiting shipment: print a label" n={O.to_ship} strong />
            <CountRow href="/sell/orders" label="Past the ship-by date" n={O.late} strong />
            <CountRow href="/offers?side=selling" label="Awaiting the buyer's payment" n={O.awaiting_payment} />
            <CountRow href="/sell/orders?status=" label="Open problems" n={O.open_problems} strong />
            <CountRow href="/sell/orders?status=shipped" label="Shipped and awaiting your feedback" n={O.awaiting_feedback} />
            <CountRow href="/sell/orders" label="Orders you can ship together" n={O.combinable} />
            <CountRow href="/sell/orders?status=" label="See all orders" />
          </ul>
        </HubCard>

        <HubCard title="Sales" href="/sell/reports">
          <SalesChart days={ov?.sales.days ?? []} />
          <dl className="mt-2 text-[14px]">
            {[["today", "Today"], ["7d", "Last 7 days"], ["31d", "Last 31 days"], ["90d", "Last 90 days"]].map(([k, label]) => (
              <div key={k} className="flex justify-between border-b border-wash py-1 last:border-0">
                <dt>{label}</dt><dd className="num font-semibold">{money(period(k).cents)} <span className="font-normal text-muted">· {period(k).orders}</span></dd>
              </div>
            ))}
          </dl>
          <p className="mt-1 text-[12px] text-muted">Paid orders, less refunds, US Eastern time.</p>
        </HubCard>

        <HubCard title="Traffic">
          <dl className="text-[14px]">
            <div className="flex justify-between border-b border-wash py-1.5"><dt>Listing views, last 31 days</dt><dd className="num font-semibold">{(ov?.traffic.views_31d ?? 0).toLocaleString()}</dd></div>
            <div className="flex justify-between border-b border-wash py-1.5"><dt>Listing views, last 90 days</dt><dd className="num font-semibold">{(ov?.traffic.views_90d ?? 0).toLocaleString()}</dd></div>
            <div className="flex justify-between py-1.5"><dt>Views that became orders (90d)</dt><dd className="num font-semibold">{(ov?.traffic.conversion_90d ?? 0).toFixed(1)}%</dd></div>
          </dl>
          <p className="mt-1 text-[12px] text-muted">Good photos of the front and back and a fair price turn more views into sales.</p>
        </HubCard>

        <HubCard title="Seller level" href="/sell/performance">
          <dl className="text-[14px]">
            <div className="flex justify-between border-b border-wash py-1.5"><dt>Price limit</dt><dd className="num font-semibold">{money(current.cap)} per card</dd></div>
            <div className="flex justify-between border-b border-wash py-1.5"><dt>Late shipment rate (12 months)</dt><dd className="num font-semibold">{(ov?.performance.late_percent ?? 0).toFixed(1)}%</dd></div>
            <div className="flex justify-between border-b border-wash py-1.5"><dt>Label orders with tracking</dt><dd className="num font-semibold">{(ov?.performance.tracked_percent ?? 100).toFixed(0)}%</dd></div>
            <div className="flex justify-between border-b border-wash py-1.5"><dt>Chargebacks lost (12 months)</dt><dd className="num font-semibold">{ov?.performance.chargebacks_12m ?? 0}</dd></div>
            <div className="flex justify-between py-1.5"><dt>Open problems</dt><dd className="num font-semibold">{ov?.performance.open_problems ?? 0}</dd></div>
          </dl>
          <p className="mt-1 text-[12px] text-muted">{nextTier ? `Next: up to ${money(nextTier.cap)} after ${nextTier.need}.` : "You're at the top level."} Checked daily.</p>
        </HubCard>

        <HubCard title="Badges" href="/help/seller-badges">
          {rep && rep.badges.length > 0
            ? <ul className="flex flex-wrap gap-1.5">{rep.badges.map((b) => <li key={b.key}><SellerBadge b={b} size="lg" /></li>)}</ul>
            : <p className="text-[14px] text-muted">No badges yet.</p>}
          {ms && ms.next_key && (
            <div className="mt-3 text-[14px]">
              <p>{(rep?.sales ?? 0).toLocaleString()} of {ms.next_sales.toLocaleString()} sales to <strong>{ms.next_label}</strong></p>
              <div className="mt-1 h-2 overflow-hidden rounded-full bg-wash" role="progressbar" aria-valuemin={0} aria-valuemax={ms.next_sales} aria-valuenow={rep?.sales ?? 0}>
                <div className="h-full rounded-full bg-ink" style={{ width: `${Math.min(100, ((rep?.sales ?? 0) / ms.next_sales) * 100)}%` }} />
              </div>
            </div>
          )}
          <p className="mt-2 text-[12px] text-muted">Top Rated: 50+ sales, 98%+ positive, ships on time.</p>
        </HubCard>

        <HubCard title="Feedback" href={`/seller/${encodeURIComponent(me.username)}`}>
          <p className="text-[13px] font-semibold text-muted">Last 30 days</p>
          <dl className="mt-1 flex justify-between text-[14px]">
            <div><dt className="sr-only">Positive</dt><dd><span aria-hidden className="text-ship">⊕</span> <span className="num">{ov?.feedback_30d.positive ?? 0}</span> Positive</dd></div>
            <div><dt className="sr-only">Neutral</dt><dd><span aria-hidden className="text-muted">⊙</span> <span className="num">{ov?.feedback_30d.neutral ?? 0}</span> Neutral</dd></div>
            <div><dt className="sr-only">Negative</dt><dd><span aria-hidden className="text-slab">⊖</span> <span className="num">{ov?.feedback_30d.negative ?? 0}</span> Negative</dd></div>
          </dl>
          <p className="mt-3 text-[13px] font-semibold text-muted">Most recent</p>
          {recent.length === 0 ? <p className="text-[14px] text-muted">No feedback yet.</p> : (
            <ul className="text-[14px]">
              {recent.map((f) => (
                <li key={f.id} className="flex justify-between gap-3 border-b border-wash py-1.5 last:border-0">
                  <span className="line-clamp-1">{f.rating > 0 ? "⊕" : f.rating < 0 ? "⊖" : "⊙"} {f.comment || f.item_title}</span>
                  <span className="whitespace-nowrap text-muted">{new Date(f.created_at).toLocaleDateString("en-US", { month: "short", day: "numeric" })}</span>
                </li>
              ))}
            </ul>
          )}
          <p className="mt-2 text-[14px]"><Link href="/sell/feedback" className="text-link hover:underline">Feedback settings</Link></p>
        </HubCard>

        <HubCard title="Marketing" href="/sell/marketing">
          <ul className="text-[14px]">
            <CountRow href="/sell/marketing" label="Fee-free sales left (promo codes)" n={leftFree} />
            <CountRow href="/sell/marketing" label={seller.bundle_discount_type && seller.bundle_discount_type !== "none" ? "Multi-card discount: on" : "Multi-card discount: off"} />
            <CountRow href="/sell/trusted" label="Trusted buyers" />
          </ul>
        </HubCard>

        <HubCard title="Selling tools">
          <ul className="text-[14px]">
            <CountRow href="/sell/connect" label="Connect FireCardManager" />
            <CountRow href="/sell/import" label="Upload a CSV" />
            <CountRow href="/sell/shipping" label="Shipping: ship-from address and label card" />
            <CountRow href="/sell/payments" label="Payments and sales tax" />
            <CountRow href="/sell/reports" label="Reports and sales CSV" />
            <CountRow href="/help/selling" label="Selling help" />
          </ul>
        </HubCard>
      </div>
    </div>
  );
}
