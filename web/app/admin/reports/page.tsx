import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { StatusBadge, when } from "@/components/admin/Badges";
import ActionForm from "@/components/admin/ActionForm";

export const dynamic = "force-dynamic";
export const metadata = { title: "Reports" };

type Report = {
  id: string; target_type: string; target_id: string; target_user_id: string | null; target_user: string; target_summary: string;
  reason: string; details: string; status: string; reporter: string; reporter_id: string | null; open_on_target: number;
  resolved_by: string; resolved_at: string | null; resolution_note: string; created_at: string;
};

const REASON: Record<string, string> = {
  counterfeit: "Counterfeit", prohibited: "Prohibited item", keyword_spam: "Keyword spam", misdescribed: "Not as described",
  off_platform: "Off-site payment", abuse: "Abuse", shill_bidding: "Suspicious bidding", other: "Other",
};

function targetLink(r: Report) {
  if (r.target_type === "listing") return `/admin/listings?q=${r.target_id}`;
  if (r.target_type === "user") return `/admin/users/${r.target_id}`;
  if (r.target_user_id) return `/admin/users/${r.target_user_id}`;
  return "#";
}

export default async function ReportsPage({ searchParams }: { searchParams: Promise<{ status?: string; type?: string }> }) {
  const s = await requireStaff("/admin/reports", "reports.manage");
  const sp = await searchParams;
  const status = sp.status ?? "open";
  const qs = new URLSearchParams({ status });
  if (sp.type) qs.set("type", sp.type);
  const r = await apiGet<{ reports: Report[] }>("/v1/admin/reports?" + qs.toString());
  const rows = r.data?.reports ?? [];
  const tab = (k: string, v: string, l: string) => {
    const on = (k === "status" ? status : sp.type ?? "") === v;
    const n = new URLSearchParams({ status, ...(sp.type ? { type: sp.type } : {}), [k]: v });
    if (!v) n.delete(k);
    return <Link key={k + v} href={`/admin/reports?${n}`} className={`rounded-full border px-3 py-1 ${on ? "border-ink bg-ink text-white" : "border-line hover:border-ink"}`}>{l}</Link>;
  };
  return (
    <div>
      <h1 className="text-[26px] font-bold">Reports</h1>
      <p className="text-muted">Reports from members. Open ones are oldest first. Members aren&apos;t told who reported them.</p>
      <div className="mt-4 flex flex-wrap gap-2 text-[13px]">
        {tab("status", "open", "Open")}{tab("status", "actioned", "Actioned")}{tab("status", "dismissed", "Dismissed")}{tab("status", "all", "All")}
        <span className="mx-2 border-l border-line" />
        {tab("type", "", "Everything")}{tab("type", "listing", "Listings")}{tab("type", "user", "Members")}{tab("type", "message", "Messages")}{tab("type", "offer", "Offers")}
      </div>
      <ul className="mt-4 space-y-3">
        {rows.map((x) => (
          <li key={x.id} className="grid gap-4 rounded-lg border border-line p-4 lg:grid-cols-[minmax(0,1fr)_300px]">
            <div className="min-w-0 text-[14px]">
              <div className="flex flex-wrap items-center gap-2">
                <span className="rounded-full bg-slab/10 px-2 py-0.5 text-[12px] font-semibold text-slab">{REASON[x.reason] ?? x.reason}</span>
                <span className="text-[12px] uppercase tracking-wide text-muted">{x.target_type}</span>
                {x.status !== "open" && <StatusBadge status={x.status} />}
                {x.open_on_target > 1 && <span className="text-[12px] font-semibold">{x.open_on_target} open reports on this</span>}
              </div>
              <Link href={targetLink(x)} className="mt-1 block font-semibold text-link hover:underline">{x.target_summary}</Link>
              {x.target_user && (
                <p className="text-muted">
                  Member: {x.target_user_id ? <Link href={`/admin/users/${x.target_user_id}`} className="underline">{x.target_user}</Link> : x.target_user}
                </p>
              )}
              {x.details && <p className="mt-1 whitespace-pre-line">“{x.details}”</p>}
              <p className="mt-1 text-[12px] text-muted">
                Reported by {x.reporter_id ? <Link href={`/admin/users/${x.reporter_id}`} className="underline">{x.reporter}</Link> : x.reporter} · <span suppressHydrationWarning>{when(x.created_at)}</span>
              </p>
              {x.status !== "open" && (
                <p className="mt-1 text-[12px] text-muted">{x.status} by {x.resolved_by} · <span suppressHydrationWarning>{when(x.resolved_at)}</span>: {x.resolution_note}</p>
              )}
            </div>
            {x.status === "open" && (
              <div>
                <ActionForm
                  endpoint={`/v1/admin/reports/${x.id}/resolve`}
                  fields={[
                    { name: "status", label: "Outcome", type: "select", options: [["actioned", "Actioned (we did something)"], ["dismissed", "Dismissed (nothing wrong)"]] },
                    { name: "note", label: "Note for the record", type: "text", placeholder: "Removed listing; warned seller" },
                    { name: "all", label: `Close all ${x.open_on_target} open reports on this`, type: "checkbox", initial: x.open_on_target > 1, showIf: () => x.open_on_target > 1 },
                  ]}
                  submit="Close report"
                  done="Closed."
                  compact
                />
                <p className="mt-2 text-[12px] text-muted">
                  Take action first from the {x.target_type === "listing" ? "listing" : "member"} page
                  {s.permissions.includes("listings.moderate") ? "" : " (ask a moderator to remove listings)"}.
                </p>
              </div>
            )}
          </li>
        ))}
      </ul>
      {rows.length === 0 && <p className="py-10 text-center text-muted">Nothing here. {status === "open" && "The queue is clear."}</p>}
    </div>
  );
}
