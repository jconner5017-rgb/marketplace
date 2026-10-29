import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { ROLE, StatusBadge, when } from "@/components/admin/Badges";
import { MemberActions } from "@/components/admin/MemberActions";
import ActionForm from "@/components/admin/ActionForm";
import type { MemberDetail } from "@/lib/admin-types";

type Warning = { id: string; reason: string; created_at: string; rescinded_at: string | null; by?: string };

export const dynamic = "force-dynamic";
export const metadata = { title: "Member" };

const COUNT_LABEL: Record<string, string> = {
  listings_total: "Listings (all time)", auctions_live: "Live auctions", bids: "Bids placed", offers_made: "Offers made",
  won_or_accepted: "Won or accepted", sold_holds: "Sold (awaiting or paid)", threads: "Conversations",
  blocked_messages: "Blocked messages", watching: "Watching", sessions: "Signed-in sessions",
};

export default async function MemberPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const s = await requireStaff(`/admin/users/${id}`, "users.view");
  const r = await apiGet<{ member: MemberDetail; warnings: Warning[]; tier_locked: boolean }>(`/v1/admin/users/${id}`);
  if (!r.data) notFound();
  const m = r.data.member;
  const warnings = r.data.warnings ?? [];
  const self = s.user.id === m.id;

  return (
    <div>
      <Link href="/admin/users" className="text-[13px] text-link hover:underline">← Members</Link>
      <div className="mt-2 flex flex-wrap items-center gap-3">
        <h1 className="text-[26px] font-bold">{m.username}</h1>
        <StatusBadge status={m.status} />
        {m.staff_role && <span className="rounded-full bg-ink px-2 py-0.5 text-[12px] font-semibold text-white">{ROLE[m.staff_role]}</span>}
        {m.owner && <span className="text-[12px] text-muted">Owner account</span>}
      </div>
      <p className="text-muted">{m.email} {m.email_verified ? "· verified" : "· not verified"} · joined <span suppressHydrationWarning>{when(m.created_at)}</span></p>
      {m.status !== "active" && (
        <p className="mt-2 rounded-lg bg-wash p-3 text-[14px]">
          <span className="font-semibold capitalize">{m.status}</span>
          {m.suspended_until && <> until <span suppressHydrationWarning>{when(m.suspended_until)}</span></>}
          {m.status_reason && <>: {m.status_reason}</>}
        </p>
      )}

      <div className="mt-6 grid gap-8 xl:grid-cols-[minmax(0,1fr)_360px]">
        <div className="min-w-0 space-y-8">
          <section>
            <h2 className="text-[18px] font-bold">Activity</h2>
            <dl className="mt-2 grid grid-cols-2 gap-2 sm:grid-cols-3">
              {Object.entries(COUNT_LABEL).map(([k, l]) => (
                <div key={k} className="rounded-md border border-line p-2">
                  <dt className="text-[12px] text-muted">{l}</dt>
                  <dd className="num text-[18px] font-bold">{m.counts[k] ?? 0}</dd>
                </div>
              ))}
            </dl>
            <p className="mt-2 text-[13px]">
              <Link href={`/search?seller=${encodeURIComponent(m.username)}`} className="text-link underline">Their public listings</Link>
            </p>
          </section>

          {m.seller && (
            <section>
              <h2 className="text-[18px] font-bold">Seller</h2>
              <dl className="mt-2 grid grid-cols-[180px_1fr] gap-y-1 text-[14px]">
                <dt className="text-muted">Our review</dt><dd><StatusBadge status={m.seller.review_status} />{m.seller.review_note && <span className="ml-2">{m.seller.review_note}</span>}</dd>
                <dt className="text-muted">Stripe account</dt><dd className="font-mono text-[13px]">{m.seller.stripe_account_id}</dd>
                <dt className="text-muted">Stripe identity check</dt><dd>{m.seller.details_submitted ? "Submitted" : "Not finished"} · {m.seller.charges_enabled ? "can take payments" : "can't take payments yet"}</dd>
                <dt className="text-muted">Seller level</dt><dd>{m.seller.tier} ({["", "$20", "$250", "$1,000"][m.seller.tier]} limit){r.data.tier_locked ? " · held by staff" : ""}</dd>
                <dt className="text-muted">Seller since</dt><dd suppressHydrationWarning>{when(m.seller.since)}</dd>
              </dl>
              <p className="mt-1 text-[12px] text-muted">Identity, business and tax documents are collected and verified by Stripe; we don&apos;t store them.</p>
            </section>
          )}

          <section>
            <h2 className="text-[18px] font-bold">Warnings</h2>
            <ul className="mt-2 space-y-2 text-[14px]">
              {warnings.map((w) => (
                <li key={w.id} className={`border-b border-wash pb-2 ${w.rescinded_at ? "text-muted line-through" : ""}`}>
                  {w.reason} · <span className="text-muted" suppressHydrationWarning>{when(w.created_at)}</span>{w.by ? ` · ${w.by}` : ""}
                  {w.rescinded_at && <span className="ml-1 no-underline">(withdrawn on appeal)</span>}
                </li>
              ))}
              {warnings.length === 0 && <li className="text-muted">No warnings.</li>}
            </ul>
          </section>

          <section>
            <h2 className="text-[18px] font-bold">Sign-in history</h2>
            <div className="mt-2 overflow-x-auto">
              <table className="w-full min-w-[560px] text-[13px]">
                <thead><tr className="border-b border-line text-left text-muted"><th className="py-1.5 pr-3">When</th><th className="py-1.5 pr-3">Result</th><th className="py-1.5 pr-3">IP</th><th className="py-1.5">Browser</th></tr></thead>
                <tbody>
                  {m.logins.map((e, i) => (
                    <tr key={i} className="border-b border-wash">
                      <td className="py-1.5 pr-3 whitespace-nowrap" suppressHydrationWarning>{when(e.at)}</td>
                      <td className={`py-1.5 pr-3 ${e.success ? "text-ship" : "text-slab"}`}>{e.success ? "Signed in" : e.reason.replace("_", " ")}</td>
                      <td className="py-1.5 pr-3 font-mono">{e.ip}</td>
                      <td className="max-w-[260px] truncate py-1.5 text-muted" title={e.user_agent}>{e.user_agent}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {m.logins.length === 0 && <p className="py-3 text-[13px] text-muted">No sign-ins recorded yet.</p>}
            </div>
          </section>

          <section>
            <h2 className="text-[18px] font-bold">Staff history</h2>
            <ul className="mt-2 space-y-2 text-[14px]">
              {m.history.map((h) => (
                <li key={h.id} className="border-b border-wash pb-2">
                  <span className="font-mono text-[13px]">{h.action}</span> by {h.actor_email} · <span className="text-muted" suppressHydrationWarning>{when(h.at)}</span>
                  {h.reason && <p className="text-muted">{h.reason}</p>}
                </li>
              ))}
              {m.history.length === 0 && <li className="text-muted">No staff actions yet.</li>}
            </ul>
            {s.permissions.includes("audit.view") && (
              <Link href={`/admin/audit?target_type=user&target_id=${m.id}`} className="mt-2 inline-block text-[13px] text-link underline">Full audit trail</Link>
            )}
          </section>
        </div>

        <aside className="space-y-6">
          {self ? (
            <p className="rounded-lg bg-wash p-3 text-[14px]">This is your account. Changes to it are made by another Super Admin.</p>
          ) : (
            <>
              <MemberActions m={m} permissions={s.permissions} myRole={s.staff_role} />
              {s.permissions.includes("users.restrict") && (
                <section className="rounded-lg border border-line p-4">
                  <h2 className="font-bold">Give a warning</h2>
                  <p className="mt-1 text-[13px] text-muted">The member sees it on their account page and can appeal once.</p>
                  <div className="mt-2">
                    <ActionForm endpoint={`/v1/admin/users/${m.id}/warn`} submit="Warn member" done="Warning added." compact
                      fields={[{ name: "reason", label: "Reason", type: "textarea" }]} />
                  </div>
                </section>
              )}
              {m.seller && s.permissions.includes("sellers.review") && (
                <section className="rounded-lg border border-line p-4">
                  <h2 className="font-bold">Seller level</h2>
                  <p className="mt-1 text-[13px] text-muted">Lowering it hides fixed-price listings above the new limit. Hold it to stop the nightly check moving it back.</p>
                  <div className="mt-2">
                    <ActionForm endpoint={`/v1/admin/users/${m.id}/tier`} submit="Change level" done="Seller level changed." compact
                      transform={(v) => ({ tier: Number(v.tier), lock: Boolean(v.lock), reason: v.reason })}
                      fields={[
                        { name: "tier", label: "Level", type: "select", initial: String(m.seller.tier),
                          options: [["1", "1 · New ($20 limit)"], ["2", "2 · Standard ($250)"], ["3", "3 · Trusted ($1,000)"]] },
                        { name: "lock", label: "Hold at this level", type: "checkbox", initial: r.data.tier_locked },
                        { name: "reason", label: "Reason (shown to the seller)", type: "textarea" },
                      ]} />
                  </div>
                </section>
              )}
            </>
          )}
          <section className="rounded-lg border border-line p-4">
            <h2 className="font-bold">Staff notes</h2>
            {(s.permissions.includes("users.edit") || s.permissions.includes("users.restrict")) && (
              <div className="mt-2"><NotesForm id={m.id} /></div>
            )}
            <ul className="mt-3 space-y-3 text-[14px]">
              {m.notes.map((n) => (
                <li key={n.id}>
                  <p className="whitespace-pre-line">{n.body}</p>
                  <p className="text-[12px] text-muted">{n.author} · <span suppressHydrationWarning>{when(n.at)}</span></p>
                </li>
              ))}
              {m.notes.length === 0 && <li className="text-muted">No notes.</li>}
            </ul>
          </section>
        </aside>
      </div>
    </div>
  );
}

function NotesForm({ id }: { id: string }) {
  return <ActionForm endpoint={`/v1/admin/users/${id}/notes`} fields={[{ name: "body", label: "Add a note", type: "textarea" }]} submit="Add note" done="Note added." compact />;
}
