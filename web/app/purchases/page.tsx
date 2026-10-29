import Link from "next/link";
import { apiGet, checkoutEnabled } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import type { Hold, Order } from "@/lib/types";
import { endTime, money } from "@/lib/format";
import PayButton from "@/components/orders/PayButton";
import { StatusPill, day } from "@/components/orders/OrderView";

export const dynamic = "force-dynamic";
export const metadata = { title: "Purchases", robots: { index: false } };

export default async function PurchasesPage() {
  await requireUser("/purchases");
  const [hr, or, canPay] = await Promise.all([
    apiGet<{ holds: Hold[] }>("/v1/holds?side=buying"),
    apiGet<{ orders: Order[] }>("/v1/orders?side=buying"),
    checkoutEnabled(),
  ]);
  const holds = hr.data?.holds ?? [];
  const orders = or.data?.orders ?? [];
  const due = holds.filter((h) => h.status === "awaiting_payment");
  const bySeller = new Map<string, Hold[]>();
  for (const h of due) bySeller.set(h.seller, [...(bySeller.get(h.seller) ?? []), h]);
  const past = holds.filter((h) => h.status !== "awaiting_payment" && h.status !== "paid");

  return (
    <div className="pt-6">
      <h1 className="text-[26px] font-bold">Purchases</h1>

      {(!hr.data || !or.data) && (
        <div role="alert" className="mt-5 rounded-lg border border-line bg-wash p-4 text-[14px]">
          <p className="font-semibold">We couldn&apos;t load all of your purchases.</p>
          <p className="mt-1 text-muted">This is usually a brief hiccup. <Link href="/purchases" className="text-link underline">Try again</Link> in a moment.</p>
        </div>
      )}

      {hr.data && or.data && holds.length === 0 && orders.length === 0 && (
        <div className="py-14 text-center">
          <p className="text-[17px] font-semibold">Nothing here yet.</p>
          <p className="mt-1 text-muted">Orders, auction wins and accepted offers show up here.</p>
          <Link href="/search?sort=newest" className="mt-5 inline-block rounded-full bg-link px-5 py-2 font-semibold text-white">Browse cards</Link>
        </div>
      )}

      {due.length > 0 && (
        <section className="mt-5">
          <h2 className="border-b border-line pb-2 text-[20px] font-bold">Awaiting payment</h2>
          {!canPay && (
            <p className="mt-3 rounded-lg bg-wash p-3 text-[14px]">
              Checkout opens shortly. Your cards stay held for you until then, and you&apos;ll get a notification when you can pay.
            </p>
          )}
          {[...bySeller.entries()].map(([seller, hs]) => {
            const total = hs.reduce((s, h) => s + h.unit_cents * h.quantity, 0);
            return (
              <div key={seller} className="mt-4 rounded-lg border border-line">
                <ul>
                  {hs.map((h) => (
                    <li key={h.id} className="flex gap-4 border-b border-wash p-4 text-[14px] last:border-0">
                      <Link href={`/item/${h.listing_id}`} className="shrink-0">
                        {/* eslint-disable-next-line @next/next/no-img-element */}
                        <img src={h.listing_photo} alt="" className="h-[84px] w-[60px] rounded bg-wash object-contain" />
                      </Link>
                      <div className="min-w-0 flex-1">
                        <Link href={`/item/${h.listing_id}`} className="title-tight text-[15px] hover:underline">{h.listing_title}</Link>
                        <p className="text-muted">{h.source === "auction" ? "Won auction" : "Accepted offer"} · Pay by <span suppressHydrationWarning>{endTime(h.due_at)}</span></p>
                      </div>
                      <p className="num shrink-0 font-semibold">{money(h.unit_cents * h.quantity)}</p>
                    </li>
                  ))}
                </ul>
                <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line bg-wash px-4 py-3 text-[14px]">
                  <span>Seller <strong>{seller}</strong> · Free shipping</span>
                  {canPay && <PayButton holdIds={hs.map((h) => h.id)} label={`Pay ${money(total)}`} className="w-full sm:w-auto" />}
                </div>
              </div>
            );
          })}
        </section>
      )}

      {orders.length > 0 && (
        <section className="mt-8">
          <h2 className="border-b border-line pb-2 text-[20px] font-bold">Orders</h2>
          <ul>
            {orders.map((o) => (
              <li key={o.id} className="border-b border-line py-4">
                <Link href={`/purchases/${o.id}`} className="group flex gap-4 text-[14px]">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  {o.lines[0]?.photo ? <img src={o.lines[0].photo} alt="" className="h-[84px] w-[60px] shrink-0 rounded bg-wash object-contain" /> : <div className="h-[84px] w-[60px] shrink-0 rounded bg-wash" />}
                  <div className="min-w-0 flex-1">
                    <p className="title-tight text-[15px] group-hover:underline">
                      {o.lines[0]?.title}{o.lines.length > 1 ? ` + ${o.lines.length - 1} more` : ""}
                    </p>
                    <p className="text-muted">Order #{o.number} · {day(o.paid_at)} · from {o.seller}</p>
                    <p className="mt-1"><StatusPill o={o} /></p>
                  </div>
                  <p className="num shrink-0 font-semibold">{money(o.total_cents)}</p>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}

      {past.length > 0 && (
        <section className="mt-8">
          <h2 className="border-b border-line pb-2 text-[17px] font-bold">Released</h2>
          <ul className="text-[14px]">
            {past.map((h) => (
              <li key={h.id} className="flex justify-between gap-3 border-b border-wash py-2">
                <Link href={`/item/${h.listing_id}`} className="min-w-0 truncate hover:underline">{h.listing_title}</Link>
                <span className="shrink-0 text-muted">{h.status === "expired" ? "Not paid in time" : "Cancelled"}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}
