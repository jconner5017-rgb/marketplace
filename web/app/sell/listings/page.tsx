import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import type { Listing, Store } from "@/lib/types";
import ListingsTable from "@/components/sell/ListingsTable";
import ListingSort from "@/components/sell/ListingSort";
import { LISTING_SORTS } from "@/lib/listingSorts";
import { Suspense } from "react";

export const dynamic = "force-dynamic";
export const metadata = { title: "Your listings", robots: { index: false } };

const VIEWS = [
  { key: "", label: "All" },
  { key: "active", label: "Active" },
  { key: "with_offers", label: "With offers" },
  { key: "auctions", label: "Auctions" },
  { key: "ending_today", label: "Ending today" },
  { key: "unsold", label: "Unsold" },
  { key: "hidden", label: "Hidden" },
  { key: "sold_out", label: "Sold out" },
];
const PAGE = 100;

export default async function SellerListings({ searchParams }: { searchParams: Promise<{ show?: string; q?: string; page?: string; sort?: string }> }) {
  const sp = await searchParams;
  const show = VIEWS.some((v) => v.key === sp.show) ? sp.show! : "";
  const q = typeof sp.q === "string" ? sp.q.slice(0, 100) : "";
  const page = Math.max(1, Number(sp.page) || 1);
  const sort = LISTING_SORTS.some(([v]) => v === sp.sort) && sp.sort !== "newest" ? sp.sort! : "";
  await requireUser("/sell/listings");
  const qs = new URLSearchParams({ limit: String(PAGE), offset: String((page - 1) * PAGE) });
  if (show) qs.set("show", show);
  if (q) qs.set("q", q);
  if (sort) qs.set("sort", sort);
  const [r, storeR, shipR] = await Promise.all([
    apiGet<{ listings: Listing[]; total: number }>(`/v1/seller/listings?${qs}`),
    apiGet<{ store: Store | null }>("/v1/seller/store"),
    apiGet<{ returns_policy?: string }>("/v1/seller/shipping"),
  ]);
  const cats = storeR.data?.store?.status === "active" ? storeR.data.store.categories : [];
  const listings = r.data?.listings ?? [];
  const total = r.data?.total ?? 0;
  const href = (p: Record<string, string | number>) => {
    const u = new URLSearchParams();
    const v = { show, q, sort, page: 1, ...p };
    if (v.show) u.set("show", String(v.show));
    if (v.q) u.set("q", String(v.q));
    if (v.sort) u.set("sort", String(v.sort));
    if (Number(v.page) > 1) u.set("page", String(v.page));
    const s = u.toString();
    return "/sell/listings" + (s ? "?" + s : "");
  };

  return (
    <div className="pb-10 pt-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <h1 className="text-[26px] font-bold">Listings <span className="num text-[18px] font-normal text-muted">({total.toLocaleString()})</span></h1>
        <div className="flex flex-wrap gap-2 text-[14px]">
          <Link href="/sell/import" className="rounded-full border border-link px-4 py-2 font-semibold text-link hover:bg-wash">Upload a CSV</Link>
          <Link href="/sell/new" className="rounded-full bg-link px-4 py-2 font-semibold text-white hover:bg-link-dark">List a card</Link>
        </div>
      </div>
      <nav aria-label="Listing views" className="mt-3 flex gap-1 overflow-x-auto border-b border-line text-[14px]">
        {VIEWS.map((v) => (
          <Link key={v.key} href={href({ show: v.key })} aria-current={show === v.key ? "page" : undefined}
            className={`whitespace-nowrap border-b-2 px-3 py-2 ${show === v.key ? "border-ink font-semibold" : "border-transparent text-muted hover:text-ink"}`}>
            {v.label}
          </Link>
        ))}
      </nav>
      <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
      <form action="/sell/listings" className="flex w-full max-w-[480px] gap-2 text-[14px]">
        {show && <input type="hidden" name="show" value={show} />}
        {sort && <input type="hidden" name="sort" value={sort} />}
        <label htmlFor="lq" className="sr-only">Search your listings</label>
        <input id="lq" name="q" defaultValue={q} placeholder="Search title, set or SKU" className="flex-1 rounded-lg border border-line px-3 py-2" />
        <button className="rounded-lg border border-ink px-4 py-2 font-semibold hover:bg-wash">Search</button>
      </form>
      <Suspense fallback={null}><ListingSort value={sort} /></Suspense>
      </div>

      {listings.length === 0 ? (
        <p className="py-10 text-center text-muted">
          {show || q ? "No listings match." : <>Nothing listed yet. <Link href="/sell/new" className="text-link underline">List your first card</Link> or{" "}
            <Link href="/sell/import" className="text-link underline">upload a CSV</Link>.</>}
        </p>
      ) : (
        <ListingsTable listings={listings} categories={cats} defaultReturns={shipR.data?.returns_policy ?? "damaged_only"} />
      )}
      {total > PAGE && (
        <nav aria-label="Pages" className="mt-4 flex items-center gap-3 text-[14px]">
          {page > 1 && <Link href={href({ page: page - 1 })} className="text-link hover:underline">← Previous</Link>}
          <span className="text-muted">Page {page} of {Math.ceil(total / PAGE)}</span>
          {page * PAGE < total && <Link href={href({ page: page + 1 })} className="text-link hover:underline">Next →</Link>}
        </nav>
      )}
    </div>
  );
}
