import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { when } from "@/components/admin/Badges";
import ActionForm from "@/components/admin/ActionForm";
import type { FeedbackRow } from "@/lib/admin-types";

export const dynamic = "force-dynamic";
export const metadata = { title: "Feedback" };

const TABS = [["mediation", "New-seller mediation"], ["negative", "Negative"], ["held", "Cooling off"], ["scrubbed", "Extortion scrubbed"],
  ["reported", "From reported members"], ["", "All"], ["removed", "Removed"]];
const RATING: Record<number, [string, string]> = { 1: ["Positive", "text-ship"], 0: ["Neutral", "text-muted"], [-1]: ["Negative", "text-slab"] };

export default async function FeedbackPage({ searchParams }: { searchParams: Promise<{ filter?: string; user?: string }> }) {
  await requireStaff("/admin/feedback", "feedback.moderate");
  const sp = await searchParams;
  const filter = sp.filter ?? "negative";
  const qs = new URLSearchParams({ filter });
  if (sp.user) qs.set("user", sp.user);
  const r = await apiGet<{ feedback: FeedbackRow[] }>(`/v1/admin/feedback?${qs}`);
  const rows = r.data?.feedback ?? [];
  return (
    <div>
      <h1 className="text-[26px] font-bold">Feedback</h1>
      <p className="text-muted">Remove feedback that&apos;s abusive, off-topic, retaliatory, or left on an order that was refunded or cancelled. Removed feedback no longer counts toward seller levels.</p>
      <div className="mt-4 flex flex-wrap items-center gap-2 text-[13px]">
        {TABS.map(([v, l]) => (
          <Link key={v || "all"} href={`/admin/feedback?filter=${v}`} className={`rounded-full border px-3 py-1 ${filter === v ? "border-ink bg-ink text-white" : "border-line hover:border-ink"}`}>{l}</Link>
        ))}
        <form action="/admin/feedback" className="ml-auto flex gap-2">
          <input type="hidden" name="filter" value={filter} />
          <input name="user" defaultValue={sp.user} placeholder="Member username" className="rounded-md border border-line px-2 py-1" />
          <button className="rounded-full border border-ink px-3">Filter</button>
        </form>
      </div>
      <ul className="mt-4 space-y-3">
        {rows.map((f) => (
          <li key={f.id} className="grid gap-4 rounded-lg border border-line p-4 md:grid-cols-[minmax(0,1fr)_280px]">
            <div className="text-[14px]">
              <p><span className={`font-semibold ${RATING[f.rating][1]}`}>{RATING[f.rating][0]}</span> from{" "}
                <Link href={`/admin/users/${f.from_id}`} className="underline">{f.from}</Link> to <Link href={`/admin/users/${f.to_id}`} className="underline">{f.to}</Link>
                {f.open_reports > 0 && <span className="ml-2 rounded-full bg-slab/10 px-2 text-[12px] text-slab">author reported</span>}</p>
              <p className="mt-1 text-[12px]">
                {f.state !== "published" && <span className="mr-2 rounded-full bg-wash px-2">{f.state === "held" ? "cooling off" : "in mediation"}</span>}
                {!f.visible && <span className="mr-2 rounded-full bg-wash px-2">hidden until both rate</span>}
                {f.auto && <span className="mr-2 rounded-full bg-wash px-2">automatic</span>}
                {f.item_rating != null && <span className="text-muted">Item {f.item_rating}/5 · Shipping {f.shipping_rating}/5 · Communication {f.communication_rating}/5</span>}
              </p>
              <p className={`mt-1 ${f.removed_at ? "text-muted line-through" : ""}`}>{f.comment ? `“${f.comment}”` : <span className="text-muted">No comment</span>}</p>
              {f.scrubbed_comment && <p className="mt-1 text-slab">Removed comment (tried to trade the rating): “{f.scrubbed_comment}”</p>}
              {f.reply && <p className="mt-1 text-muted">Reply: {f.reply}</p>}
              <p className="mt-1 text-[12px] text-muted">Order <Link href={`/admin/orders/${f.order_id}`} className="underline">#{f.order_number}</Link> · <span suppressHydrationWarning>{when(f.created_at)}</span>
                {f.removed_reason && ` · removed: ${f.removed_reason}`}</p>
            </div>
            {f.state === "mediation" && !f.removed_at ? (
              <ActionForm endpoint={`/v1/admin/feedback/${f.id}/moderate`} submit="Save decision" done="Saved." compact
                fields={[
                  { name: "action", label: "Decision", type: "select", options: [["publish", "Let it stand (it counts)"], ["remove", "Remove it"]] },
                  { name: "reason", label: "Reason", placeholder: "Seller refunded in full" },
                ]} />
            ) : (
              <ActionForm endpoint={`/v1/admin/feedback/${f.id}/moderate`} extra={{ action: f.removed_at ? "restore" : "remove" }}
                fields={[{ name: "reason", label: "Reason", placeholder: f.removed_at ? "Removed by mistake" : "Abusive language" }]}
                submit={f.removed_at ? "Restore" : "Remove feedback"} danger={!f.removed_at} done="Saved." compact />
            )}
          </li>
        ))}
      </ul>
      {rows.length === 0 && <p className="py-10 text-center text-muted">No feedback here yet. Feedback starts once orders are delivered.</p>}
    </div>
  );
}
