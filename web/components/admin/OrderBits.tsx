import Link from "next/link";
import { money } from "@/lib/format";
import { StatusBadge, when } from "./Badges";

export type OrderRow = {
  id: string; number: number; status: string; buyer: string; buyer_id: string; seller: string; seller_id: string; items: number;
  total_cents: number; fee_cents: number; refunded_cents: number; ship_method: string; ship_by: string | null; created_at: string;
  paid_at: string | null; shipped_at: string | null; open_cases: number; tracking: string | null; ship_status: string | null; late: boolean;
};

export const ORDER_STATUS: Record<string, string> = {
  pending_payment: "Awaiting payment", paid: "Paid, not shipped", shipped: "Shipped", delivered: "Delivered",
  cancelled: "Cancelled", refunded: "Refunded", partially_refunded: "Partly refunded",
};

/** Orders as cards on phones and a table on wider screens. */
export function OrderList({ rows }: { rows: OrderRow[] }) {
  if (rows.length === 0) {
    return <p className="py-10 text-center text-muted">No orders yet. They appear here as soon as checkout is switched on.</p>;
  }
  return (
    <>
      <ul className="space-y-2 md:hidden">
        {rows.map((o) => (
          <li key={o.id}>
            <Link href={`/admin/orders/${o.id}`} className="block rounded-lg border border-line p-3 text-[14px] hover:border-ink">
              <div className="flex justify-between gap-2"><span className="font-semibold">#{o.number}</span><span className="num font-semibold">{money(o.total_cents)}</span></div>
              <p className="text-muted">{o.buyer} → {o.seller} · {o.items} cards</p>
              <p className={o.late ? "font-semibold text-slab" : "text-muted"}>{o.late ? "Late to ship" : ORDER_STATUS[o.status] ?? o.status}{o.open_cases ? ` · ${o.open_cases} open case` : ""}</p>
            </Link>
          </li>
        ))}
      </ul>
      <div className="hidden overflow-x-auto md:block">
        <table className="w-full text-[14px]">
          <thead>
            <tr className="border-b border-line text-left text-[13px] text-muted">
              <th className="py-2 pr-3 font-semibold">Order</th><th className="py-2 pr-3 font-semibold">Buyer → seller</th>
              <th className="py-2 pr-3 font-semibold">Status</th><th className="py-2 pr-3 text-right font-semibold">Total</th>
              <th className="py-2 pr-3 text-right font-semibold">Our fee</th><th className="py-2 pr-3 font-semibold">Shipping</th>
              <th className="py-2 font-semibold">Placed</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((o) => (
              <tr key={o.id} className="border-b border-wash">
                <td className="py-2 pr-3"><Link href={`/admin/orders/${o.id}`} className="font-semibold text-link hover:underline">#{o.number}</Link>
                  {o.open_cases > 0 && <span className="ml-2 rounded-full bg-slab/10 px-2 text-[12px] text-slab">case</span>}</td>
                <td className="py-2 pr-3">{o.buyer} → {o.seller}<div className="text-[12px] text-muted">{o.items} cards</div></td>
                <td className="py-2 pr-3">{ORDER_STATUS[o.status] ?? o.status}</td>
                <td className="num py-2 pr-3 text-right">{money(o.total_cents)}{o.refunded_cents > 0 && <div className="text-[12px] text-slab">−{money(o.refunded_cents)}</div>}</td>
                <td className="num py-2 pr-3 text-right">{money(o.fee_cents)}</td>
                <td className="py-2 pr-3 text-[13px]">
                  {o.late ? <span className="font-semibold text-slab">Late (due <span suppressHydrationWarning>{when(o.ship_by)}</span>)</span>
                    : o.tracking ? <>{o.ship_status?.replace("_", " ")} · <span className="font-mono">{o.tracking}</span></>
                    : o.ship_method === "envelope" ? "Stamped envelope" : "Label"}
                </td>
                <td className="py-2 text-[13px]" suppressHydrationWarning>{when(o.created_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}

export { StatusBadge };
