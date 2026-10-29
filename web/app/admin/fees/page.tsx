import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { money } from "@/lib/format";
import { when } from "@/components/admin/Badges";
import ActionForm from "@/components/admin/ActionForm";
import DeleteButton from "@/components/admin/DeleteButton";

export const dynamic = "force-dynamic";
export const metadata = { title: "Fees" };

type Schedule = { id: number; flat_cents: number; threshold_cents: number; percent_bps: number; cap_cents: number; effective_at: string; note: string; created_at: string };
type Upgrade = { key: string; label: string; description: string; price_cents: number; active: boolean };

function fee(s: Schedule, c: number) {
  if (c <= s.threshold_cents) return Math.min(s.flat_cents, c);
  return Math.min(s.cap_cents, c, Math.max(s.flat_cents, Math.floor((c * s.percent_bps) / 10000)));
}
const pct = (bps: number) => `${(bps / 100).toFixed(bps % 100 ? 2 : 0)}%`;
const describe = (s: Schedule) =>
  `${money(s.flat_cents)} per card up to ${money(s.threshold_cents)}, then ${pct(s.percent_bps)} capped at ${money(s.cap_cents)}`;

export default async function FeesPage() {
  const s = await requireStaff("/admin/fees", "orders.view");
  const edit = s.permissions.includes("fees.edit");
  const r = await apiGet<{ current: Schedule; history: Schedule[]; upgrades: Upgrade[] }>("/v1/admin/fees");
  if (!r.data) return <p className="text-muted">Couldn&apos;t load fees.</p>;
  const { current, history, upgrades } = r.data;
  const now = Date.now();
  const upcoming = history.filter((h) => Date.parse(h.effective_at) > now).reverse();
  const examples = [500, 2000, 10000, 15000, 50000, 100000];
  return (
    <div>
      <h1 className="text-[26px] font-bold">Fees</h1>
      <section className="mt-4 rounded-lg border border-ink p-4">
        <p className="text-[13px] text-muted">In force now</p>
        <p className="text-[18px] font-bold">{describe(current)}</p>
        <table className="mt-3 w-full max-w-[520px] text-[14px]">
          <thead><tr className="border-b border-line text-left text-muted"><th className="py-1 font-normal">Card sells for</th><th className="py-1 text-right font-normal">Our fee</th><th className="py-1 text-right font-normal">Share</th></tr></thead>
          <tbody>
            {examples.map((c) => (
              <tr key={c} className="border-b border-wash">
                <td className="num py-1">{money(c)}</td>
                <td className="num py-1 text-right">{money(fee(current, c))}</td>
                <td className="num py-1 text-right text-muted">{((fee(current, c) / c) * 100).toFixed(1)}%</td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="mt-2 text-[12px] text-muted">The same fee applies to Buy It Now sales, winning bids and accepted offers. Each sale keeps the fees in force when it happened.</p>
      </section>

      {upcoming.length > 0 && (
        <section className="mt-6">
          <h2 className="text-[18px] font-bold">Scheduled changes</h2>
          <ul className="mt-2 space-y-2 text-[14px]">
            {upcoming.map((u) => (
              <li key={u.id} className="flex flex-wrap items-center gap-3 rounded-lg bg-wash p-3">
                <span>From <span suppressHydrationWarning className="font-semibold">{when(u.effective_at)}</span>: {describe(u)}{u.note && ` · ${u.note}`}</span>
                {edit && <DeleteButton endpoint={`/v1/admin/fees/${u.id}`} label="Cancel" />}
              </li>
            ))}
          </ul>
        </section>
      )}

      {edit && (
        <section className="mt-6 max-w-[560px] rounded-lg border border-line p-4">
          <h2 className="mb-2 font-bold">Change fees</h2>
          <ActionForm
            endpoint="/v1/admin/fees"
            fields={[
              { name: "flat", key: "flat_cents", as: "cents", label: "Flat fee per card ($)", initial: (current.flat_cents / 100).toFixed(2) },
              { name: "threshold", key: "threshold_cents", as: "cents", label: "Flat fee applies up to ($)", initial: (current.threshold_cents / 100).toFixed(2) },
              { name: "percent", key: "percent_bps", as: "bps", label: "Above that, percent of the price", initial: String(current.percent_bps / 100) },
              { name: "cap", key: "cap_cents", as: "cents", label: "Fee cap per card ($)", initial: (current.cap_cents / 100).toFixed(2) },
              { name: "effective", key: "effective_at", as: "iso", omitEmpty: true, label: "Starts (leave blank for now)", type: "datetime" },
              { name: "note", label: "Note (shown in history and the audit log)", placeholder: "Holiday promotion" },
            ]}
            submit="Save new fees"
            done="Saved. The fee schedule is updated."
          />
          <p className="mt-2 text-[12px] text-muted">Update the Selling fees help page to match when fees change.</p>
        </section>
      )}

      <section className="mt-8">
        <h2 className="text-[18px] font-bold">Listing upgrades</h2>
        <p className="text-[13px] text-muted">Optional extras sellers could pay for. Switched off upgrades aren&apos;t offered.</p>
        <ul className="mt-3 grid gap-3 md:grid-cols-2">
          {upgrades.map((u) => (
            <li key={u.key} className="rounded-lg border border-line p-4 text-[14px]">
              <p className="font-semibold">{u.label} · {money(u.price_cents)} · {u.active ? <span className="text-ship">on</span> : <span className="text-muted">off</span>}</p>
              <p className="text-muted">{u.description}</p>
              {edit && (
                <details className="mt-2">
                  <summary className="cursor-pointer text-[13px] text-link underline">Edit</summary>
                  <div className="mt-2">
                    <ActionForm
                      endpoint={`/v1/admin/fees/upgrades/${u.key}`}
                      method="PATCH"
                      fields={[
                        { name: "label", label: "Name", initial: u.label },
                        { name: "description", label: "Description", type: "textarea", initial: u.description },
                        { name: "price", key: "price_cents", as: "cents", label: "Price ($)", initial: (u.price_cents / 100).toFixed(2) },
                        { name: "active", label: "Offer this upgrade", type: "checkbox", initial: u.active },
                      ]}
                      submit="Save"
                      compact
                    />
                  </div>
                </details>
              )}
            </li>
          ))}
        </ul>
        <p className="mt-2 text-[12px] text-muted">Selling upgrades (charging for them and showing them in search) gets built when you&apos;re ready to offer them.</p>
      </section>

      <section className="mt-8">
        <h2 className="text-[18px] font-bold">History</h2>
        <ul className="mt-2 space-y-1 text-[14px]">
          {history.map((h) => (
            <li key={h.id} className="border-b border-wash py-1">
              <span suppressHydrationWarning className="text-muted">{when(h.effective_at)}</span> · {describe(h)}{h.note && ` · ${h.note}`}
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
