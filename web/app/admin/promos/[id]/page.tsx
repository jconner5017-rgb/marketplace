import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { niceDate, type Promo, type Redemption } from "@/lib/promos";
import ActionForm from "@/components/admin/ActionForm";
import { promoFields } from "@/components/admin/promoFields";

export const dynamic = "force-dynamic";
export const metadata = { title: "Promo code" };

export default async function PromoPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const s = await requireStaff(`/admin/promos/${id}`, "orders.view");
  const edit = s.permissions.includes("fees.edit");
  const r = await apiGet<{ promo: Promo; redemptions: Redemption[] }>(`/v1/admin/promos/${encodeURIComponent(id)}`);
  if (r.status === 404) notFound();
  if (!r.data) return <p className="text-muted">Couldn&apos;t load this promo code.</p>;
  const { promo: p, redemptions } = r.data;
  return (
    <div>
      <Link href="/admin/promos" className="text-[14px] text-link hover:underline">← Promo codes</Link>
      <h1 className="mt-2 font-mono text-[26px] font-bold">{p.code}</h1>
      <p className="text-[14px] text-muted">
        {p.label || "No label"} · {p.fee_free_sales} fee-free sales per seller · signup link{" "}
        <span className="font-mono">slabsnsingles.com/sell?promo={p.code}</span>
      </p>

      <section className="mt-5">
        <h2 className="text-[18px] font-bold">Sellers who redeemed it ({redemptions.length})</h2>
        {redemptions.length === 0 ? (
          <p className="mt-1 text-[14px] text-muted">Nobody yet.</p>
        ) : (
          <table className="mt-2 w-full max-w-[680px] text-[14px]">
            <thead><tr className="border-b border-line text-left text-muted"><th className="py-1 font-normal">Seller</th><th className="py-1 font-normal">Redeemed</th><th className="py-1 text-right font-normal">Used</th><th className="py-1 pl-4 font-normal">Fee-free sales end</th></tr></thead>
            <tbody>
              {redemptions.map((x) => (
                <tr key={x.id} className="border-b border-wash">
                  <td className="py-1"><Link href={`/seller/${x.seller}`} className="text-link hover:underline">{x.seller}</Link></td>
                  <td className="py-1">{niceDate(x.redeemed_at)}</td>
                  <td className="num py-1 text-right">{x.sales_used} of {x.sales_granted}</td>
                  <td className="py-1 pl-4 text-muted">{x.expires_at ? niceDate(x.expires_at) : "No end"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      {edit && (
        <section className="mt-8 max-w-[560px] rounded-lg border border-line p-4">
          <h2 className="mb-1 font-bold">Edit</h2>
          <p className="mb-3 text-[13px] text-muted">Changes apply to sellers who redeem from now on. Sellers who already redeemed keep the sales they got. Untick Active to pause the code.</p>
          <ActionForm endpoint={`/v1/admin/promos/${p.id}`} method="PATCH"
            fields={[...promoFields(p), { name: "reason", label: "Note for the audit log", placeholder: "Extended for the holidays" }]}
            submit="Save changes" />
        </section>
      )}
    </div>
  );
}
