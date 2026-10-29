import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { when } from "@/components/admin/Badges";
import ActionForm from "@/components/admin/ActionForm";
import RunChecks from "@/components/admin/RunChecks";

export const dynamic = "force-dynamic";
export const metadata = { title: "Fraud alerts" };

type Alert = {
  id: string; kind: string; severity: "low" | "medium" | "high"; subject_user_id: string | null; subject: string; subject_status: string;
  listing_id: string | null; listing_title: string; summary: string; details: Record<string, unknown>; status: string;
  reviewed_by: string; reviewed_at: string | null; review_note: string | null; created_at: string;
};

const KIND: Record<string, string> = {
  high_value_spike: "High-value spike", takeover_ips: "Possible account takeover", failed_logins: "Password guessing",
  off_platform: "Off-site payment attempts", repeat_reports: "Repeated reports", staff_mfa_locked: "Staff code lockout",
  shill_shared_ip: "Shill bidding: shared network", shill_pattern: "Shill bidding: pattern", bid_burst: "Bid burst", unpaid_items: "Unpaid items",
  admin_ip_blocked: "Back office from an unapproved network", banned_identifier: "Matches a banned member",
};
const SEV: Record<string, string> = { high: "bg-slab text-white", medium: "bg-[#fff4e0] text-[#8a5300]", low: "bg-wash text-muted" };

export default async function FraudPage({ searchParams }: { searchParams: Promise<{ status?: string }> }) {
  const s = await requireStaff("/admin/fraud", "reports.manage");
  const status = (await searchParams).status ?? "open";
  const r = await apiGet<{ alerts: Alert[] }>(`/v1/admin/fraud?status=${status}`);
  const rows = r.data?.alerts ?? [];
  const canAct = s.permissions.includes("users.restrict");
  return (
    <div>
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-[26px] font-bold">Fraud alerts</h1>
          <p className="text-muted">Raised automatically every 10 minutes. Review the member, act from their page, then mark the alert.</p>
        </div>
        {canAct && <RunChecks />}
      </div>
      <nav className="mt-4 flex flex-wrap gap-2 text-[13px]">
        {[["open", "Open"], ["reviewed", "Reviewed"], ["dismissed", "Dismissed"], ["all", "All"]].map(([v, l]) => (
          <Link key={v} href={`/admin/fraud?status=${v}`} className={`rounded-full border px-3 py-1 ${status === v ? "border-ink bg-ink text-white" : "border-line hover:border-ink"}`}>{l}</Link>
        ))}
      </nav>
      <ul className="mt-4 space-y-3">
        {rows.map((a) => (
          <li key={a.id} className="grid gap-4 rounded-lg border border-line p-4 lg:grid-cols-[minmax(0,1fr)_280px]">
            <div className="min-w-0 text-[14px]">
              <div className="flex flex-wrap items-center gap-2">
                <span className={`rounded-full px-2 py-0.5 text-[12px] font-semibold uppercase ${SEV[a.severity]}`}>{a.severity}</span>
                <span className="font-semibold">{KIND[a.kind] ?? a.kind}</span>
                <span className="text-[12px] text-muted" suppressHydrationWarning>{when(a.created_at)}</span>
              </div>
              <p className="mt-1">{a.summary}</p>
              <p className="mt-1 text-muted">
                {a.subject_user_id && <>Member: <Link href={`/admin/users/${a.subject_user_id}`} className="text-link underline">{a.subject}</Link> ({a.subject_status})</>}
                {a.listing_id && <> · listing <Link href={`/admin/listings?q=${a.listing_id}`} className="underline">{a.listing_title}</Link></>}
              </p>
              <p className="mt-1 break-all font-mono text-[12px] text-muted">{Object.entries(a.details ?? {}).map(([k, v]) => `${k}: ${JSON.stringify(v)}`).join(" · ")}</p>
              {a.status !== "open" && <p className="mt-1 text-[12px] text-muted">{a.status} by {a.reviewed_by}: {a.review_note}</p>}
            </div>
            {a.status === "open" && canAct && (
              <ActionForm endpoint={`/v1/admin/fraud/${a.id}/review`}
                fields={[
                  { name: "status", label: "Outcome", type: "select", options: [["reviewed", "Reviewed, action taken"], ["dismissed", "Dismissed, nothing wrong"]] },
                  { name: "note", label: "Note", placeholder: "Froze account, opened case" },
                ]} submit="Mark" done="Saved." compact />
            )}
          </li>
        ))}
      </ul>
      {rows.length === 0 && <p className="py-10 text-center text-muted">No alerts here.</p>}
    </div>
  );
}
