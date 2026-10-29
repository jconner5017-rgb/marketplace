import Link from "next/link";
import { notFound, redirect } from "next/navigation";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import type { Order } from "@/lib/types";
import OrderView, { StatusPill, day } from "@/components/orders/OrderView";
import ShipForm from "@/components/orders/ShipForm";
import BuyerActions from "@/components/orders/BuyerActions";
import BuyerRecord from "@/components/feedback/BuyerRecord";
import { BuyLabel, VoidLabel } from "@/components/orders/LabelBuyer";

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
      <BuyerRecord username={o.buyer} />
      {o.status === "paid" && (
        <div className="mt-4 max-w-[560px] rounded-lg border border-ink p-4">
          <h2 className="font-semibold">Ship by {day(o.ship_by)}</h2>
          {o.ship_method === "envelope" ? (
            <p className="text-[14px] text-muted">One raw card: a stamped envelope is fine. Add tracking if you have it.</p>
          ) : (
            <>
              <p className="text-[14px] text-muted">Ship in one package with tracking. Buy the label here, or enter tracking from a label you bought elsewhere.</p>
              <BuyLabel orderId={o.id} />
              <p className="mt-4 text-[13px] font-semibold">Already have a label?</p>
            </>
          )}
          <ShipForm orderId={o.id} method={o.ship_method} />
        </div>
      )}
      {o.shipment?.label_url && (
        <div className="mt-4 flex max-w-[560px] flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border border-line p-4 text-[14px]">
          <a href={o.shipment.label_url} target="_blank" rel="noopener noreferrer" className="rounded-full bg-link px-4 py-2 font-semibold text-white hover:bg-link-dark">Print label</a>
          <span>{o.shipment.carrier} {o.shipment.service} · <span className="num">{o.shipment.tracking_number}</span></span>
          {o.shipment.status === "label_created" && <VoidLabel orderId={o.id} />}
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
