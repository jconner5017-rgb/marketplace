import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { StatusBadge, when } from "@/components/admin/Badges";
import { money } from "@/lib/format";
import ModerateListing from "@/components/admin/ModerateListing";

export const dynamic = "force-dynamic";
export const metadata = { title: "Listings" };

type Row = {
  id: string; title: string; seller: string; seller_id: string; external_id: string | null; category: string; game_or_sport: string;
  format: string; status: string; price_cents: number; quantity: number; bid_count: number; auction_state: string | null;
  ends_at: string | null; offers_enabled: boolean; photo: string; open_reports: number; moderation_reason: string | null; created_at: string;
};

export default async function ListingsPage({ searchParams }: { searchParams: Promise<Record<string, string | undefined>> }) {
  const s = await requireStaff("/admin/listings", "users.view");
  const sp = await searchParams;
  const page = Math.max(1, Number(sp.page) || 1);
  const qs = new URLSearchParams({ limit: "50", offset: String((page - 1) * 50) });
  for (const k of ["q", "status", "seller", "format", "reported", "min_price", "max_price"]) if (sp[k]) qs.set(k, sp[k]!);
  const r = await apiGet<{ listings: Row[]; total: number }>("/v1/admin/listings?" + qs.toString());
  const rows = r.data?.listings ?? [];
  const total = r.data?.total ?? 0;
  const canModerate = s.permissions.includes("listings.moderate");
  const link = (p: number) => {
    const n = new URLSearchParams(Object.entries(sp).filter(([, v]) => v) as [string, string][]);
    n.set("page", String(p));
    return `/admin/listings?${n}`;
  };
  const input = "mt-1 rounded-md border border-line px-3 py-2";
  return (
    <div>
      <h1 className="text-[26px] font-bold">Listings</h1>
      <form action="/admin/listings" className="mt-4 flex flex-wrap items-end gap-3 text-[14px]">
        <label className="min-w-[200px] flex-1"><span className="block text-[13px] font-semibold">Search</span>
          <input name="q" defaultValue={sp.q} placeholder="Title, listing ID or seller SKU" className={`${input} w-full`} /></label>
        <label><span className="block text-[13px] font-semibold">Seller</span><input name="seller" defaultValue={sp.seller} placeholder="Username" className={`${input} w-36`} /></label>
        <label><span className="block text-[13px] font-semibold">Status</span>
          <select name="status" defaultValue={sp.status ?? ""} className={input}>
            <option value="">Any</option>
            {["active", "sold_out", "hidden", "ended", "removed", "blocked"].map((v) => <option key={v} value={v}>{v === "blocked" ? "taken down" : v.replace("_", " ")}</option>)}
          </select></label>
        <label><span className="block text-[13px] font-semibold">Format</span>
          <select name="format" defaultValue={sp.format ?? ""} className={input}>
            <option value="">Any</option><option value="fixed">Buy It Now</option><option value="auction">Auction</option>
          </select></label>
        <label className="flex items-center gap-1.5 pb-2"><input type="checkbox" name="reported" value="1" defaultChecked={sp.reported === "1"} /> Reported</label>
        <button className="rounded-full bg-link px-5 py-2 font-semibold text-white hover:bg-link-dark">Search</button>
      </form>
      <p className="mt-4 text-[13px] text-muted">{total.toLocaleString("en-US")} listings</p>
      <ul className="mt-2 divide-y divide-line border-y border-line">
        {rows.map((l) => (
          <li key={l.id} className="flex flex-col gap-3 py-3 sm:flex-row">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={l.photo} alt="" className="h-[84px] w-[60px] shrink-0 rounded bg-wash object-contain" />
            <div className="min-w-0 flex-1 text-[14px]">
              <div className="flex flex-wrap items-center gap-2">
                <Link href={`/item/${l.id}`} className="font-semibold hover:underline">{l.title}</Link>
                <StatusBadge status={l.status === "blocked" ? "rejected" : l.status === "active" ? "active" : "pending"} />
                <span className="text-[12px] text-muted">{l.status === "blocked" ? "taken down" : l.status.replace("_", " ")}</span>
                {l.open_reports > 0 && <Link href={`/admin/reports?type=listing`} className="rounded-full bg-slab/10 px-2 py-0.5 text-[12px] font-semibold text-slab">{l.open_reports} open reports</Link>}
              </div>
              <p className="text-muted">
                <Link href={`/admin/users/${l.seller_id}`} className="underline">{l.seller}</Link> · {l.game_or_sport}
                {l.external_id && <> · SKU {l.external_id}</>} · listed <span suppressHydrationWarning>{when(l.created_at)}</span>
              </p>
              <p>
                <span className="num font-semibold">{money(l.price_cents)}</span>{" "}
                {l.format === "auction" ? <>auction · {l.bid_count} bids · {l.auction_state}</> : <>Buy It Now · qty {l.quantity}{l.offers_enabled ? " · offers on" : ""}</>}
              </p>
              {l.moderation_reason && <p className="text-[13px] text-slab">Taken down: {l.moderation_reason}</p>}
            </div>
            {canModerate && (l.status === "blocked" ? l.format !== "auction" : l.status !== "removed") && (
              <div className="sm:w-[260px]"><ModerateListing id={l.id} action={l.status === "blocked" ? "restore" : "remove"} /></div>
            )}
          </li>
        ))}
      </ul>
      {rows.length === 0 && <p className="py-8 text-center text-muted">No listings match.</p>}
      <div className="mt-4 flex gap-4 text-[14px]">
        {page > 1 && <Link href={link(page - 1)} className="text-link underline">Previous</Link>}
        {page * 50 < total && <Link href={link(page + 1)} className="text-link underline">Next</Link>}
      </div>
    </div>
  );
}
