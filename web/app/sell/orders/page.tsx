import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import { money } from "@/lib/format";
import type { Order } from "@/lib/types";
import ShipForm from "@/components/orders/ShipForm";
import { StatusPill, day } from "@/components/orders/OrderView";

export const dynamic = "force-dynamic";
export const metadata = { title: "Orders", robots: { index: false } };

const TABS = [
  { key: "to_ship", label: "To ship" },
  { key: "shipped", label: "Shipped" },
  { key: "delivered", label: "Delivered" },
  { key: "", label: "All" },
];

export default async function SellerOrders({ searchParams }: { searchParams: Promise<{ status?: string }> }) {
  const sp = await searchParams;
  const status = TABS.some((t) => t.key === sp.status) ? sp.status! : "to_ship";
  await requireUser("/sell/orders");
  const r = await apiGet<{ orders: Order[] }>(`/v1/orders?side=selling&status=${status}`);
  const orders = r.data?.orders ?? [];

  return (
    <div className="pt-6">
      <Link href="/sell" className="text-[14px] text-link hover:underline">← Seller hub</Link>
      <h1 className="mt-2 text-[26px] font-bold">Orders</h1>
      <nav aria-label="Order status" className="mt-3 flex gap-1 overflow-x-auto border-b border-line text-[14px]">
        {TABS.map((t) => (
          <Link key={t.key} href={t.key ? `/sell/orders?status=${t.key}` : "/sell/orders?status="}
            aria-current={t.key === status ? "page" : undefined}
            className={`whitespace-nowrap border-b-2 px-3 py-2 font-semibold ${t.key === status ? "border-ink" : "border-transparent text-muted hover:text-ink"}`}>
            {t.label}
          </Link>
        ))}
      </nav>

      {orders.length === 0 ? (
        <p className="py-12 text-center text-muted">{status === "to_ship" ? "Nothing to ship right now." : "No orders here yet."}</p>
      ) : (
        <ul>
          {orders.map((o) => (
            <li key={o.id} className="border-b border-line py-4 text-[14px]">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <Link href={`/sell/orders/${o.id}`} className="font-semibold text-link hover:underline">Order #{o.number}</Link>
                <span className="flex items-center gap-2"><StatusPill o={o} /><span className="num font-semibold">{money(o.total_cents)}</span></span>
              </div>
              <p className="text-muted">
                {o.buyer} · paid {day(o.paid_at)}
                {o.status === "paid" && o.ship_by ? <> · ship by <strong className="text-ink">{day(o.ship_by)}</strong></> : null}
              </p>
              <ul className="mt-1">
                {o.lines.map((l, i) => <li key={i} className="truncate">{l.quantity > 1 ? `${l.quantity} × ` : ""}{l.title}</li>)}
              </ul>
              {o.status === "paid" && (
                <>
                  <p className="mt-2">
                    {o.ship_to?.name}, {o.ship_to?.line1}{o.ship_to?.line2 ? `, ${o.ship_to.line2}` : ""}, {o.ship_to?.city}, {o.ship_to?.state} {o.ship_to?.zip}
                  </p>
                  <ShipForm orderId={o.id} method={o.ship_method} />
                </>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
