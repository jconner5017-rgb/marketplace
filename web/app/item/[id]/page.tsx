import Link from "next/link";
import { notFound } from "next/navigation";
import type { Metadata } from "next";
import { getListing, getSummary, search } from "@/lib/api";
import ItemActions from "@/components/ItemActions";
import ContactSeller from "@/components/ContactSeller";
import { GradeBadge, ListingTile } from "@/components/CardBits";
import Gallery from "@/components/Gallery";
import { ATTRIBUTES, CONDITIONS, gradeLabel, money } from "@/lib/format";

export const dynamic = "force-dynamic";

type Props = { params: Promise<{ id: string }> };

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const l = await getListing((await params).id);
  return { title: l ? `${l.title} — ${money(l.price_cents)}` : "Listing not found" };
}

export default async function ItemPage({ params }: Props) {
  const l = await getListing((await params).id);
  if (!l) notFound();

  const [more, me] = await Promise.all([search(`seller=${encodeURIComponent(l.seller_username)}&limit=7`), getSummary()]);
  const others = more.listings.filter((x) => x.id !== l.id).slice(0, 6);
  const gameHref = `/search?category=${l.category}&game=${encodeURIComponent(l.game_or_sport)}`;

  const specifics: [string, string | null][] = [
    ["Category", l.category === "tcg" ? "Trading card game" : "Sports card"],
    [l.category === "tcg" ? "Game" : "Sport", l.game_or_sport],
    ["Set", l.set_name],
    ["Year", l.year ? String(l.year) : null],
    ["Card number", l.card_number],
    ["Graded", l.grader ? "Yes" : "No"],
    ["Grader", l.grader],
    ["Grade", l.grader ? gradeLabel(l.grade) : null],
    ["Certification number", l.cert_number],
    ["Condition", l.condition ? CONDITIONS[l.condition] ?? l.condition : null],
    ["Features", l.attributes.length ? l.attributes.map((a) => ATTRIBUTES[a] ?? a).join(", ") : null],
  ];

  return (
    <div className="pt-4">
      <nav aria-label="Breadcrumb" className="text-[13px] text-muted">
        <Link href="/" className="hover:underline">Home</Link>
        <span className="mx-1.5">›</span>
        <Link href={`/search?category=${l.category}`} className="hover:underline">
          {l.category === "tcg" ? "Trading card games" : "Sports cards"}
        </Link>
        <span className="mx-1.5">›</span>
        <Link href={gameHref} className="hover:underline">{l.game_or_sport}</Link>
        <span className="mx-1.5">›</span>
        <Link href={`${gameHref}&set=${encodeURIComponent(l.set_name)}`} className="hover:underline">{l.set_name}</Link>
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

          <p className="num mt-4 text-[32px] font-extrabold leading-none">{money(l.price_cents)}</p>
          <p className="mt-1 text-[14px] text-ship">Free shipping · USPS</p>

          <ItemActions id={l.id} signedIn={!!me} quantity={l.quantity} />

          <div className="mt-6 rounded-lg border border-line p-4 text-[14px]">
            <p>
              Sold by <Link href={`/search?seller=${encodeURIComponent(l.seller_username)}`} className="font-semibold text-link hover:underline">{l.seller_username}</Link>
            </p>
            <p className="mt-1 text-muted">Ships from the United States. Delivery to US addresses only.</p>
            {l.grader ? (
              <p className="mt-1 text-muted">Graded cards ship in a padded mailer with tracking.</p>
            ) : (
              <p className="mt-1 text-muted">Raw cards ship sleeved and protected.</p>
            )}
            <ContactSeller id={l.id} seller={l.seller_username} signedIn={!!me} />
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
              <p className="mt-3 max-w-[70ch] whitespace-pre-line text-[15px]">{l.description}</p>
            </>
          )}
        </div>
      </section>

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
