import Link from "next/link";
import { notFound, redirect } from "next/navigation";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import type { Order } from "@/lib/types";
import OrderView, { StatusPill, day } from "@/components/orders/OrderView";
import ShipForm from "@/components/orders/ShipForm";
import BuyerActions from "@/components/orders/BuyerActions";

export const dynamic = "force-dynamic";
export const metadata = { title: "Order", robots: { index: false } };

export default async function SellerOrder({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  await requireUser(`/sell/orders/${id}`);
  const r = await apiGet<{ order: Order }>(`/v1/orders/${encodeURIComponent(id)}`);
  const o = r.data?.order;
  if (!o) notFound();
  if (o.side === "buying") redirect(`/purchases/${o.id}`);
  return (
    <div className="pt-6">
      <Link href="/sell/orders" className="text-[14px] text-link hover:underline">← Orders</Link>
      <div className="mt-2 flex flex-wrap items-center gap-3">
        <h1 className="text-[24px] font-bold">Order #{o.number}</h1>
        <StatusPill o={o} />
      </div>
      <p className="text-[14px] text-muted">Paid {day(o.paid_at)} · Buyer {o.buyer}</p>
      {o.status === "paid" && (
        <div className="mt-4 max-w-[560px] rounded-lg border border-ink p-4">
          <h2 className="font-semibold">Ship by {day(o.ship_by)}</h2>
          <p className="text-[14px] text-muted">
            {o.ship_method === "envelope"
              ? "One raw card: a stamped envelope is fine. Add tracking if you have it."
              : "Ship in one package with tracking, then enter the tracking number."}
          </p>
          <ShipForm orderId={o.id} method={o.ship_method} />
        </div>
      )}
      <BuyerActions o={o} />
      <div className="mt-4 flex flex-wrap gap-x-5 gap-y-2 text-[14px]">
        {o.stripe_payment_url && (
          <a href={o.stripe_payment_url} target="_blank" rel="noopener noreferrer" className="font-semibold text-link hover:underline">
            Refund in Stripe
          </a>
        )}
        <a href={`/api/v1/orders/${o.id}/evidence`} className="font-semibold text-link hover:underline">Download evidence packet</a>
      </div>
      <p className="mt-1 text-[13px] text-muted">
        Refunds are made from your own Stripe dashboard; the order updates here automatically. The evidence packet holds the
        order, the listing as it was sold, tracking and your messages, for answering a chargeback.
      </p>
      <div className="mt-6"><OrderView o={o} /></div>
    </div>
  );
}
