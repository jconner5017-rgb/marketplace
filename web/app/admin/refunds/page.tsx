import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { money } from "@/lib/format";
import { when } from "@/components/admin/Badges";
import { REFUND_KIND, type RefundReport } from "@/lib/admin-types";

export const dynamic = "force-dynamic";
export const metadata = { title: "Refunds and chargebacks" };

const TABS = [["", "All"], ["refund", "Refunds"], ["partial_refund", "Partial refunds"], ["chargebacks", "Chargebacks"]];

export default async function RefundsPage({ searchParams }: { searchParams: Promise<{ kind?: string }> }) {
  await requireStaff("/admin/refunds", "orders.view");
  const kind = (await searchParams).kind ?? "";
  const r = await apiGet<{ refunds: RefundReport[] }>(`/v1/admin/refunds?kind=${kind}`);
  const rows = r.data?.refunds ?? [];
  const total = rows.reduce((s, x) => s + x.amount_cents, 0);
  return (
    <div>
      <h1 className="text-[26px] font-bold">Refunds and chargebacks</h1>
      <p className="text-muted">What sellers, buyers, staff and Stripe have reported. Sellers issue refunds from their own Stripe accounts; we record them here.</p>
      <nav className="mt-4 flex flex-wrap gap-2 text-[13px]">
        {TABS.map(([v, l]) => (
          <Link key={v || "all"} href={`/admin/refunds?kind=${v}`} className={`rounded-full border px-3 py-1 ${kind === v ? "border-ink bg-ink text-white" : "border-line hover:border-ink"}`}>{l}</Link>
        ))}
      </nav>
      <p className="mt-4 text-[13px] text-muted">{rows.length} reports · {money(total)}</p>
      <div className="mt-2 overflow-x-auto">
        <table className="w-full min-w-[640px] text-[14px]">
          <thead><tr className="border-b border-line text-left text-[13px] text-muted">
            <th className="py-2 pr-3">When</th><th className="py-2 pr-3">Order</th><th className="py-2 pr-3">What</th>
            <th className="py-2 pr-3 text-right">Amount</th><th className="py-2 pr-3">Reported by</th><th className="py-2">Note</th></tr></thead>
          <tbody>
            {rows.map((f) => (
              <tr key={f.id} className="border-b border-wash align-top">
                <td className="py-2 pr-3 whitespace-nowrap" suppressHydrationWarning>{when(f.created_at)}</td>
                <td className="py-2 pr-3">{f.order_id ? <Link href={`/admin/orders/${f.order_id}`} className="text-link underline">#{f.order_number}</Link> : "—"}<div className="text-[12px] text-muted">{f.seller}</div></td>
                <td className="py-2 pr-3">{REFUND_KIND[f.kind] ?? f.kind}</td>
                <td className="num py-2 pr-3 text-right">{money(f.amount_cents)}</td>
                <td className="py-2 pr-3">{f.source}{f.reported_by && <div className="text-[12px] text-muted">{f.reported_by}</div>}</td>
                <td className="py-2">{f.note}{f.stripe_ref && <div className="font-mono text-[12px] text-muted">{f.stripe_ref}</div>}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {rows.length === 0 && <p className="py-10 text-center text-muted">Nothing reported yet.</p>}
      </div>
    </div>
  );
}
