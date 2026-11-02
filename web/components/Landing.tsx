// Category landing pages: /sports, /sports/baseball, /tcg/pokemon,
// /nonsport/superheroes. Each is the regular search view with the category
// (and sport, game or theme) filled in, plus a heading, a short intro and
// breadcrumbs, at a clean address search engines can rank.
import Link from "next/link";
import { notFound } from "next/navigation";
import type { Metadata } from "next";
import { SearchView, type SP } from "@/components/SearchView";
import { categoryInfo, landingHref, slugFor, type CategoryKind } from "@/lib/format";
import { choicesFor, getCatalog, type Choice } from "@/lib/catalog";
import { SITE_URL, ldJson } from "@/lib/seo";

// What collectors look for in each, for the intro. Anything not listed gets
// the general wording for its category.
const DETAIL: Record<string, string> = {
  Baseball: "rookie cards, Topps and Bowman Chrome prospects, numbered parallels, autographs and vintage stars",
  Football: "rookie cards, Panini Prizm, Select, Mosaic and Optic parallels, autographs and patch cards",
  Basketball: "rookie cards, Prizm and Select parallels, numbered cards, autographs and vintage legends",
  Hockey: "Young Guns and other rookie cards, Upper Deck parallels, autographs and patch cards",
  Soccer: "Topps Chrome and Panini Prizm rookies, numbered parallels and autographs from clubs worldwide",
  "Auto Racing": "NASCAR and Formula 1 cards, parallels, autographs and relics",
  Wrestling: "WWE and AEW cards, Prizm and Select parallels, autographs and relics",
  MMA: "UFC Prizm and Select cards, rookies, parallels and autographs",
  Golf: "Upper Deck and Panini golf cards, rookies, autographs and parallels",
  Boxing: "boxing rookies, parallels, autographs and vintage fighters",
  Tennis: "tennis rookies, parallels and autographs",
  Pokemon: "single cards from every era: holos, full arts, illustration rares, promos and graded slabs",
  Magic: "singles for Commander, Modern and collectors: rares, mythics, foils, showcase frames and graded cards",
  "Yu-Gi-Oh!": "singles from every era: secret, ultimate and starlight rares, staples and graded cards",
  "One Piece": "leaders, alt arts, manga rares and other singles",
  Lorcana: "enchanted, legendary and foil singles",
  "Dragon Ball": "Dragon Ball Super and Z singles, special rares and alt arts",
  Digimon: "singles, alt arts and secret rares",
  Superheroes: "Marvel and DC trading cards: base sets, inserts, sketch cards and autographs",
  Entertainment: "movie and TV trading cards: inserts, autographs, costume relics and sketch cards",
  "Sci-Fi": "Star Trek, Star Wars and other sci-fi trading cards: inserts, autographs and relics",
  Anime: "anime trading cards and collectible inserts",
  Music: "music trading cards, autographs and inserts",
  Historical: "historical, political and educational trading cards",
};

const KIND: Record<CategoryKind, { heading: string; detail: string }> = {
  sports: { heading: "Sports cards for sale", detail: "baseball, football, basketball, hockey, soccer and more: rookies, autographs, numbered parallels and graded slabs" },
  tcg: { heading: "Trading card game singles for sale", detail: "Pokémon, Magic: The Gathering, Yu-Gi-Oh!, One Piece, Lorcana and more, raw or graded" },
  nonsport: { heading: "Non-sport cards for sale", detail: "superheroes, movies and TV, sci-fi, fantasy, anime, music and more: inserts, sketch cards and autographs" },
};

/** "Pokémon TCG" -> heading "Pokémon TCG cards for sale". */
function subjectHeading(kind: CategoryKind, c: Choice): string {
  const name = kind === "nonsport" ? c.short : c.label;
  return /cards?$/i.test(name) ? `${name} for sale` : `${name} cards for sale`;
}

function introText(kind: CategoryKind, c: Choice | null): string {
  const tail = "Prices include shipping, sellers ship within 3 business days, and every order is paid securely through Stripe.";
  if (!c) return `Shop ${KIND[kind].detail}, listed by collectors and card shops across the US. ${tail}`;
  const name = kind === "nonsport" ? c.short : c.label;
  const what = DETAIL[c.value] ?? (kind === "tcg" ? "singles, foils, promos and graded cards" : "singles, inserts, autographs and graded cards");
  return `Buy ${name} cards from collectors and card shops across the US: ${what}. ${tail}`;
}

async function resolve(kind: CategoryKind, slug?: string): Promise<{ choice: Choice | null; all: Choice[] }> {
  const all = choicesFor(await getCatalog(), kind);
  if (!slug) return { choice: null, all };
  const choice = all.find((c) => slugFor(c.value) === slug);
  if (!choice) notFound();
  return { choice, all };
}

const sansFilters = (sp: SP): SP => {
  const out: SP = { ...sp };
  delete out.category;
  delete out.game;
  delete out.cat;
  return out;
};

export async function landingMetadata(kind: CategoryKind, slug: string | undefined, sp: SP): Promise<Metadata> {
  const { choice } = await resolve(kind, slug);
  const title = choice ? subjectHeading(kind, choice) : KIND[kind].heading;
  const description = introText(kind, choice).slice(0, 300);
  const url = SITE_URL + landingHref(kind, choice?.value);
  // Sorting or filtering this page makes a variant that shouldn't compete with it.
  const extra = Object.keys(sansFilters(sp)).length > 0;
  return {
    title,
    description,
    alternates: { canonical: url },
    openGraph: { type: "website", url, title, description },
    ...(extra ? { robots: { index: false, follow: true } } : {}),
  };
}

export async function LandingPage({ kind, slug, sp }: { kind: CategoryKind; slug?: string; sp: SP }) {
  const { choice, all } = await resolve(kind, slug);
  const info = categoryInfo(kind)!;
  const raw: SP = { ...sansFilters(sp), category: kind, ...(choice ? { game: choice.value } : {}) };
  const heading = choice ? subjectHeading(kind, choice) : KIND[kind].heading;

  const crumbs = [
    { name: "Home", url: SITE_URL + "/" },
    { name: info.label, url: SITE_URL + landingHref(kind) },
    ...(choice ? [{ name: kind === "nonsport" ? choice.short : choice.label, url: SITE_URL + landingHref(kind, choice.value) }] : []),
  ];
  const ld = {
    "@context": "https://schema.org",
    "@type": "BreadcrumbList",
    itemListElement: crumbs.map((c, i) => ({ "@type": "ListItem", position: i + 1, name: c.name, item: c.url })),
  };

  const intro = (
    <div className="mt-1 max-w-[780px]">
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: ldJson(ld) }} />
      <nav aria-label="Breadcrumb" className="text-[12px] text-muted">
        <Link href="/" className="hover:underline">Home</Link>
        {" › "}
        {choice ? <Link href={landingHref(kind)} className="hover:underline">{info.label}</Link> : <span>{info.label}</span>}
        {choice && <>{" › "}<span>{kind === "nonsport" ? choice.short : choice.label}</span></>}
      </nav>
      <p className="mt-1 text-[14px] text-muted">{introText(kind, choice)}</p>
      {!choice && (
        <p className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-[13px]">
          {all.map((c) => (
            <Link key={c.value} href={landingHref(kind, c.value)} className="text-link hover:underline">{c.short}</Link>
          ))}
        </p>
      )}
    </div>
  );

  return <SearchView raw={raw} heading={heading} intro={intro} onLanding />;
}
