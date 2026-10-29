import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { money } from "@/lib/format";
import { when } from "@/components/admin/Badges";
import { ORDER_STATUS } from "@/components/admin/OrderBits";
import ActionForm from "@/components/admin/ActionForm";
import { CASE_KIND, CASE_STATUS, REFUND_KIND, type OrderDetail } from "@/lib/admin-types";

type LedgerRow = { kind: string; amount_cents: number; stripe_object: string; note: string; created_at: string };
const LEDGER_KIND: Record<string, string> = {
  charge: "Buyer paid", application_fee: "Marketplace fee", discount: "Multi-card discount", label_cost: "Shipping label",
  refund: "Refund", fee_refund: "Fee returned", chargeback_opened: "Chargeback opened", chargeback_won: "Chargeback won",
  chargeback_lost: "Chargeback lost", adjustment: "Adjustment",
};

export const dynamic = "force-dynamic";
export const metadata = { title: "Order" };

export default async function OrderPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const s = await requireStaff(`/admin/orders/${id}`, "orders.view");
  const r = await apiGet<{ order: OrderDetail; ledger: LedgerRow[] }>(`/v1/admin/orders/${id}`);
  if (!r.data) notFound();
  const o = r.data.order;
  const ledger = r.data.ledger ?? [];
  const cases = s.permissions.includes("cases.manage");
  const dl = "grid grid-cols-[150px_1fr] gap-y-1 text-[14px]";
  return (
    <div>
      <Link href="/admin/orders" className="text-[13px] text-link hover:underline">← Orders</Link>
      <h1 className="mt-2 text-[26px] font-bold">Order #{o.number}</h1>
      <p className={o.late ? "font-semibold text-slab" : "text-muted"}>
        {o.late ? "Late to ship · " : ""}{ORDER_STATUS[o.status] ?? o.status} · placed <span suppressHydrationWarning>{when(o.created_at)}</span>
      </p>
      <div className="mt-6 grid gap-8 xl:grid-cols-[minmax(0,1fr)_360px]">
        <div className="min-w-0 space-y-8">
          <section>
            <h2 className="text-[18px] font-bold">Items</h2>
            <ul className="mt-2 divide-y divide-wash text-[14px]">
              {o.lines.map((l, i) => (
                <li key={i} className="flex justify-between gap-3 py-2">
                  <span>{l.listing_id ? <Link href={`/item/${l.listing_id}`} className="text-link hover:underline">{l.title}</Link> : l.title}
                    <span className="text-muted"> · {l.source === "auction" ? "won auction" : l.source === "offer" ? "accepted offer" : "Buy It Now"}{l.listing_version ? ` · listing v${l.listing_version}` : ""}</span></span>
                  <span className="num">{l.quantity} × {money(l.unit_cents)}</span>
                </li>
              ))}
            </ul>
            <dl className={`mt-3 ${dl}`}>
              <dt className="text-muted">Subtotal</dt><dd className="num">{money(o.subtotal_cents)}</dd>
              {o.discount_cents > 0 && <><dt className="text-muted">Multi-card discount</dt><dd className="num">−{money(o.discount_cents)}</dd></>}
              <dt className="text-muted">Total paid</dt><dd className="num font-semibold">{money(o.total_cents)}</dd>
              <dt className="text-muted">Our fee</dt><dd className="num">{money(o.fee_cents)}</dd>
              {o.refunded_cents > 0 && <><dt className="text-muted">Refunded</dt><dd className="num text-slab">{money(o.refunded_cents)}</dd></>}
            </dl>
          </section>
          <section>
            <h2 className="text-[18px] font-bold">People and payment</h2>
            <dl className={`mt-2 ${dl}`}>
              <dt className="text-muted">Buyer</dt><dd><Link href={`/admin/users/${o.buyer_id}`} className="text-link underline">{o.buyer}</Link></dd>
              <dt className="text-muted">Seller</dt><dd><Link href={`/admin/users/${o.seller_id}`} className="text-link underline">{o.seller}</Link></dd>
              <dt className="text-muted">Ship to</dt><dd>{[o.ship_to.name, o.ship_to.city, o.ship_to.state].filter(Boolean).join(", ") || "—"}</dd>
              <dt className="text-muted">Paid</dt><dd suppressHydrationWarning>{when(o.paid_at)}</dd>
              <dt className="text-muted">Stripe account</dt><dd className="font-mono text-[13px]">{o.stripe_account ?? "—"}</dd>
              <dt className="text-muted">Payment</dt><dd className="font-mono text-[13px]">{o.stripe_payment_intent ?? "—"}</dd>
              <dt className="text-muted">Charge</dt><dd className="font-mono text-[13px]">{o.stripe_charge ?? "—"}</dd>
              {o.cancel_reason && <><dt className="text-muted">Cancelled</dt><dd>{o.cancel_reason}</dd></>}
            </dl>
          </section>
          <section>
            <h2 className="text-[18px] font-bold">Shipping</h2>
            <p className="text-[14px] text-muted">{o.ship_method === "envelope" ? "Stamped envelope" : "Label with tracking"} · ship by <span suppressHydrationWarning>{when(o.ship_by)}</span>
              {o.shipped_at && <> · shipped <span suppressHydrationWarning>{when(o.shipped_at)}</span></>}</p>
            <ul className="mt-2 space-y-2 text-[14px]">
              {o.shipments.map((sh) => (
                <li key={sh.id} className="rounded-md border border-line p-3">
                  <p className="font-semibold">{[sh.carrier, sh.service].filter(Boolean).join(" ") || sh.method} · {sh.status.replace("_", " ")}</p>
                  {sh.tracking_number && <p className="font-mono text-[13px]">{sh.tracking_number}</p>}
                  {sh.last_event && <p className="text-muted">{sh.last_event} · <span suppressHydrationWarning>{when(sh.last_event_at)}</span></p>}
                  {sh.label_cost_cents != null && <p className="text-muted">Label cost {money(sh.label_cost_cents)}</p>}
                </li>
              ))}
              {o.shipments.length === 0 && <li className="text-muted">No shipment yet.</li>}
            </ul>
          </section>
          <section>
            <h2 className="text-[18px] font-bold">Money ledger</h2>
            <p className="text-[13px] text-muted">Every money event from Stripe, in order. Entries are never changed; corrections are new rows.</p>
            <div className="mt-2 overflow-x-auto">
              <table className="w-full min-w-[520px] text-[13px]">
                <thead><tr className="border-b border-line text-left text-muted"><th className="py-1.5 pr-3">When</th><th className="py-1.5 pr-3">Event</th><th className="py-1.5 pr-3 text-right">Amount</th><th className="py-1.5">Stripe</th></tr></thead>
                <tbody>
                  {ledger.map((e, i) => (
                    <tr key={i} className="border-b border-wash">
                      <td className="py-1.5 pr-3 whitespace-nowrap" suppressHydrationWarning>{when(e.created_at)}</td>
                      <td className="py-1.5 pr-3">{LEDGER_KIND[e.kind] ?? e.kind}{e.note ? <span className="text-muted"> · {e.note}</span> : null}</td>
                      <td className="num py-1.5 pr-3 text-right">{money(e.amount_cents)}</td>
                      <td className="max-w-[200px] truncate py-1.5 font-mono" title={e.stripe_object}>{e.stripe_object}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {ledger.length === 0 && <p className="py-2 text-[13px] text-muted">No money events yet.</p>}
            </div>
          </section>
          <section>
            <h2 className="text-[18px] font-bold">Refunds and chargebacks</h2>
            <ul className="mt-2 space-y-1 text-[14px]">
              {o.refunds.map((f) => (
                <li key={f.id}>{REFUND_KIND[f.kind] ?? f.kind} · <span className="num">{money(f.amount_cents)}</span> · reported by {f.source}{f.reported_by && ` (${f.reported_by})`}
                  {f.stripe_ref && <span className="font-mono text-[13px]"> · {f.stripe_ref}</span>}{f.note && <span className="text-muted"> · {f.note}</span>}</li>
              ))}
              {o.refunds.length === 0 && <li className="text-muted">None reported.</li>}
            </ul>
          </section>
          <section>
            <h2 className="text-[18px] font-bold">Feedback</h2>
            <ul className="mt-2 space-y-1 text-[14px]">
              {o.feedback.map((f) => (
                <li key={f.id} className={f.removed_at ? "text-muted line-through" : ""}>
                  {f.rating > 0 ? "Positive" : f.rating < 0 ? "Negative" : "Neutral"} from {f.from}: “{f.comment}”
                </li>
              ))}
              {o.feedback.length === 0 && <li className="text-muted">None yet.</li>}
            </ul>
          </section>
        </div>
        <aside className="space-y-6">
          <section className="rounded-lg border border-line p-4">
            <h2 className="font-bold">Cases</h2>
            <ul className="mt-2 space-y-2 text-[14px]">
              {o.cases.map((c) => (
                <li key={c.id}><Link href={`/admin/cases/${c.id}`} className="text-link underline">Case {c.number}</Link> · {CASE_KIND[c.kind]} · {CASE_STATUS[c.status]}</li>
              ))}
              {o.cases.length === 0 && <li className="text-muted">No cases.</li>}
            </ul>
            {cases && (
              <details className="mt-3">
                <summary className="cursor-pointer text-[14px] font-semibold text-link">Open a case</summary>
                <div className="mt-2">
                  <ActionForm endpoint="/v1/admin/cases" extra={{ order: o.id }} done="Case opened."
                    fields={[
                      { name: "kind", label: "Problem", type: "select", options: Object.entries(CASE_KIND) },
                      { name: "summary", label: "What happened", type: "textarea" },
                    ]} submit="Open case" compact />
                </div>
              </details>
            )}
          </section>
          {cases && (
            <section className="rounded-lg border border-line p-4">
              <h2 className="mb-1 font-bold">Record a refund or chargeback</h2>
              <p className="mb-2 text-[12px] text-muted">Sellers refund from their own Stripe account. This records what happened.</p>
              <ActionForm endpoint="/v1/admin/refunds" extra={{ order: o.id }} done="Recorded."
                fields={[
                  { name: "kind", label: "What", type: "select", options: Object.entries(REFUND_KIND) },
                  { name: "amount", label: "Amount ($)", initial: (o.total_cents / 100).toFixed(2) },
                  { name: "stripe_ref", label: "Stripe refund or dispute ID (optional)", placeholder: "re_… or dp_…" },
                  { name: "note", label: "Note", placeholder: "Seller refunded after case 5003" },
                ]}
                transform={(v) => ({ kind: v.kind, amount_cents: Math.round(Number(String(v.amount).replace(/[$,]/g, "")) * 100), stripe_ref: v.stripe_ref, note: v.note })}
                submit="Record" compact />
            </section>
          )}
          {s.permissions.includes("messages.view") && o.thread_ids.length > 0 && (
            <section className="rounded-lg border border-line p-4 text-[14px]">
              <h2 className="font-bold">Buyer–seller messages</h2>
              {o.thread_ids.map((t) => <Link key={t} href={`/admin/messages/${t}`} className="mt-1 block text-link underline">Open conversation</Link>)}
            </section>
          )}
        </aside>
      </div>
    </div>
  );
}
