import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import type { Order } from "@/lib/types";
import OrderView, { StatusPill, day } from "@/components/orders/OrderView";
import BuyerActions from "@/components/orders/BuyerActions";
import ShipForm from "@/components/orders/ShipForm";

export const dynamic = "force-dynamic";
export const metadata = { title: "Order", robots: { index: false } };

export default async function PurchaseOrder({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  await requireUser(`/purchases/${id}`);
  const r = await apiGet<{ order: Order }>(`/v1/orders/${encodeURIComponent(id)}`);
  const o = r.data?.order;
  if (!o) notFound();
  if (o.side === "selling") return <SellerRedirect id={o.id} />;
  return (
    <div className="pt-6">
      <Link href="/purchases" className="text-[14px] text-link hover:underline">← Purchases</Link>
      <div className="mt-2 flex flex-wrap items-center gap-3">
        <h1 className="text-[24px] font-bold">Order #{o.number}</h1>
        <StatusPill o={o} />
      </div>
      <p className="text-[14px] text-muted">Paid {day(o.paid_at)} · Seller {o.seller}</p>
      {o.test && o.seller === "sample_card_shop" && o.status === "paid" && (
        <div className="mt-4 rounded-lg border border-dashed border-slab p-4">
          <p className="text-[14px] font-semibold">Test order: play the seller</p>
          <p className="mt-1 text-[13px] text-muted">Nobody can sign in as the sample shop, so ship it from here. Then use &ldquo;I got it&rdquo; and leave feedback as the buyer.</p>
          <div className="mt-3"><ShipForm orderId={o.id} method={o.ship_method} /></div>
        </div>
      )}
      <BuyerActions o={o} />
      <div className="mt-6"><OrderView o={o} /></div>
    </div>
  );
}

function SellerRedirect({ id }: { id: string }) {
  return (
    <p className="py-16 text-center">
      You sold this order. <Link href={`/sell/orders/${id}`} className="text-link underline">Open it in Selling</Link>.
    </p>
  );
}
