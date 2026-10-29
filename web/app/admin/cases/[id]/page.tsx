import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { money } from "@/lib/format";
import { when } from "@/components/admin/Badges";
import ActionForm from "@/components/admin/ActionForm";
import { CASE_KIND, CASE_STATUS, type CaseRow, type OrderDetail } from "@/lib/admin-types";

export const dynamic = "force-dynamic";
export const metadata = { title: "Case" };

type CaseDetail = CaseRow & { notes: { id: number; author: string; internal: boolean; body: string; at: string }[]; order: OrderDetail };

export default async function CasePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const s = await requireStaff(`/admin/cases/${id}`, "cases.manage");
  const r = await apiGet<{ case: CaseDetail }>(`/v1/admin/cases/${id}`);
  if (!r.data) notFound();
  const c = r.data.case;
  const o = c.order;
  const open = ["open", "waiting_seller", "waiting_buyer"].includes(c.status);
  return (
    <div>
      <Link href="/admin/cases" className="text-[13px] text-link hover:underline">← Cases</Link>
      <h1 className="mt-2 text-[26px] font-bold">Case {c.number}: {CASE_KIND[c.kind]}</h1>
      <p className="text-muted">{CASE_STATUS[c.status]} · opened by {c.opened_by || "staff"} <span suppressHydrationWarning>{when(c.created_at)}</span>
        {c.assigned_to && ` · assigned to ${c.assigned_to}`}</p>
      <p className="mt-3 max-w-[70ch] whitespace-pre-line rounded-lg bg-wash p-3 text-[15px]">{c.summary}</p>
      {c.outcome && <p className="mt-2 text-[14px]"><span className="font-semibold">Outcome:</span> {c.outcome}</p>}

      <div className="mt-6 grid gap-8 xl:grid-cols-[minmax(0,1fr)_360px]">
        <div className="min-w-0 space-y-6">
          <section className="rounded-lg border border-line p-4 text-[14px]">
            <h2 className="font-bold">Order <Link href={`/admin/orders/${o.id}`} className="text-link underline">#{o.number}</Link></h2>
            <p className="mt-1">
              <Link href={`/admin/users/${o.buyer_id}`} className="underline">{o.buyer}</Link> bought from{" "}
              <Link href={`/admin/users/${o.seller_id}`} className="underline">{o.seller}</Link> · <span className="num">{money(o.total_cents)}</span> · paid <span suppressHydrationWarning>{when(o.paid_at)}</span>
            </p>
            <ul className="mt-2 text-muted">{o.lines.map((l, i) => <li key={i}>{l.quantity} × {l.title} at {money(l.unit_cents)}</li>)}</ul>
            <p className="mt-2">
              {o.shipments.length
                ? o.shipments.map((sh) => `${sh.carrier ?? sh.method} ${sh.tracking_number ?? ""} · ${sh.status.replace("_", " ")}${sh.last_event ? ` · ${sh.last_event}` : ""}`).join("; ")
                : o.ship_method === "envelope" ? "Stamped envelope (processing scans only)" : "No shipment recorded"}
            </p>
            {s.permissions.includes("messages.view") && o.thread_ids.map((t) => (
              <Link key={t} href={`/admin/messages/${t}`} className="mt-2 inline-block text-link underline">Read their messages</Link>
            ))}
          </section>
          <section>
            <h2 className="text-[18px] font-bold">Notes</h2>
            <ul className="mt-2 space-y-3 text-[14px]">
              {c.notes.map((n) => (
                <li key={n.id} className={`rounded-lg p-3 ${n.internal ? "bg-wash" : "border border-link/40"}`}>
                  <p className="whitespace-pre-line">{n.body}</p>
                  <p className="mt-1 text-[12px] text-muted">{n.author} · {n.internal ? "staff only" : "shared with buyer and seller"} · <span suppressHydrationWarning>{when(n.at)}</span></p>
                </li>
              ))}
              {c.notes.length === 0 && <li className="text-muted">No notes yet.</li>}
            </ul>
            <div className="mt-3 max-w-[560px]">
              <ActionForm endpoint={`/v1/admin/cases/${c.id}/notes`} done="Note added."
                fields={[
                  { name: "body", label: "Add a note", type: "textarea" },
                  { name: "shared", label: "Share with the buyer and seller (they get a notification)", type: "checkbox" },
                ]} submit="Add note" compact />
            </div>
          </section>
        </div>
        <aside className="space-y-6">
          <section className="rounded-lg border border-line p-4">
            <h2 className="mb-2 font-bold">Status</h2>
            <ActionForm endpoint={`/v1/admin/cases/${c.id}`} method="PATCH" done="Saved."
              fields={[
                { name: "status", label: "Status", type: "select", options: Object.entries(CASE_STATUS), initial: c.status },
                { name: "outcome", label: "Outcome (needed to close)", type: "textarea", initial: c.outcome ?? "",
                  showWhen: { field: "status", notIn: ["open", "waiting_seller", "waiting_buyer"] } },
                { name: "assign_me", key: "assigned_to", label: c.assigned_id === s.user.id ? "Assigned to you" : "Assign to me", type: "checkbox",
                  initial: c.assigned_id === s.user.id, checked: "me", unchecked: c.assigned_id === s.user.id ? "" : undefined },
              ]}
              submit={open ? "Update case" : "Save"} compact />
          </section>
          <p className="text-[12px] text-muted">
            Closing a case tells both members the outcome. Record refunds and chargebacks on the order page.
          </p>
        </aside>
      </div>
    </div>
  );
}
