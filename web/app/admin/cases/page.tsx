import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { money } from "@/lib/format";
import { when } from "@/components/admin/Badges";
import { CASE_KIND, CASE_STATUS, type CaseRow } from "@/lib/admin-types";

export const dynamic = "force-dynamic";
export const metadata = { title: "Cases" };

const TABS = [["open", "Open"], ["mine", "Assigned to me"], ["resolved_buyer", "Resolved for buyer"], ["resolved_seller", "Resolved for seller"], ["all", "All"]];

export default async function CasesPage({ searchParams }: { searchParams: Promise<{ view?: string }> }) {
  await requireStaff("/admin/cases", "cases.manage");
  const view = (await searchParams).view ?? "open";
  const qs = view === "mine" ? "status=open&mine=1" : `status=${view}`;
  const r = await apiGet<{ cases: CaseRow[] }>(`/v1/admin/cases?${qs}`);
  const rows = r.data?.cases ?? [];
  return (
    <div>
      <h1 className="text-[26px] font-bold">Cases</h1>
      <p className="text-muted">Item-not-received and not-as-described problems. Refunds happen in the seller&apos;s Stripe account; card disputes go through the buyer&apos;s card issuer.</p>
      <nav className="mt-4 flex flex-wrap gap-2 text-[13px]">
        {TABS.map(([v, l]) => (
          <Link key={v} href={`/admin/cases?view=${v}`} className={`rounded-full border px-3 py-1 ${view === v ? "border-ink bg-ink text-white" : "border-line hover:border-ink"}`}>{l}</Link>
        ))}
      </nav>
      <ul className="mt-4 space-y-2">
        {rows.map((c) => (
          <li key={c.id}>
            <Link href={`/admin/cases/${c.id}`} className="block rounded-lg border border-line p-4 text-[14px] hover:border-ink">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-semibold">Case {c.number}</span>
                <span className="rounded-full bg-wash px-2 text-[12px]">{CASE_KIND[c.kind]}</span>
                <span className="text-[13px]">{CASE_STATUS[c.status]}</span>
                <span className="ml-auto num">{money(c.total_cents)}</span>
              </div>
              <p className="mt-1 line-clamp-2">{c.summary}</p>
              <p className="mt-1 text-[12px] text-muted">
                Order #{c.order_number} · {c.buyer} → {c.seller} · {c.assigned_to ? `assigned to ${c.assigned_to}` : "unassigned"} · updated <span suppressHydrationWarning>{when(c.updated_at)}</span>
              </p>
            </Link>
          </li>
        ))}
      </ul>
      {rows.length === 0 && <p className="py-10 text-center text-muted">No cases here.</p>}
    </div>
  );
}
