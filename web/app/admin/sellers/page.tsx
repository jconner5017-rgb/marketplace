import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { StatusBadge, when } from "@/components/admin/Badges";
import ActionForm from "@/components/admin/ActionForm";

export const dynamic = "force-dynamic";
export const metadata = { title: "Seller review" };

type Row = {
  user_id: string; username: string; email: string; status: string; stripe_account_id: string; charges_enabled: boolean;
  details_submitted: boolean; tier: number; review_status: string; review_note: string | null; reviewed_at: string | null;
  active_listings: number; since: string;
};

const TABS = [["pending", "Waiting for review"], ["approved", "Approved"], ["rejected", "Rejected"], ["", "All"]];

export default async function SellersPage({ searchParams }: { searchParams: Promise<{ review?: string }> }) {
  await requireStaff("/admin/sellers", "sellers.review");
  const review = (await searchParams).review ?? "pending";
  const r = await apiGet<{ sellers: Row[] }>(`/v1/admin/sellers?review=${review}`);
  const rows = r.data?.sellers ?? [];
  return (
    <div>
      <h1 className="text-[26px] font-bold">Seller review</h1>
      <p className="text-muted">Stripe verifies each seller&apos;s identity and business details. This is our own approval on top of that. Rejected sellers&apos; listings are hidden.</p>
      <nav className="mt-4 flex flex-wrap gap-2 text-[13px]">
        {TABS.map(([v, l]) => (
          <Link key={v || "all"} href={`/admin/sellers?review=${v}`}
            className={`rounded-full border px-3 py-1 ${review === v ? "border-ink bg-ink text-white" : "border-line hover:border-ink"}`}>{l}</Link>
        ))}
      </nav>
      <ul className="mt-4 space-y-3">
        {rows.map((s) => (
          <li key={s.user_id} className="grid gap-4 rounded-lg border border-line p-4 md:grid-cols-[minmax(0,1fr)_320px]">
            <div className="min-w-0 text-[14px]">
              <div className="flex flex-wrap items-center gap-2">
                <Link href={`/admin/users/${s.user_id}`} className="text-[16px] font-semibold text-link hover:underline">{s.username}</Link>
                <StatusBadge status={s.review_status} />
                {s.status !== "active" && <StatusBadge status={s.status} />}
              </div>
              <p className="text-muted">{s.email} · seller since <span suppressHydrationWarning>{when(s.since)}</span></p>
              <p className="mt-1">
                Stripe: {s.details_submitted ? "details submitted" : "details not finished"} · {s.charges_enabled ? <span className="text-ship">taking payments</span> : <span className="text-slab">can&apos;t take payments</span>}
              </p>
              <p>Level {s.tier} · {s.active_listings} active listings · <Link href={`/search?seller=${encodeURIComponent(s.username)}`} className="text-link underline">view listings</Link></p>
              {s.review_note && <p className="mt-1 text-muted">Note: {s.review_note}</p>}
            </div>
            <ActionForm
              endpoint={`/v1/admin/sellers/${s.user_id}/review`}
              fields={[
                { name: "decision", label: "Decision", type: "select", options: [["approve", "Approve"], ["reject", "Reject (hides listings)"], ["pending", "Back to pending"]] },
                { name: "note", label: "Note (required to reject)", type: "text" },
              ]}
              submit="Save"
              done="Saved."
              compact
            />
          </li>
        ))}
      </ul>
      {rows.length === 0 && <p className="py-8 text-center text-muted">No sellers here.</p>}
    </div>
  );
}
