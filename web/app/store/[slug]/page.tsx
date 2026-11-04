import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGet, getSummary, search } from "@/lib/api";
import { SITE_URL } from "@/lib/seo";
import type { FeedbackSummary, Store } from "@/lib/types";
import { ListingTile } from "@/components/CardBits";
import { BadgeList, SellerLine } from "@/components/feedback/Bits";
import FollowStoreButton from "@/components/FollowStoreButton";

export const dynamic = "force-dynamic";

type Props = { params: Promise<{ slug: string }>; searchParams: Promise<Record<string, string | undefined>> };

async function load(slug: string) {
  return (await apiGet<{ store: Store }>(`/v1/stores/${encodeURIComponent(slug)}`)).data?.store ?? null;
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const st = await load((await params).slug);
  if (!st) return { title: "Store not found" };
  return {
    title: `${st.name}: store`,
    description: st.tagline || `Shop ${st.name} on Slabs & Singles: ${st.listings} cards for sale.`,
    alternates: { canonical: `${SITE_URL}/store/${st.slug}` },
  };
}

const SORTS = [["newest", "Newly listed"], ["price_asc", "Price: low to high"], ["price_desc", "Price: high to low"], ["ending", "Auctions ending soonest"]] as const;
const PAGE = 48;

export default async function StorePage({ params, searchParams }: Props) {
  const { slug } = await params;
  const sp = await searchParams;
  const st = await load(slug);
  if (!st) notFound();
  const cat = Number(sp.cat) || 0;
  const q = (sp.q ?? "").slice(0, 100);
  const sort = SORTS.some(([v]) => v === sp.sort) ? sp.sort! : sp.sort === "new" ? "newest" : "newest";
  const onSale = sp.sale === "1";
  const tab = sp.tab === "about" ? "about" : sp.tab === "policies" ? "policies" : "shop";
  const page = Math.max(1, Number(sp.page) || 1);
  const qs = new URLSearchParams({ seller: st.username, sort, limit: String(PAGE), offset: String((page - 1) * PAGE) });
  if (cat) qs.set("store_category", String(cat));
  if (q) qs.set("q", q);
  if (onSale) qs.set("on_sale", "1");
  const [res, me, repR] = await Promise.all([
    tab === "shop" ? search(qs.toString()) : Promise.resolve(null),
    getSummary(),
    apiGet<{ summary: FeedbackSummary }>(`/v1/members/${encodeURIComponent(st.username)}/feedback?role=seller&limit=1`),
  ]);
  const rep = repR.data?.summary;
  const href = (p: Record<string, string | number | undefined>) => {
    const v: Record<string, string | number | undefined> = { cat: cat || undefined, q: q || undefined, sort: sort === "newest" ? undefined : sort, sale: onSale ? "1" : undefined, ...p };
    const u = new URLSearchParams();
    for (const [k, x] of Object.entries(v)) if (x !== undefined && x !== "" && x !== 0) u.set(k, String(x));
    const s = u.toString();
    return `/store/${st.slug}${s ? "?" + s : ""}`;
  };
  const sale = st.sale;
  const ends = sale ? new Date(sale.ends_at).toLocaleDateString("en-US", { month: "long", day: "numeric" }) : "";
  const total = res?.total ?? 0;

  return (
    <div className="pb-12 pt-4">
      {/* Banner and header */}
      <div className="overflow-hidden rounded-xl border border-line">
        {st.banner_url
          ? /* eslint-disable-next-line @next/next/no-img-element */ <img src={st.banner_url} alt="" className="h-[140px] w-full object-cover sm:h-[200px]" />
          : <div className="h-[80px] w-full bg-wash sm:h-[120px]" />}
        <div className="flex flex-wrap items-end justify-between gap-4 p-4">
          <div className="flex items-end gap-4">
            <div className="-mt-14 h-24 w-24 shrink-0 overflow-hidden rounded-xl border-4 border-paper bg-wash shadow-sm sm:h-28 sm:w-28">
              {st.logo_url
                ? /* eslint-disable-next-line @next/next/no-img-element */ <img src={st.logo_url} alt={`${st.name} logo`} className="h-full w-full object-cover" />
                : <div className="flex h-full w-full items-center justify-center text-[32px] font-bold text-muted">{st.name.slice(0, 1).toUpperCase()}</div>}
            </div>
            <div>
              <h1 className="text-[26px] font-bold leading-tight">{st.name}</h1>
              {st.tagline && <p className="text-muted">{st.tagline}</p>}
              <p className="mt-1 text-[14px]">{rep ? <SellerLine s={rep} /> : st.username} · <span className="num">{st.listings.toLocaleString()}</span> cards for sale</p>
              {rep && <BadgeList s={rep} />}
            </div>
          </div>
          <FollowStoreButton slug={st.slug} following={st.following} followers={st.followers} signedIn={!!me} />
        </div>
      </div>

      {sale && (
        <Link href={href({ sale: "1", page: undefined })} className="mt-4 block rounded-xl bg-slab px-4 py-3 text-white hover:opacity-95">
          <strong>{sale.name}: {sale.percent}% off</strong>{sale.category ? ` ${sale.category}` : " storewide"} · ends {ends}. Prices shown already include the sale.
        </Link>
      )}

      <nav aria-label="Store" className="mt-4 flex gap-1 border-b border-line text-[15px]">
        {[["shop", "Shop"], ["about", "About"], ["policies", "Policies"]].map(([k, label]) => (
          <Link key={k} href={k === "shop" ? `/store/${st.slug}` : `/store/${st.slug}?tab=${k}`} aria-current={tab === k ? "page" : undefined}
            className={`border-b-2 px-3 py-2 ${tab === k ? "border-ink font-semibold" : "border-transparent text-muted hover:text-ink"}`}>{label}</Link>
        ))}
      </nav>

      {tab === "about" && (
        <section className="mt-4 max-w-[68ch] whitespace-pre-line text-[16px] leading-relaxed">
          {st.about || "This store hasn't written an introduction yet."}
          <p className="mt-4 text-[14px] text-muted">Selling on Slabs & Singles since {new Date(st.created_at).toLocaleDateString("en-US", { month: "long", year: "numeric" })}.
            {" "}<Link href={`/seller/${encodeURIComponent(st.username)}`} className="text-link underline">Seller feedback</Link></p>
        </section>
      )}
      {tab === "policies" && (
        <section className="mt-4 max-w-[68ch] text-[16px] leading-relaxed">
          <p className="whitespace-pre-line">{st.policies || "This store follows the standard Slabs & Singles policies."}</p>
          <p className="mt-4 text-[14px] text-muted">Every order is also covered by the <Link href="/help/user-agreement" className="text-link underline">User Agreement</Link>: free shipping, shipped within 3 business days, and card-issuer protection when you pay on the site.</p>
        </section>
      )}

      {tab === "shop" && (
        <div className="mt-4 grid gap-8 md:grid-cols-[220px_1fr]">
          <aside>
            <form action={`/store/${st.slug}`} className="flex gap-2 text-[14px]">
              {cat > 0 && <input type="hidden" name="cat" value={cat} />}
              <label htmlFor="sq" className="sr-only">Search this store</label>
              <input id="sq" name="q" defaultValue={q} placeholder="Search this store" className="min-w-0 flex-1 rounded-lg border border-line px-3 py-2" />
              <button className="rounded-lg border border-ink px-3 py-2 font-semibold hover:bg-wash">Go</button>
            </form>
            <h2 className="mt-5 text-[14px] font-bold">Categories</h2>
            <ul className="mt-2 space-y-1 text-[14px]">
              <li><Link href={href({ cat: undefined, page: undefined })} className={cat === 0 ? "font-semibold" : "text-link hover:underline"}>All cards</Link></li>
              {st.categories.filter((c) => c.listings > 0).map((c) => (
                <li key={c.id}>
                  <Link href={href({ cat: c.id, page: undefined })} className={cat === c.id ? "font-semibold" : "text-link hover:underline"}>{c.name}</Link>
                  <span className="num text-muted"> ({c.listings})</span>
                </li>
              ))}
              {sale && <li className="pt-2"><Link href={href({ sale: onSale ? undefined : "1", page: undefined })} className={onSale ? "font-semibold text-slab" : "text-slab hover:underline"}>On sale</Link></li>}
            </ul>
          </aside>
          <section>
            <div className="flex flex-wrap items-center justify-between gap-3 text-[14px]">
              <p className="text-muted"><span className="num">{total.toLocaleString()}</span> {total === 1 ? "card" : "cards"}</p>
              <nav aria-label="Sort" className="flex flex-wrap gap-3">
                {SORTS.map(([v, label]) => (
                  <Link key={v} href={href({ sort: v === "newest" ? undefined : v, page: undefined })} aria-current={sort === v ? "true" : undefined}
                    className={sort === v ? "font-semibold" : "text-link hover:underline"}>{label}</Link>
                ))}
              </nav>
            </div>
            {!res || res.listings.length === 0 ? (
              <p className="py-16 text-center text-muted">{q || cat || onSale ? "No cards match." : "No cards for sale right now. Follow the store to hear when new ones are listed."}</p>
            ) : (
              <div className="mt-4 grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 lg:grid-cols-4">
                {res.listings.map((l) => <ListingTile key={l.id} l={l} />)}
              </div>
            )}
            {total > PAGE && (
              <nav aria-label="Pages" className="mt-6 flex items-center gap-3 text-[14px]">
                {page > 1 && <Link href={href({ page: page - 1 })} className="text-link hover:underline">← Previous</Link>}
                <span className="text-muted">Page {page} of {Math.ceil(total / PAGE)}</span>
                {page * PAGE < total && <Link href={href({ page: page + 1 })} className="text-link hover:underline">Next →</Link>}
              </nav>
            )}
          </section>
        </div>
      )}
    </div>
  );
}
