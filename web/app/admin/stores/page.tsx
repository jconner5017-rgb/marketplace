import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { StatusBadge, when } from "@/components/admin/Badges";
import ActionForm from "@/components/admin/ActionForm";
import type { Store } from "@/lib/types";

export const dynamic = "force-dynamic";
export const metadata = { title: "Stores" };

const TABS = [["pending", "Waiting for review"], ["active", "Open"], ["rejected", "Rejected"], ["suspended", "Paused"], ["", "All"]];
const TYPE: Record<string, string> = { llc: "LLC", corporation: "Corporation", partnership: "Partnership", sole_proprietor: "Sole proprietor", other: "Other" };

export default async function StoresPage({ searchParams }: { searchParams: Promise<{ status?: string }> }) {
  await requireStaff("/admin/stores", "sellers.review");
  const status = (await searchParams).status ?? "pending";
  const r = await apiGet<{ stores: Store[] }>(`/v1/admin/stores?status=${encodeURIComponent(status)}`);
  const rows = r.data?.stores ?? [];
  return (
    <div>
      <h1 className="text-[26px] font-bold">Business stores</h1>
      <p className="max-w-[700px] text-muted">
        Stores are free for registered businesses. Check the business is real (state business search, website, show listings)
        before opening it. Sellers are told the decision in their notifications.
      </p>
      <nav className="mt-4 flex flex-wrap gap-2 text-[13px]">
        {TABS.map(([v, l]) => (
          <Link key={v || "all"} href={`/admin/stores?status=${v}`}
            className={`rounded-full border px-3 py-1 ${status === v ? "border-ink bg-ink text-white" : "border-line hover:border-ink"}`}>{l}</Link>
        ))}
      </nav>
      <ul className="mt-4 space-y-3">
        {rows.map((s) => (
          <li key={s.username} className="grid gap-4 rounded-lg border border-line p-4 md:grid-cols-[minmax(0,1fr)_320px]">
            <div className="min-w-0 text-[14px]">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-[16px] font-semibold">{s.name}</span>
                <StatusBadge status={s.status === "active" ? "approved" : s.status} />
              </div>
              <p className="text-muted">
                slabsnsingles.com/store/{s.slug} · seller <Link href={`/search?seller=${encodeURIComponent(s.username)}`} className="text-link underline">{s.username}</Link> ·
                applied <span suppressHydrationWarning>{when(s.created_at)}</span>
              </p>
              <dl className="mt-2 grid grid-cols-[140px_1fr] gap-x-3 gap-y-0.5">
                <dt className="text-muted">Legal name</dt><dd>{s.legal_name}</dd>
                <dt className="text-muted">Business type</dt><dd>{TYPE[s.business_type ?? ""] ?? s.business_type}</dd>
                <dt className="text-muted">Website</dt><dd>{s.website ? <a href={s.website} target="_blank" rel="noopener noreferrer nofollow" className="text-link underline">{s.website}</a> : "—"}</dd>
                <dt className="text-muted">Their note</dt><dd className="whitespace-pre-line">{s.business_note || "—"}</dd>
                <dt className="text-muted">Listings / followers</dt><dd className="num">{s.listings} / {s.followers}</dd>
              </dl>
              {s.review_note && <p className="mt-1 text-muted">Our note: {s.review_note}</p>}
              {s.status === "active" && <p className="mt-1"><Link href={`/store/${s.slug}`} className="text-link underline">Open store page</Link></p>}
            </div>
            <ActionForm
              endpoint={`/v1/admin/stores/${encodeURIComponent(s.username)}/review`}
              fields={[
                { name: "status", label: "Decision", type: "select", options: [["active", "Open the store"], ["rejected", "Reject (seller can fix and resend)"], ["suspended", "Pause (hides the store)"]] },
                { name: "note", label: "Note to the seller (required unless opening)", type: "text" },
                { name: "reason", label: "Reason for the audit log", type: "text" },
              ]}
              submit="Save"
              done="Saved."
              compact
            />
          </li>
        ))}
      </ul>
      {rows.length === 0 && <p className="py-8 text-center text-muted">No stores here.</p>}
    </div>
  );
}
