import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { when } from "@/components/admin/Badges";
import ActionForm from "@/components/admin/ActionForm";

export const dynamic = "force-dynamic";
export const metadata = { title: "Outside feedback" };

type Claim = { id: string; user_id: string; username: string; platform: string; profile_url: string; feedback_count: number;
  positive_percent: number; verify_code: string; status: string; review_note: string | null; created_at: string };
const NAMES: Record<string, string> = { ebay: "eBay", etsy: "Etsy", discogs: "Discogs", tcgplayer: "TCGplayer", comc: "COMC", other: "Other" };

export default async function OutsidePage({ searchParams }: { searchParams: Promise<{ status?: string }> }) {
  await requireStaff("/admin/outside-feedback", "sellers.review");
  const status = (await searchParams).status ?? "pending";
  const r = await apiGet<{ claims: Claim[] }>(`/v1/admin/outside-feedback?status=${status}`);
  const list = r.data?.claims ?? [];
  return (
    <div>
      <h1 className="text-[26px] font-bold">Outside feedback</h1>
      <p className="max-w-[720px] text-muted">
        Sellers ask to show feedback from other sites. Open their profile link, check the verification code is in the bio and the
        numbers match, then verify. It shows as a separate badge and never changes their score here.
      </p>
      <div className="mt-3 flex gap-2 text-[13px]">
        {[["pending", "Waiting"], ["verified", "Verified"], ["rejected", "Rejected"]].map(([v, l]) => (
          <Link key={v} href={`/admin/outside-feedback?status=${v}`} className={`rounded-full border px-3 py-1 ${status === v ? "border-ink bg-ink text-white" : "border-line"}`}>{l}</Link>
        ))}
      </div>
      <ul className="mt-4 space-y-3">
        {list.map((c) => (
          <li key={c.id} className="grid gap-4 rounded-lg border border-line p-4 text-[14px] md:grid-cols-[minmax(0,1fr)_300px]">
            <div>
              <p><Link href={`/admin/users/${c.user_id}`} className="font-semibold underline">{c.username}</Link> · {NAMES[c.platform]} · {c.feedback_count.toLocaleString()} ratings, {c.positive_percent}%</p>
              <p className="mt-1"><a href={c.profile_url} target="_blank" rel="noopener noreferrer nofollow" className="break-all text-link underline">{c.profile_url}</a></p>
              <p className="mt-1">Code to look for: <code className="rounded bg-wash px-1.5">{c.verify_code}</code></p>
              <p className="mt-1 text-[12px] text-muted">Submitted <span suppressHydrationWarning>{when(c.created_at)}</span>{c.review_note ? ` · ${c.review_note}` : ""}</p>
            </div>
            {c.status === "pending" && (
              <ActionForm endpoint={`/v1/admin/outside-feedback/${c.id}`} submit="Save decision" done="Saved." compact
                fields={[
                  { name: "decision", label: "Decision", type: "select", options: [["verified", "Verified: code found, numbers match"], ["rejected", "Reject"]] },
                  { name: "note", label: "Note (the seller sees a rejection note)", type: "textarea" },
                ]} />
            )}
          </li>
        ))}
      </ul>
      {list.length === 0 && <p className="py-10 text-center text-muted">Nothing here.</p>}
    </div>
  );
}
