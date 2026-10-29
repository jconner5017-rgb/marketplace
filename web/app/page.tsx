import Link from "next/link";
import { search } from "@/lib/api";
import { ListingTile } from "@/components/CardBits";
import type { Listing } from "@/lib/types";
import { gameHref, sportHref } from "@/lib/format";
import { getCatalog } from "@/lib/catalog";
import { SITE_URL, ldJson } from "@/lib/seo";
import { SITE_NAME } from "@/lib/format";

export const dynamic = "force-dynamic";

export default async function Home() {
  const [ending, fresh, graded, cheap, cat] = await Promise.all([
    search("format=auction&sort=ending&limit=6"),
    search("sort=newest&limit=12"),
    search("min_grade=1&sort=newest&limit=6"),
    search("max_price=20&sort=price_asc&limit=6"),
    getCatalog(),
  ]);
  const SHELVES = [
    { title: "Sports cards", category: "sports", items: cat.sports.map((s) => ({ label: s.short, href: sportHref(s.value) })) },
    { title: "Trading card games", category: "tcg", items: cat.games.map((g) => ({ label: g.short, href: gameHref(g.value) })) },
  ];

  const site = {
    "@context": "https://schema.org",
    "@type": "WebSite",
    name: SITE_NAME,
    url: SITE_URL,
    potentialAction: { "@type": "SearchAction", target: `${SITE_URL}/search?q={search_term_string}`, "query-input": "required name=search_term_string" },
  };
  return (
    <div className="pt-6">
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: ldJson(site) }} />
      {/* Opening band: what the place is, in one line, plus the two ways in. */}
      <section className="grid gap-4 lg:grid-cols-[1.1fr_1fr]">
        <div className="rounded-lg bg-ink px-6 py-7 text-white">
          <h1 className="text-[28px] font-extrabold leading-tight" style={{ fontStretch: "94%" }}>
            Slabs and singles, sports and TCG. Every card ships free.
          </h1>
          <p className="mt-2 max-w-[52ch] text-[15px] text-white/80">
            Cards up to $1,000 from US sellers, paid through Stripe. Grading, condition and cert number shown
            on every listing.
          </p>
          <div className="mt-5 flex flex-wrap gap-3">
            <Link href="/search?sort=newest" className="rounded-full bg-white px-5 py-2 font-semibold text-ink hover:bg-wash">
              Shop new listings
            </Link>
            <Link href="/sell" className="rounded-full border border-white/60 px-5 py-2 font-semibold hover:bg-white/10">
              Start selling
            </Link>
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          {SHELVES.map((s) => (
            <div key={s.title} className="rounded-lg border border-line p-4">
              <h2 className="font-bold">
                <Link href={`/search?category=${s.category}`} className="hover:underline">{s.title}</Link>
              </h2>
              <ul className="mt-2 grid grid-cols-2 gap-x-3 gap-y-1 text-[14px]">
                {s.items.map((g) => (
                  <li key={g.href}>
                    <Link href={g.href} className="text-link hover:underline">{g.label}</Link>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      </section>

      {ending.listings.length > 0 && (
        <Shelf title="Auctions ending soon" href="/search?format=auction&sort=ending" items={ending.listings} />
      )}
      <Shelf title="Newly listed" href="/search?sort=newest" items={fresh.listings} />
      <Shelf title="Graded slabs" href="/search?min_grade=1" items={graded.listings} />
      <Shelf title="Singles under $20" href="/search?max_price=20&sort=price_asc" items={cheap.listings} />

      {fresh.failed && (
        <p role="alert" className="mt-10 rounded-lg border border-line p-6 text-center text-muted">
          We couldn&apos;t load cards just now. <Link href="/" className="text-link underline">Try again</Link> in a moment.
        </p>
      )}
      {!fresh.failed && fresh.total === 0 && (
        <p className="mt-10 rounded-lg border border-line p-6 text-center text-muted">
          No cards are listed yet. <Link href="/sell" className="text-link underline">List the first one</Link>.
        </p>
      )}
    </div>
  );
}

function Shelf({ title, href, items }: { title: string; href: string; items: Listing[] }) {
  if (items.length === 0) return null;
  return (
    <section className="mt-10">
      <div className="flex items-baseline justify-between border-b border-line pb-2">
        <h2 className="text-[20px] font-bold">{title}</h2>
        <Link href={href} className="text-[14px] text-link hover:underline">See all</Link>
      </div>
      <div className="mt-4 grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
        {items.map((l) => (
          <ListingTile key={l.id} l={l} />
        ))}
      </div>
    </section>
  );
}
