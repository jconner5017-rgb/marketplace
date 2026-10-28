import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import type { Hold } from "@/lib/types";
import { endTime, money } from "@/lib/format";

export const dynamic = "force-dynamic";
export const metadata = { title: "Purchases", robots: { index: false } };

export default async function PurchasesPage() {
  await requireUser("/purchases");
  const r = await apiGet<{ holds: Hold[] }>("/v1/holds?side=buying");
  const holds = r.data?.holds ?? [];
  const due = holds.filter((h) => h.status === "awaiting_payment");
  const total = due.reduce((s, h) => s + h.unit_cents * h.quantity, 0);
  return (
    <div className="pt-6">
      <h1 className="text-[26px] font-bold">Purchases</h1>
      <p className="text-muted">Auctions you won and offers that were accepted.</p>

      {holds.length === 0 ? (
        <div className="py-14 text-center">
          <p className="text-[17px] font-semibold">Nothing here yet.</p>
          <p className="mt-1 text-muted">Win an auction or get an offer accepted and the card is held for you here.</p>
          <Link href="/search?format=auction&sort=ending" className="mt-5 inline-block rounded-full bg-link px-5 py-2 font-semibold text-white">Auctions ending soon</Link>
        </div>
      ) : (
        <>
          {due.length > 0 && (
            <div className="mt-4 rounded-lg border border-ink p-4 text-[14px]">
              <p className="font-semibold">Awaiting payment: {due.length} {due.length === 1 ? "card" : "cards"}, {money(total)}</p>
              <p className="mt-1 text-muted">
                Each card is held for you until its pay-by date. Checkout opens here shortly; you&apos;ll get a notification when you can pay.
              </p>
            </div>
          )}
          <ul className="mt-4">
            {holds.map((h) => (
              <li key={h.id} className="flex gap-4 border-b border-line py-4 text-[14px]">
                <Link href={`/item/${h.listing_id}`} className="shrink-0">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src={h.listing_photo} alt="" className="h-[98px] w-[70px] rounded bg-wash object-contain" />
                </Link>
                <div className="min-w-0 flex-1">
                  <Link href={`/item/${h.listing_id}`} className="title-tight text-[16px] hover:underline">{h.listing_title}</Link>
                  <p className="text-muted">Seller: {h.seller} · {h.source === "auction" ? "Won auction" : "Accepted offer"}</p>
                  <p className="num mt-1 text-[18px] font-bold">
                    {money(h.unit_cents * h.quantity)}
                    {h.quantity > 1 && <span className="text-[13px] font-normal text-muted"> ({h.quantity} × {money(h.unit_cents)})</span>}
                  </p>
                  {h.status === "awaiting_payment" ? (
                    <p className="text-slab">Pay by <span suppressHydrationWarning>{endTime(h.due_at)}</span></p>
                  ) : (
                    <p className="text-muted">{h.status === "paid" ? "Paid" : h.status === "expired" ? "Not paid in time" : "Cancelled"}</p>
                  )}
                </div>
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}
