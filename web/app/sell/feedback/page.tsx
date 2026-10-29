import Link from "next/link";
import { apiGet, getMe } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import { AutoFeedbackForm, OutsideClaimForm } from "@/components/feedback/SellerFeedbackForms";

export const dynamic = "force-dynamic";
export const metadata = { title: "Feedback tools", robots: { index: false } };

type Claim = { id: string; platform: string; profile_url: string; feedback_count: number; positive_percent: number; verify_code: string; status: string; review_note: string | null };
type Data = { auto_feedback: string; auto_feedback_comment: string; outside: Claim[]; platforms: Record<string, string> };

export default async function SellerFeedbackPage() {
  await requireUser("/sell/feedback");
  const [r, me] = await Promise.all([apiGet<Data>("/v1/seller/feedback"), getMe()]);
  if (!r.data) return <p className="py-16 text-center">Set up selling first. <Link href="/sell" className="text-link underline">Seller hub</Link></p>;
  const d = r.data;
  return (
    <div className="max-w-[720px] pt-6">
      <Link href="/sell" className="text-[14px] text-link hover:underline">← Seller hub</Link>
      <h1 className="mt-2 text-[26px] font-bold">Feedback tools</h1>
      <p className="text-muted">
        Buyers see your score and positive percentage on every listing.{" "}
        {me && <Link href={`/seller/${encodeURIComponent(me.username)}`} className="text-link underline">See your public profile</Link>}
      </p>

      <section className="mt-6 rounded-lg border border-line p-4">
        <AutoFeedbackForm mode={d.auto_feedback} comment={d.auto_feedback_comment} />
      </section>

      <section className="mt-8">
        <h2 className="border-b border-line pb-2 text-[18px] font-bold">Bring your feedback from other sites</h2>
        <p className="mt-2 text-[14px]">
          Selling on eBay, TCGplayer or elsewhere already? Once our team confirms the profile is yours, your profile shows a
          &quot;Verified&quot; badge with that site&apos;s numbers. It sits beside your score here and doesn&apos;t change it.
        </p>
        {d.outside.length > 0 && (
          <ul className="mt-3 text-[14px]">
            {d.outside.map((c) => (
              <li key={c.id} className="border-b border-wash py-2">
                <span className="font-semibold">{d.platforms[c.platform]}</span> · {c.feedback_count.toLocaleString()} ratings, {c.positive_percent}% ·{" "}
                <span className={c.status === "verified" ? "text-ship" : c.status === "rejected" ? "text-slab" : ""}>
                  {c.status === "pending" ? <>checking; add <code className="rounded bg-wash px-1">{c.verify_code}</code> to your bio there</> : c.status}
                </span>
                {c.review_note && <span className="block text-[13px] text-muted">{c.review_note}</span>}
              </li>
            ))}
          </ul>
        )}
        <div className="mt-4"><OutsideClaimForm platforms={d.platforms} /></div>
      </section>
    </div>
  );
}
