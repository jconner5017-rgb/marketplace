import Link from "next/link";
import { notFound } from "next/navigation";
import type { Metadata } from "next";
import { apiGet, getBids, getListing, getSummary, search } from "@/lib/api";
import type { FeedbackEntry, FeedbackSummary } from "@/lib/types";
import ShareButton from "@/components/ShareButton";
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
    apiGet<{ summary: FeedbackSummary; feedback: FeedbackEntry[] }>(`/v1/members/${encodeURIComponent(l.seller_username)}/feedback?role=seller&limit=5`),
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
        {/* Top of its column, level with the title. Scrolls with the page (not pinned). */}
        <div className="self-start">
          <Gallery photos={l.photo_urls} title={l.title} />
          {/* Like eBay: invite visitors to sell (gets new sellers signed up), and Share on the right. */}
          <div className="mt-4 flex flex-wrap items-center gap-2 border-b border-line pb-4">
            <span className="text-[14px] text-muted">Have one to sell?</span>
            {!l.lot_count && (
              <Link href={`/sell/new?like=${l.id}`} className="rounded-full border border-line px-4 py-2 text-[14px] font-semibold hover:bg-wash">Sell one like this</Link>
            )}
            <Link href="/sell" className="rounded-full border border-line px-4 py-2 text-[14px] font-semibold hover:bg-wash">Sell something else</Link>
            <div className="ml-auto">
              <ShareButton url={`${SITE_URL}/item/${l.id}`} title={l.title} image={l.photo_urls[0]} />
            </div>
          </div>
        </div>

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
              <p className="mt-2 text-[14px] text-ship">Free shipping · USPS · Usually arrives in 3–7 days</p>
            </>
          ) : (
            <>
              <Price l={l} className="num mt-4 text-[32px] font-extrabold leading-none" />
              <p className="mt-1 text-[14px] text-muted">{l.offers_enabled ? "Buy It Now or Best Offer" : "Buy It Now"}</p>
              <p className="mt-1 text-[14px] text-ship">Free shipping · USPS · Usually arrives in 3–7 days</p>
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
            {!sold && <ContactSeller id={l.id} seller={l.seller_username} signedIn={!!me} />}
          </div>
          <section aria-labelledby="srp" className="mt-6 rounded-lg border border-line p-4 text-[14px]">
            <h2 id="srp" className="text-[17px] font-bold">Shipping, returns and payments</h2>
            <dl className="mt-3 space-y-3">
              <div className="grid grid-cols-[88px_1fr] gap-3">
                <dt className="text-muted">Shipping</dt>
                <dd>
                  <span className="font-semibold text-ship">Free</span> · USPS
                  <div className="text-muted">Usually arrives in 3–7 days. Ships within 3 business days of payment, from the United States to US addresses only.</div>
                  <div className="text-muted">{l.grader ? "Graded cards ship in a padded mailer with tracking." : "Raw cards ship sleeved and protected."}</div>
                </dd>
              </div>
              <div className="grid grid-cols-[88px_1fr] gap-3">
                <dt className="text-muted">Returns</dt>
                <dd>
                  {l.returns_policy === "30_days"
                    ? <><span className="font-semibold">30-day returns.</span> Return for any reason within 30 days of delivery, no questions asked.</>
                    : <><span className="font-semibold">Returns only if defective or not as described,</span> within 30 days of delivery.</>}
                  <div className="text-muted">
                    Full refund of the price paid; the buyer pays return shipping. <Link href="/help/returns" className="text-link hover:underline">How returns work</Link>
                  </div>
                </dd>
              </div>
              <div className="grid grid-cols-[88px_1fr] gap-3">
                <dt className="text-muted">Payments</dt>
                <dd>
                  <span className="font-semibold">Secure checkout with Stripe</span>
                  <div className="text-muted">Pay by card, Apple Pay or Google Pay. Your card details go to Stripe, never to the seller.</div>
                </dd>
              </div>
            </dl>
          </section>

          <div className="mt-3">
            <ReportButton type="listing" id={l.id} label="Report this listing" signedIn={!!me} next={`/item/${l.id}`} />
          </div>
        </div>
      </div>

      {/* Boxed like eBay's "About this item": a tab label, then specifics and the seller's description. */}
      <section className="mt-10" aria-labelledby="about-item">
        <div className="relative z-[1] -mb-px inline-block rounded-t-lg border border-b-0 border-line bg-paper px-5 py-2.5">
          <h2 id="about-item" className="text-[14px] font-bold text-link">About this item</h2>
        </div>
        <div className="rounded-b-lg rounded-tr-lg border border-line bg-paper p-5 sm:p-6">
          <p className="text-[13px] text-muted">Seller assumes all responsibility for this listing.</p>
          <h3 className="mt-5 text-[22px] font-bold">Item specifics</h3>
          <dl className="mt-3 grid grid-cols-1 gap-x-10 md:grid-cols-2">
            {specifics
              .filter(([, v]) => v)
              .map(([k, v]) => (
                <div key={k} className="grid grid-cols-[150px_1fr] gap-3 py-1.5 text-[14px] sm:grid-cols-[180px_1fr]">
                  <dt className="text-muted">{k}</dt>
                  <dd>{v}</dd>
                </div>
              ))}
          </dl>
          {l.description && (
            <>
              <h3 className="mt-8 text-[22px] font-bold">Item description from the seller</h3>
              {l.description_html
                ? /* Cleaned on the server to a short list of plain tags (no styles, scripts or images). */
                  <div className="rich-desc mt-3 text-[15px]" dangerouslySetInnerHTML={{ __html: l.description_html }} />
                : <p className="mt-3 whitespace-pre-line text-[15px]">{l.description}</p>}
            </>
          )}
          {isAuction && bids && <BidList history={bids} auction={l.auction!} />}
        </div>
      </section>

      {rep.data && <AboutSeller s={rep.data.summary} feedback={rep.data.feedback ?? []} store={store} />}

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

const STARS: [keyof FeedbackSummary, string][] = [
  ["item_avg", "Accurate description"],
  ["shipping_avg", "Shipping speed"],
  ["communication_avg", "Communication"],
];

/** About the seller and their latest feedback, like a marketplace seller card. */
function AboutSeller({ s, feedback, store }: {
  s: FeedbackSummary; feedback: FeedbackEntry[]; store: { slug: string; name: string } | null;
}) {
  const joined = new Date(s.member_since).toLocaleDateString("en-US", { month: "short", year: "numeric" });
  const seller = `/seller/${encodeURIComponent(s.username)}`;
  return (
    <section className="mt-12 grid gap-8 border-t border-line pt-8 lg:grid-cols-[360px_minmax(0,1fr)]">
      <div>
        <h2 className="text-[20px] font-bold">About this seller</h2>
        <div className="mt-4 flex items-center gap-4">
          <div aria-hidden="true" className="flex h-16 w-16 flex-none items-center justify-center rounded-full bg-ink text-[26px] font-bold text-white">
            {s.username.charAt(0).toUpperCase()}
          </div>
          <div>
            <Link href={seller} className="text-[18px] font-bold hover:underline">{store?.name ?? s.username}</Link>
            <p className="num text-[14px] text-muted">
              {s.positive_percent != null ? `${s.positive_percent.toFixed(1)}% positive feedback` : "No feedback yet"} · {s.sales.toLocaleString()} sold
            </p>
          </div>
        </div>
        <p className="mt-3 text-[14px] text-muted">Joined {joined}</p>
        <BadgeList s={s} />
        <div className="mt-4 flex flex-col gap-2">
          {store && (
            <Link href={`/store/${store.slug}`} className="rounded-full bg-link px-5 py-2.5 text-center font-semibold text-white hover:bg-link-dark">Visit store</Link>
          )}
          <Link href={seller} className="rounded-full border border-line px-5 py-2.5 text-center font-semibold hover:bg-wash">Seller profile</Link>
        </div>
        {STARS.some(([k]) => s[k] != null) && (
          <div className="mt-6">
            <h3 className="font-bold">Detailed seller ratings</h3>
            <dl className="mt-2 space-y-1.5 text-[14px]">
              {STARS.map(([k, label]) => {
                const v = s[k] as number | null;
                if (v == null) return null;
                return (
                  <div key={k} className="grid grid-cols-[1fr_96px_32px] items-center gap-3">
                    <dt>{label}</dt>
                    <dd aria-hidden="true" className="h-1.5 rounded-full bg-wash">
                      <div className="h-1.5 rounded-full bg-ink" style={{ width: `${Math.max(0, Math.min(100, (v / 5) * 100))}%` }} />
                    </dd>
                    <dd className="num text-right">{v.toFixed(1)}</dd>
                  </div>
                );
              })}
            </dl>
            <p className="mt-1 text-[12px] text-muted">Average for the last 12 months</p>
          </div>
        )}
      </div>
      <div>
        <div className="flex items-baseline justify-between">
          <h2 className="text-[20px] font-bold">Seller feedback <span className="num font-normal text-muted">({s.total.toLocaleString()})</span></h2>
          <Link href={seller} className="text-[14px] text-link hover:underline">See all</Link>
        </div>
        {feedback.length === 0 ? (
          <p className="mt-4 text-[14px] text-muted">No feedback yet.</p>
        ) : (
          <ul className="mt-4 divide-y divide-wash">
            {feedback.map((f) => (
              <li key={f.id} className="py-3 text-[14px]">
                <p className="text-[13px] text-muted">
                  <span aria-label={f.rating > 0 ? "Positive" : f.rating < 0 ? "Negative" : "Neutral"}
                    className={f.rating > 0 ? "text-ship" : f.rating < 0 ? "text-slab" : ""}>{f.rating > 0 ? "●" : f.rating < 0 ? "●" : "○"}</span>{" "}
                  {f.from} · {new Date(f.created_at).toLocaleDateString("en-US", { month: "short", year: "numeric" })}
                  {f.verified_purchase && " · Verified purchase"}
                </p>
                {f.comment && <p className="mt-1">{f.comment}</p>}
                {f.item_title && <p className="mt-1 truncate text-[13px] text-muted">{f.item_title}</p>}
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
