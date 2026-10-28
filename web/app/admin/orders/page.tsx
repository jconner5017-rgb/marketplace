import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { OrderList, ORDER_STATUS, type OrderRow } from "@/components/admin/OrderBits";

export const dynamic = "force-dynamic";
export const metadata = { title: "Orders" };

export default async function OrdersPage({ searchParams }: { searchParams: Promise<Record<string, string | undefined>> }) {
  await requireStaff("/admin/orders", "orders.view");
  const sp = await searchParams;
  const page = Math.max(1, Number(sp.page) || 1);
  const qs = new URLSearchParams({ limit: "50", offset: String((page - 1) * 50) });
  for (const k of ["q", "status", "party"]) if (sp[k]) qs.set(k, sp[k]!);
  const r = await apiGet<{ orders: OrderRow[]; total: number }>("/v1/admin/orders?" + qs.toString());
  const total = r.data?.total ?? 0;
  const link = (p: number) => {
    const n = new URLSearchParams(Object.entries(sp).filter(([, v]) => v) as [string, string][]);
    n.set("page", String(p));
    return `/admin/orders?${n}`;
  };
  return (
    <div>
      <h1 className="text-[26px] font-bold">Orders</h1>
      <p className="text-muted">Every order, with its buyer, seller, totals and Stripe IDs. Search by order number, order ID, Stripe payment ID or tracking number.</p>
      <form action="/admin/orders" className="mt-4 flex flex-wrap items-end gap-3 text-[14px]">
        <label className="min-w-[200px] flex-1"><span className="block text-[13px] font-semibold">Search</span>
          <input name="q" defaultValue={sp.q} placeholder="100123, pi_…, tracking number" className="mt-1 w-full rounded-md border border-line px-3 py-2" /></label>
        <label><span className="block text-[13px] font-semibold">Member</span>
          <input name="party" defaultValue={sp.party} placeholder="Buyer or seller username" className="mt-1 w-44 rounded-md border border-line px-3 py-2" /></label>
        <label><span className="block text-[13px] font-semibold">Status</span>
          <select name="status" defaultValue={sp.status ?? ""} className="mt-1 rounded-md border border-line px-2 py-2">
            <option value="">Any</option>
            {Object.entries(ORDER_STATUS).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
          </select></label>
        <button className="rounded-full bg-link px-5 py-2 font-semibold text-white hover:bg-link-dark">Search</button>
      </form>
      <p className="mb-2 mt-4 text-[13px] text-muted">{total.toLocaleString("en-US")} orders</p>
      <OrderList rows={r.data?.orders ?? []} />
      <div className="mt-4 flex gap-4 text-[14px]">
        {page > 1 && <Link href={link(page - 1)} className="text-link underline">Previous</Link>}
        {page * 50 < total && <Link href={link(page + 1)} className="text-link underline">Next</Link>}
      </div>
    </div>
  );
}
