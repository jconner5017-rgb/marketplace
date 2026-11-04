import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import type { Store, StoreSale } from "@/lib/types";
import { StoreCategoriesForm, StoreSalesForm, StoreSettingsForm, StoreUpdateForm } from "@/components/sell/StoreForms";

export const dynamic = "force-dynamic";
export const metadata = { title: "Your store", robots: { index: false } };

const STATUS: Record<Store["status"], [string, string]> = {
  pending: ["Waiting for review", "We're checking your business details, usually within a day. We'll let you know in your notifications."],
  active: ["Open", "Your store is live."],
  rejected: ["Needs changes", "Update the details below and save to send it for review again."],
  suspended: ["Paused", "Your store page is hidden. Contact us if you have questions."],
};

export default async function SellerStore() {
  const me = await requireUser("/sell/store");
  const r = await apiGet<{ store: Store | null; sales: StoreSale[] }>("/v1/seller/store");
  const st = r.data?.store ?? null;
  const sales = r.data?.sales ?? [];
  const card = "rounded-xl border border-line p-4";

  if (!st) {
    return (
      <div className="max-w-[820px] pb-10 pt-6">
        <h1 className="text-[26px] font-bold">Open a business store</h1>
        <p className="mt-2 text-muted">Free for registered businesses: card shops, breakers, show dealers and online sellers.</p>
        <ul className="mt-4 grid gap-2 text-[15px] sm:grid-cols-2">
          <li>✓ Your own page at slabsnsingles.com/store/your-name</li>
          <li>✓ Logo, banner, About and store policies</li>
          <li>✓ Your own categories and in-store search</li>
          <li>✓ Store-wide sales, up to 50% off</li>
          <li>✓ Followers who hear about new cards</li>
          <li>✓ Weekly updates to your followers</li>
        </ul>
        <section className={`mt-6 ${card}`}><StoreSettingsForm store={null} username={me.username} /></section>
        <p className="mt-3 text-[13px] text-muted"><Link href="/help/stores" className="text-link underline">How stores work</Link></p>
      </div>
    );
  }

  const [label, note] = STATUS[st.status];
  return (
    <div className="pb-10 pt-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-[26px] font-bold">{st.name}</h1>
          <p className="text-[14px]"><span className={`font-semibold ${st.status === "active" ? "text-ship" : st.status === "pending" ? "text-warn" : "text-slab"}`}>{label}</span>
            <span className="text-muted"> · {note}</span></p>
          {st.review_note && st.status !== "active" && <p className="mt-1 text-[14px] text-slab">Our note: {st.review_note}</p>}
        </div>
        {st.status === "active" && (
          <div className="flex gap-2 text-[14px]">
            <span className="num self-center text-muted">{st.followers.toLocaleString()} {st.followers === 1 ? "follower" : "followers"}</span>
            <Link href={`/store/${st.slug}`} className="rounded-full bg-link px-5 py-2 font-semibold text-white hover:bg-link-dark">View your store</Link>
          </div>
        )}
      </div>

      <div className="mt-5 grid gap-4 lg:grid-cols-2">
        {st.status === "active" && (
          <>
            <section className={card}>
              <h2 className="text-[18px] font-bold">Store sale</h2>
              {st.sale && <p className="mt-1 text-[14px] text-slab">Running now: {st.sale.name}, {st.sale.percent}% off.</p>}
              <div className="mt-3"><StoreSalesForm sales={sales} categories={st.categories} /></div>
            </section>
            <section className={card}>
              <h2 className="text-[18px] font-bold">Categories</h2>
              <div className="mt-3"><StoreCategoriesForm categories={st.categories} /></div>
            </section>
            <section className={card}>
              <h2 className="text-[18px] font-bold">Update your followers</h2>
              <div className="mt-3"><StoreUpdateForm followers={st.followers} last={st.last_update_at} /></div>
            </section>
          </>
        )}
        <section className={`${card} lg:col-span-2`}>
          <h2 className="text-[18px] font-bold">Store settings</h2>
          <div className="mt-3"><StoreSettingsForm store={st} username={me.username} /></div>
        </section>
      </div>
    </div>
  );
}
