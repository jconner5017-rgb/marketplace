import Link from "next/link";
import { apiGet, getSeller } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import PromoBox from "@/components/PromoBox";
import BundleDiscountForm from "@/components/BundleDiscountForm";
import type { Redemption } from "@/lib/promos";

export const dynamic = "force-dynamic";
export const metadata = { title: "Marketing", robots: { index: false } };

export default async function SellerMarketing() {
  await requireUser("/sell/marketing");
  const [{ seller }, promosR] = await Promise.all([
    getSeller(), apiGet<{ fee_free_sales_left: number; redemptions: Redemption[] }>("/v1/seller/promos"),
  ]);
  return (
    <div className="pb-10 pt-6">
      <h1 className="text-[26px] font-bold">Marketing</h1>
      <p className="mt-1 text-muted">Ways to bring buyers back and sell more cards per order.</p>
      <div className="mt-5 grid gap-4 md:grid-cols-2">
        <section className="rounded-xl border border-line p-4">
          <h2 className="font-bold">Multi-card discount</h2>
          <p className="mt-1 text-[14px] text-muted">Buyers who order several cards from you ship in one package, so pass the savings on.</p>
          {seller && <BundleDiscountForm type={seller.bundle_discount_type} value={seller.bundle_discount_val} />}
        </section>
        <section className="rounded-xl border border-line p-4">
          <h2 className="font-bold">Trusted buyers</h2>
          <p className="mt-1 text-[14px] text-muted">
            Let friends and regular buyers choose a stamped envelope for one raw card up to $100, so they save on shipping.
          </p>
          <Link href="/sell/trusted" className="mt-3 inline-block rounded-lg border border-ink px-4 py-2 text-[14px] font-semibold hover:bg-wash">Manage trusted buyers</Link>
        </section>
        <section className="rounded-xl border border-line p-4 md:col-span-2">
          <h2 className="font-bold">Store sales and followers</h2>
          <p className="mt-1 text-[14px] text-muted">
            Business stores can run store-wide sales up to 50% off and send weekly updates to their followers. Stores are free for registered businesses.
          </p>
          <Link href="/sell/store" className="mt-3 inline-block rounded-lg border border-ink px-4 py-2 text-[14px] font-semibold hover:bg-wash">Go to your store</Link>
        </section>
        <div className="md:col-span-2">
          <PromoBox left={promosR.data?.fee_free_sales_left ?? 0} redemptions={promosR.data?.redemptions ?? []} initial="" />
        </div>
      </div>
    </div>
  );
}
