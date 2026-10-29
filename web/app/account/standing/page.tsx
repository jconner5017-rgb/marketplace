import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import { money } from "@/lib/format";
import AppealForm from "@/components/AppealForm";

export const dynamic = "force-dynamic";
export const metadata = { title: "Account standing", robots: { index: false } };

type Appeal = { id: string; about: string; ref: string; status: string; decision_note: string | null; created_at: string; decision: string };
type Standing = {
  status: string; status_reason: string | null; suspended_until: string | null;
  warnings: { id: string; reason: string; created_at: string; rescinded_at: string | null }[];
  appeals: Appeal[];
  tier: number | null; tier_reason: string | null; tier_change_id: string | null;
  blocked_listings: { id: string; title: string; reason: string }[] | null;
};

const CAP: Record<number, number> = { 1: 2000, 2: 25000, 3: 100000 };
const day = (iso: string) => new Date(iso).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" });

export default async function StandingPage() {
  await requireUser("/account/standing");
  const r = await apiGet<Standing>("/v1/me/standing");
  const s = r.data;
  if (!s) return <p className="py-16 text-center text-muted">Couldn&apos;t load your account standing. Try again in a minute.</p>;
  const appealFor = (about: string, ref: string) => s.appeals.find((a) => a.about === about && (ref === "" || a.ref === ref));
  const live = s.warnings.filter((w) => !w.rescinded_at);

  return (
    <div className="mx-auto max-w-[720px] pt-6">
      <h1 className="text-[26px] font-bold">Account standing</h1>
      {s.status === "active" && live.length === 0 && (s.blocked_listings ?? []).length === 0 && (
        <p className="mt-3 rounded-lg bg-ship/10 p-4 text-[15px] text-ship">Your account is in good standing.</p>
      )}

      {s.status === "frozen" && (
        <section className="mt-5 rounded-lg border border-ink p-4">
          <h2 className="font-bold">Your account is frozen</h2>
          <p className="mt-1 text-[14px]">You can browse but not buy, sell, bid or message while we review it.{s.status_reason ? ` Reason: ${s.status_reason}` : ""}</p>
          <AppealStatus a={appealFor("freeze", "")} />
          {!appealFor("freeze", "") && <AppealForm about="freeze" />}
        </section>
      )}

      <section className="mt-6">
        <h2 className="border-b border-line pb-2 text-[18px] font-bold">Warnings</h2>
        {s.warnings.length === 0 ? <p className="py-3 text-[14px] text-muted">No warnings.</p> : (
          <ul>
            {s.warnings.map((w) => {
              const a = appealFor("warning", w.id);
              return (
                <li key={w.id} className="border-b border-wash py-3 text-[14px]">
                  <p className={w.rescinded_at ? "text-muted line-through" : ""}>{w.reason}</p>
                  <p className="text-[13px] text-muted">{day(w.created_at)}{w.rescinded_at ? " · withdrawn" : ""}</p>
                  <AppealStatus a={a} />
                  {!a && !w.rescinded_at && <AppealForm about="warning" refId={w.id} />}
                </li>
              );
            })}
          </ul>
        )}
      </section>

      {s.tier != null && (
        <section className="mt-6">
          <h2 className="border-b border-line pb-2 text-[18px] font-bold">Seller level</h2>
          <p className="mt-3 text-[14px]">Level {s.tier}: cards up to {money(CAP[s.tier])}. <Link href="/help/seller-levels" className="text-link underline">How levels work</Link></p>
          {s.tier_reason && s.tier_change_id && (
            <div className="mt-2 text-[14px]">
              <p className="text-muted">Last lowered: {s.tier_reason}</p>
              <AppealStatus a={appealFor("demotion", s.tier_change_id)} />
              {!appealFor("demotion", s.tier_change_id) && <AppealForm about="demotion" refId={s.tier_change_id} />}
            </div>
          )}
        </section>
      )}

      {(s.blocked_listings ?? []).length > 0 && (
        <section className="mt-6">
          <h2 className="border-b border-line pb-2 text-[18px] font-bold">Listings taken down</h2>
          <ul>
            {s.blocked_listings!.map((l) => {
              const a = appealFor("listing", l.id);
              return (
                <li key={l.id} className="border-b border-wash py-3 text-[14px]">
                  <p className="font-semibold">{l.title}</p>
                  <p className="text-muted">{l.reason}</p>
                  <AppealStatus a={a} />
                  {!a && <AppealForm about="listing" refId={l.id} />}
                </li>
              );
            })}
          </ul>
        </section>
      )}
    </div>
  );
}

function AppealStatus({ a }: { a?: Appeal }) {
  if (!a) return null;
  const label = a.status === "open" ? "Appeal under review" : a.status === "overturned" ? "Appeal accepted" : "Appeal reviewed: the decision stands";
  return (
    <p className={`mt-2 text-[13px] ${a.status === "overturned" ? "text-ship" : a.status === "open" ? "" : "text-muted"}`}>
      {label}{a.decision_note ? `: ${a.decision_note}` : ""}
    </p>
  );
}
