import Link from "next/link";
import { notFound } from "next/navigation";
import type { Metadata } from "next";
import { apiGet, getBids, getListing, getSummary, search } from "@/lib/api";
import type { FeedbackSummary } from "@/lib/types";
import { BadgeList, SellerLine } from "@/components/feedback/Bits";
import ItemActions from "@/components/ItemActions";
import AuctionPanel, { BidList } from "@/components/AuctionPanel";
import ContactSeller from "@/components/ContactSeller";
import ReportButton from "@/components/ReportButton";
import { GradeBadge, ListingTile, Price } from "@/components/CardBits";
import Gallery from "@/components/Gallery";
import { CONDITIONS, categoryInfo, gradeLabel, landingHref, money } from "@/lib/format";
import { getCatalog, labelFor } from "@/lib/catalog";
import { SITE_URL, describeListing, ldJson, productJsonLd } from "@/lib/seo";

export const dynamic = "force-dynamic";

type Props = { params: Promise<{ id: string }> };

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const [l, cat] = await Promise.all([getListing((await params).id), getCatalog()]);
  if (!l) return { title: "Listing not found", robots: { index: false } };
  const sold = l.status === "sold_out";
  const title = sold ? `${l.title} — sold` : l.format === "auction" ? `${l.title} — auction, ${money(l.price_cents)}` : `${l.title} — ${money(l.price_cents)}`;
  const description = describeListing(l, labelFor(cat, l.game_or_sport));
  const url = `${SITE_URL}/item/${l.id}`;
  return {
    title,
    description,
    alternates: { canonical: url },
    openGraph: { type: "website", url, title, description, images: l.photo_urls.slice(0, 1) },
    twitter: { card: "summary_large_image", title, description, images: l.photo_urls.slice(0, 1) },
    // Finished auctions stay viewable for bidders but leave search results.
    // Sold cards stay indexed: their pages keep earned rankings and point to similar cards.
    ...(l.status !== "active" && !sold ? { robots: { index: false, follow: true } } : {}),
  };
}

export default async function ItemPage({ params }: Props) {
  const l = await getListing((await params).id, true);
  if (!l) notFound();

  const isAuction = l.format === "auction" && !!l.auction;
  const sold = l.status === "sold_out";
  const game = encodeURIComponent(l.game_or_sport);
  const [more, me, bids, catalog, rep, bySet, byGame, storeR] = await Promise.all([
    search(`seller=${encodeURIComponent(l.seller_username)}&limit=7`),
    getSummary(),
    isAuction ? getBids(l.id) : Promise.resolve(null),
    getCatalog(),
    apiGet<{ summary: FeedbackSummary }>(`/v1/members/${encodeURIComponent(l.seller_username)}/feedback?role=seller&limit=1`),
    // A sold card's page points to similar cards still for sale: same set first, then the same sport or game.
    sold && l.set_name ? search(`category=${l.category}&game=${game}&set=${encodeURIComponent(l.set_name)}&limit=12`) : Promise.resolve(null),
    sold ? search(`category=${l.category}&game=${game}&limit=12`) : Promise.resolve(null),
    apiGet<{ store: { slug: string; name: string } | null }>(`/v1/stores/by-seller/${encodeURIComponent(l.seller_username)}`),
  ]);
  const store = storeR.data?.store ?? null;
  const similar = [...(bySet?.listings ?? []), ...(byGame?.listings ?? [])]
    .filter((x, i, all) => x.id !== l.id && all.findIndex((y) => y.id === x.id) === i)
    .slice(0, 6);
  const others = more.listings.filter((x) => x.id !== l.id).slice(0, 6);
  const gameHref = landingHref(l.category, l.game_or_sport);
  const setHref = `/search?category=${l.category}&game=${encodeURIComponent(l.game_or_sport)}&set=${encodeURIComponent(l.set_name)}`;

  const specifics: [string, string | null][] = [
    ["Category", categoryInfo(l.category)?.one ?? "Card"],
    [categoryInfo(l.category)?.subject ?? "Type", labelFor(catalog, l.game_or_sport)],
    ["Set", l.set_name],
    ["Year", l.year ? String(l.year) : null],
    ["Card number", l.card_number],
    ["Graded", l.grader ? "Yes" : "No"],
    ["Grader", l.grader],
    ["Grade", l.grader ? gradeLabel(l.grade) : null],
    ["Certification number", l.cert_number],
    ["Condition", l.condition ? CONDITIONS[l.condition] ?? l.condition : null],
    ["Features", l.attributes.length ? l.attributes.map((a) => catalog.features[a] ?? a).join(", ") : null],
  ];

  return (
    <div className="pt-4">
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: ldJson(productJsonLd(l, labelFor(catalog, l.game_or_sport))) }} />
      <nav aria-label="Breadcrumb" className="text-[13px] text-muted">
        <Link href="/" className="hover:underline">Home</Link>
        <span className="mx-1.5">›</span>
        <Link href={landingHref(l.category)} className="hover:underline">
          {categoryInfo(l.category)?.label ?? "Cards"}
        </Link>
        <span className="mx-1.5">›</span>
        <Link href={gameHref} className="hover:underline">{l.game_or_sport}</Link>
        <span className="mx-1.5">›</span>
        <Link href={setHref} className="hover:underline">{l.set_name}</Link>
      </nav>

      <div className="mt-4 grid gap-8 lg:grid-cols-[minmax(0,1fr)_420px]">
        <Gallery photos={l.photo_urls} title={l.title} />

        <div>
          <h1 className="title-tight text-[26px] font-bold leading-tight">{l.title}</h1>
          <p className="mt-1 text-[14px] text-muted">
            {[l.year, l.set_name, l.card_number ? `#${l.card_number}` : null].filter(Boolean).join(" · ")}
          </p>

          <div className="mt-4 flex items-center gap-3 border-b border-line pb-4">
            <GradeBadge l={l} size="lg" />
            {l.grader && l.cert_number && (
              <span className="text-[13px] text-muted">Cert #{l.cert_number}</span>
            )}
          </div>

          {sold ? (
            <div className="mt-4 rounded-lg border border-line bg-wash p-4">
              <p className="text-[20px] font-bold">This card has sold</p>
              <p className="num mt-1 text-[14px] text-muted">Last listed at {money(l.price_cents)}</p>
              <Link href={gameHref} className="mt-3 inline-block rounded-full bg-ink px-4 py-2 text-[14px] font-semibold text-white hover:opacity-90">
                Shop {labelFor(catalog, l.game_or_sport)} cards
              </Link>
            </div>
          ) : isAuction && bids ? (
            <>
              <AuctionPanel l={l} history={bids} signedIn={!!me} />
              <p className="mt-2 text-[14px] text-ship">Free shipping · USPS</p>
            </>
          ) : (
            <>
              <Price l={l} className="num mt-4 text-[32px] font-extrabold leading-none" />
              <p className="mt-1 text-[14px] text-muted">{l.offers_enabled ? "Buy It Now or Best Offer" : "Buy It Now"}</p>
              <p className="mt-1 text-[14px] text-ship">Free shipping · USPS</p>
              <ItemActions id={l.id} signedIn={!!me} quantity={l.quantity} price={l.price_cents} offers={l.offers_enabled} />
            </>
          )}

          <div className="mt-6 rounded-lg border border-line p-4 text-[14px]">
            <p>
              Sold by {rep.data ? <SellerLine s={rep.data.summary} /> : (
                <Link href={`/seller/${encodeURIComponent(l.seller_username)}`} className="font-semibold text-link hover:underline">{l.seller_username}</Link>
              )}
            </p>
            {rep.data && <BadgeList s={rep.data.summary} />}
            {store && (
              <p className="mt-2"><Link href={`/store/${store.slug}`} className="font-semibold text-link hover:underline">Visit {store.name}</Link></p>
            )}
            <p className="mt-1 text-muted">Ships from the United States. Delivery to US addresses only.</p>
            {l.grader ? (
              <p className="mt-1 text-muted">Graded cards ship in a padded mailer with tracking.</p>
            ) : (
              <p className="mt-1 text-muted">Raw cards ship sleeved and protected.</p>
            )}
            {!sold && <ContactSeller id={l.id} seller={l.seller_username} signedIn={!!me} />}
          </div>
          <div className="mt-3">
            <ReportButton type="listing" id={l.id} label="Report this listing" signedIn={!!me} next={`/item/${l.id}`} />
          </div>
        </div>
      </div>

      <section className="mt-10 grid gap-8 lg:grid-cols-[minmax(0,1fr)_420px]">
        <div>
          <h2 className="border-b border-line pb-2 text-[20px] font-bold">Item specifics</h2>
          <dl className="mt-3 grid grid-cols-1 gap-x-8 sm:grid-cols-2">
            {specifics
              .filter(([, v]) => v)
              .map(([k, v]) => (
                <div key={k} className="grid grid-cols-[150px_1fr] gap-2 border-b border-wash py-2 text-[14px]">
                  <dt className="text-muted">{k}</dt>
                  <dd>{v}</dd>
                </div>
              ))}
          </dl>
          {l.description && (
            <>
              <h2 className="mt-8 border-b border-line pb-2 text-[20px] font-bold">Seller&apos;s description</h2>
              {l.description_html
                ? /* Cleaned on the server to a short list of plain tags (no styles, scripts or images). */
                  <div className="rich-desc mt-3 max-w-[78ch] text-[15px]" dangerouslySetInnerHTML={{ __html: l.description_html }} />
                : <p className="mt-3 max-w-[70ch] whitespace-pre-line text-[15px]">{l.description}</p>}
            </>
          )}
          {isAuction && bids && <BidList history={bids} auction={l.auction!} />}
        </div>
      </section>

      {similar.length > 0 && (
        <section className="mt-12">
          <div className="flex items-baseline justify-between border-b border-line pb-2">
            <h2 className="text-[20px] font-bold">Similar cards for sale</h2>
            <Link href={gameHref} className="text-[14px] text-link hover:underline">See all</Link>
          </div>
          <div className="mt-4 grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
            {similar.map((x) => <ListingTile key={x.id} l={x} />)}
          </div>
        </section>
      )}

      {others.length > 0 && (
        <section className="mt-12">
          <div className="flex items-baseline justify-between border-b border-line pb-2">
            <h2 className="text-[20px] font-bold">More from this seller</h2>
            <Link href={`/search?seller=${encodeURIComponent(l.seller_username)}`} className="text-[14px] text-link hover:underline">
              See all
            </Link>
          </div>
          <div className="mt-4 grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
            {others.map((x) => <ListingTile key={x.id} l={x} />)}
          </div>
        </section>
      )}
    </div>
  );
}
