import Link from "next/link";
import { search } from "@/lib/api";
import { ListingTile } from "@/components/CardBits";
import type { Listing } from "@/lib/types";
import { GAMES, SPORTS, gameHref, sportHref } from "@/lib/format";

export const dynamic = "force-dynamic";

const SHELVES = [
  { title: "Sports cards", category: "sports", items: SPORTS.map((s) => ({ label: s.value, href: sportHref(s.value) })) },
  { title: "Trading card games", category: "tcg", items: GAMES.map((g) => ({ label: g.short, href: gameHref(g.value) })) },
];

export default async function Home() {
  const [fresh, graded, cheap] = await Promise.all([
    search("sort=newest&limit=12"),
    search("min_grade=1&sort=newest&limit=6"),
    search("max_price=20&sort=price_asc&limit=6"),
  ]);

  return (
    <div className="pt-6">
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

      <Shelf title="Newly listed" href="/search?sort=newest" items={fresh.listings} />
      <Shelf title="Graded slabs" href="/search?min_grade=1" items={graded.listings} />
      <Shelf title="Singles under $20" href="/search?max_price=20&sort=price_asc" items={cheap.listings} />

      {fresh.total === 0 && (
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
