import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { when } from "@/components/admin/Badges";
import ActionForm from "@/components/admin/ActionForm";

export const dynamic = "force-dynamic";
export const metadata = { title: "Appeals" };

type Appeal = {
  id: string; user_id: string; username: string; email: string; about: string; ref: string; message: string;
  status: "open" | "upheld" | "overturned"; decision_note: string | null; decided_by: string | null;
  created_at: string; decided_at: string | null; decision: string;
};

const ABOUT: Record<string, string> = {
  warning: "Warning", demotion: "Seller level lowered", listing: "Listing taken down",
  freeze: "Account frozen", suspension: "Account suspended", ban: "Account banned",
};
const TABS = [["open", "Open"], ["upheld", "Upheld"], ["overturned", "Overturned"]] as const;

export default async function AppealsPage({ searchParams }: { searchParams: Promise<{ status?: string }> }) {
  const sp = await searchParams;
  const status = TABS.some(([k]) => k === sp.status) ? sp.status! : "open";
  await requireStaff("/admin/appeals", "users.restrict");
  const r = await apiGet<{ appeals: Appeal[] }>(`/v1/admin/appeals?status=${status}`);
  const list = r.data?.appeals ?? [];
  return (
    <div>
      <h1 className="text-[26px] font-bold">Appeals</h1>
      <p className="text-muted">Each decision can be appealed once. Overturning undoes it: the account is reinstated, the warning withdrawn, the seller level or listing restored.</p>
      <nav aria-label="Appeal status" className="mt-3 flex gap-1 border-b border-line text-[14px]">
        {TABS.map(([k, l]) => (
          <Link key={k} href={`/admin/appeals?status=${k}`} aria-current={k === status ? "page" : undefined}
            className={`border-b-2 px-3 py-2 font-semibold ${k === status ? "border-ink" : "border-transparent text-muted hover:text-ink"}`}>{l}</Link>
        ))}
      </nav>
      {list.length === 0 ? <p className="py-10 text-center text-muted">No {status} appeals.</p> : (
        <ul>
          {list.map((a) => (
            <li key={a.id} className="border-b border-line py-4 text-[14px]">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <p><span className="font-semibold">{ABOUT[a.about] ?? a.about}</span> · <Link href={`/admin/users/${a.user_id}`} className="text-link underline">{a.username}</Link></p>
                <span className="text-muted" suppressHydrationWarning>{when(a.created_at)}</span>
              </div>
              <p className="mt-1 text-muted">Decision: {a.decision || "—"}</p>
              {a.about === "listing" && <Link href={`/item/${a.ref}`} className="text-[13px] text-link underline">View listing</Link>}
              <blockquote className="mt-2 whitespace-pre-line border-l-4 border-line pl-3">{a.message}</blockquote>
              {a.status === "open" ? (
                <div className="mt-3 max-w-[520px]">
                  <ActionForm endpoint={`/v1/admin/appeals/${a.id}`} submit="Record decision" done="Decided." compact
                    fields={[
                      { name: "decision", label: "Decision", type: "select", options: [["upheld", "Uphold (decision stands)"], ["overturned", "Overturn (undo it)"]] },
                      { name: "note", label: "Note to the member", type: "textarea", hint: "They see this; it's also saved in the audit log." },
                    ]} />
                </div>
              ) : (
                <p className="mt-2"><span className="font-semibold capitalize">{a.status}</span> by {a.decided_by} · <span suppressHydrationWarning>{when(a.decided_at)}</span>{a.decision_note ? `: ${a.decision_note}` : ""}</p>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
