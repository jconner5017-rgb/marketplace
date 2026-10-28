import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { OrderList, type OrderRow } from "@/components/admin/OrderBits";

export const dynamic = "force-dynamic";
export const metadata = { title: "Shipping" };

const TABS = [["late", "Late to ship"], ["awaiting", "Awaiting shipment"], ["in_transit", "In transit"], ["exception", "Problems"], ["delivered", "Delivered"]];

export default async function ShippingPage({ searchParams }: { searchParams: Promise<{ view?: string }> }) {
  await requireStaff("/admin/shipping", "orders.view");
  const view = (await searchParams).view ?? "late";
  const r = await apiGet<{ orders: OrderRow[]; total: number }>(`/v1/admin/orders?shipping=${view}&limit=100`);
  return (
    <div>
      <h1 className="text-[26px] font-bold">Shipping</h1>
      <p className="text-muted">Sellers ship within three business days of payment. Late orders count against the seller&apos;s level.</p>
      <nav className="mt-4 flex flex-wrap gap-2 text-[13px]">
        {TABS.map(([v, l]) => (
          <Link key={v} href={`/admin/shipping?view=${v}`} className={`rounded-full border px-3 py-1 ${view === v ? "border-ink bg-ink text-white" : "border-line hover:border-ink"}`}>{l}</Link>
        ))}
      </nav>
      <p className="mb-2 mt-4 text-[13px] text-muted">{r.data?.total ?? 0} orders</p>
      <OrderList rows={r.data?.orders ?? []} />
    </div>
  );
}
