import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { AUDIENCE, niceDate, type Promo } from "@/lib/promos";
import ActionForm from "@/components/admin/ActionForm";
import { promoFields } from "@/components/admin/promoFields";

export const dynamic = "force-dynamic";
export const metadata = { title: "Promo codes" };

function limits(p: Promo): string {
  const out = [p.max_redemptions ? `${p.redemptions} of ${p.max_redemptions} used` : `${p.redemptions} redeemed`];
  if (p.valid_days) out.push(`sales last ${p.valid_days} days`);
  if (p.expires_at) out.push(`redeem by ${niceDate(p.expires_at)}`);
  if (p.new_sellers_only) out.push("new sellers");
  return out.join(" · ");
}

export default async function PromosPage() {
  const s = await requireStaff("/admin/promos", "orders.view");
  const edit = s.permissions.includes("fees.edit");
  const r = await apiGet<{ promos: Promo[] }>("/v1/admin/promos");
  if (!r.data) return <p className="text-muted">Couldn&apos;t load promo codes.</p>;
  const list = r.data.promos;
  const now = Date.now();
  return (
    <div>
      <h1 className="text-[26px] font-bold">Promo codes</h1>
      <p className="mt-1 max-w-[680px] text-[14px] text-muted">
        Codes give sellers a number of sales with no marketplace fee. Sellers enter them in the seller hub, or open
        a link like <span className="font-mono">slabsnsingles.com/sell?promo=WELCOME25</span>. Stripe&apos;s card processing still applies.
      </p>

      <section className="mt-5">
        {list.length === 0 ? (
          <p className="text-[14px] text-muted">No promo codes yet.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[640px] text-[14px]">
              <thead>
                <tr className="border-b border-line text-left text-muted">
                  <th className="py-2 font-normal">Code</th><th className="py-2 font-normal">For</th>
                  <th className="py-2 text-right font-normal">Fee-free sales</th><th className="py-2 text-right font-normal">Sales used</th>
                  <th className="py-2 pl-4 font-normal">Limits</th><th className="py-2 font-normal">Status</th>
                </tr>
              </thead>
              <tbody>
                {list.map((p) => {
                  const ended = p.expires_at && Date.parse(p.expires_at) <= now;
                  const full = p.max_redemptions != null && p.redemptions >= p.max_redemptions;
                  return (
                    <tr key={p.id} className="border-b border-wash align-top">
                      <td className="py-2"><Link href={`/admin/promos/${p.id}`} className="font-mono font-semibold text-link hover:underline">{p.code}</Link></td>
                      <td className="py-2">{p.label || "—"}<div className="text-[12px] text-muted">{AUDIENCE.find((a) => a[0] === p.audience)?.[1]}</div></td>
                      <td className="num py-2 text-right">{p.fee_free_sales}</td>
                      <td className="num py-2 text-right">{p.sales_used}</td>
                      <td className="py-2 pl-4 text-[13px] text-muted">{limits(p)}</td>
                      <td className="py-2">{!p.active ? <span className="text-muted">Paused</span> : ended ? <span className="text-muted">Ended</span> : full ? <span className="text-muted">Used up</span> : <span className="text-ship">Active</span>}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {edit && (
        <section className="mt-8 max-w-[560px] rounded-lg border border-line p-4">
          <h2 className="mb-1 font-bold">New promo code</h2>
          <p className="mb-3 text-[13px] text-muted">
            Example: <span className="font-mono">WELCOME25</span> with 25 sales for any new seller, and <span className="font-mono">CARDSHOP-SMITH</span> with
            250 sales, single use, for one business you&apos;re signing up.
          </p>
          <ActionForm endpoint="/v1/admin/promos" fields={promoFields()} submit="Create code" done="Created. Share the code or link with the seller." />
        </section>
      )}
    </div>
  );
}
