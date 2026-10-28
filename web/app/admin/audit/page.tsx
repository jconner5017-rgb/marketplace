import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import VerifyChain from "@/components/admin/VerifyChain";

export const dynamic = "force-dynamic";
export const metadata = { title: "Audit log" };

type Entry = {
  id: number; at: string; actor_id: string; actor_email: string; actor_role: string; action: string;
  target_type: string; target_id: string; reason: string; details: Record<string, unknown> | null; ip: string;
};

const GROUPS = [
  ["", "All actions"], ["staff.", "Staff sign-ins"], ["user.", "Members"], ["listing.", "Listings"],
  ["seller.", "Seller review"], ["report.", "Reports"], ["catalog.", "Catalog"], ["fees.", "Fees"], ["case.", "Cases"], ["refund.", "Refunds"], ["feedback.", "Feedback"], ["messages.", "Message views"], ["fraud.", "Fraud alerts"],
  ["announcement.", "Announcements"], ["audit.", "Audit checks"],
];

export default async function AuditPage({ searchParams }: { searchParams: Promise<Record<string, string | undefined>> }) {
  await requireStaff("/admin/audit", "audit.view");
  const sp = await searchParams;
  const qs = new URLSearchParams();
  for (const k of ["action", "actor", "target_type", "target_id", "before"]) if (sp[k]) qs.set(k, sp[k]!);
  const r = await apiGet<{ entries: Entry[] }>("/v1/admin/audit?" + qs.toString());
  const entries = r.data?.entries ?? [];
  const older = entries.length === 100 ? entries[entries.length - 1].id : null;

  return (
    <div>
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-[26px] font-bold">Audit log</h1>
          <p className="text-muted">Every staff action, newest first. Entries can&apos;t be edited or deleted.</p>
        </div>
        <VerifyChain />
      </div>
      <nav className="mt-4 flex flex-wrap gap-2 text-[13px]" aria-label="Filter by action">
        {GROUPS.map(([v, l]) => (
          <Link key={v || "all"} href={v ? `/admin/audit?action=${v}` : "/admin/audit"}
            className={`rounded-full border px-3 py-1 ${(sp.action ?? "") === v ? "border-ink bg-ink text-white" : "border-line hover:border-ink"}`}>
            {l}
          </Link>
        ))}
      </nav>
      {(sp.actor || sp.target_id) && (
        <p className="mt-3 text-[13px]">Filtered to {sp.actor ? "one staff member" : `${sp.target_type ?? "item"} ${sp.target_id}`}. <Link href="/admin/audit" className="text-link underline">Clear</Link></p>
      )}
      <div className="mt-4 overflow-x-auto">
        <table className="w-full min-w-[760px] text-[13px]">
          <thead>
            <tr className="border-b border-line text-left text-muted">
              <th className="py-2 pr-3 font-semibold">When</th>
              <th className="py-2 pr-3 font-semibold">Staff</th>
              <th className="py-2 pr-3 font-semibold">Action</th>
              <th className="py-2 pr-3 font-semibold">Target</th>
              <th className="py-2 pr-3 font-semibold">Reason and details</th>
              <th className="py-2 font-semibold">IP</th>
            </tr>
          </thead>
          <tbody>
            {entries.map((e) => (
              <tr key={e.id} className="border-b border-wash align-top">
                <td className="whitespace-nowrap py-2 pr-3" suppressHydrationWarning>{new Date(e.at).toLocaleString("en-US")}</td>
                <td className="py-2 pr-3">
                  <Link href={`/admin/audit?actor=${e.actor_id}`} className="hover:underline">{e.actor_email}</Link>
                  <div className="text-muted">{e.actor_role}</div>
                </td>
                <td className="py-2 pr-3 font-mono">{e.action}</td>
                <td className="py-2 pr-3">
                  {e.target_id ? (
                    <Link href={`/admin/audit?target_type=${e.target_type}&target_id=${e.target_id}`} className="font-mono hover:underline">
                      {e.target_type} {e.target_id.slice(0, 8)}
                    </Link>
                  ) : "—"}
                </td>
                <td className="py-2 pr-3">
                  {e.reason && <p>{e.reason}</p>}
                  {e.details && Object.keys(e.details).length > 0 && (
                    <p className="font-mono text-muted">{Object.entries(e.details).map(([k, v]) => `${k}: ${JSON.stringify(v)}`).join(" · ")}</p>
                  )}
                </td>
                <td className="py-2 font-mono text-muted">{e.ip}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {entries.length === 0 && <p className="py-8 text-center text-muted">No entries.</p>}
      </div>
      {older && (
        <Link href={`/admin/audit?${new URLSearchParams({ ...Object.fromEntries(qs), before: String(older) })}`} className="mt-4 inline-block text-link underline">
          Older entries
        </Link>
      )}
    </div>
  );
}
