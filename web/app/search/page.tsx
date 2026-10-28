import Link from "next/link";
import type { Metadata } from "next";
import { facets as getFacets, search } from "@/lib/api";
import { ListingRow, ListingTile } from "@/components/CardBits";
import { ATTRIBUTES, CONDITIONS, money } from "@/lib/format";
import type { Facet, Facets } from "@/lib/types";
import SortSelect from "@/components/SortSelect";
import PriceFilter from "@/components/PriceFilter";

export const dynamic = "force-dynamic";

type SP = Record<string, string | string[] | undefined>;

// Query keys the API understands; anything else in the URL is ignored.
const API_KEYS = ["q", "category", "game", "set", "year", "grader", "min_grade", "condition", "attribute",
  "seller", "min_price", "max_price", "sort"];
const PAGE_SIZE = 48;

function pick(sp: SP): URLSearchParams {
  const out = new URLSearchParams();
  for (const k of [...API_KEYS, "view", "page"]) {
    const v = sp[k];
    const s = Array.isArray(v) ? v[0] : v;
    if (s) out.set(k, s);
  }
  return out;
}

function hrefWith(base: URLSearchParams, changes: Record<string, string | null>): string {
  const next = new URLSearchParams(base);
  for (const [k, v] of Object.entries(changes)) {
    if (v === null || v === "") next.delete(k);
    else next.set(k, v);
  }
  if (!("page" in changes)) next.delete("page");
  const s = next.toString();
  return "/search" + (s ? "?" + s : "");
}

export async function generateMetadata({ searchParams }: { searchParams: Promise<SP> }): Promise<Metadata> {
  const p = pick(await searchParams);
  const what = p.get("q") || p.get("game") || (p.get("category") === "tcg" ? "TCG singles" : p.get("category") === "sports" ? "Sports cards" : "All cards");
  return { title: what };
}

export default async function SearchPage({ searchParams }: { searchParams: Promise<SP> }) {
  const params = pick(await searchParams);
  const page = Math.max(1, Number(params.get("page")) || 1);
  const view = params.get("view") === "grid" ? "grid" : "list";

  const apiQs = new URLSearchParams();
  for (const k of API_KEYS) {
    const v = params.get(k);
    if (v) apiQs.set(k, v);
  }
  const facetQs = apiQs.toString();
  apiQs.set("limit", String(PAGE_SIZE));
  apiQs.set("offset", String((page - 1) * PAGE_SIZE));

  const [res, facets] = await Promise.all([search(apiQs.toString()), getFacets(facetQs)]);
  const pages = Math.max(1, Math.ceil(res.total / PAGE_SIZE));
  const first = res.total === 0 ? 0 : (page - 1) * PAGE_SIZE + 1;
  const last = Math.min(page * PAGE_SIZE, res.total);

  const q = params.get("q");
  const heading = q ? `Results for “${q}”` : params.get("game") || (params.get("category") === "tcg" ? "Trading card games" : params.get("category") === "sports" ? "Sports cards" : "All cards");

  return (
    <div className="grid gap-8 pt-4 md:grid-cols-[230px_1fr]">
      <aside aria-label="Filters" className="order-2 md:order-1">
        <Sidebar params={params} facets={facets} />
      </aside>

      <section className="order-1 min-w-0 md:order-2">
        <h1 className="text-[22px] font-bold">{heading}</h1>
        <ActiveFilters params={params} />

        <div className="mt-3 flex flex-wrap items-center gap-3 border-b border-line pb-3 text-[13px]">
          <span className="num">
            {res.total === 0 ? "No results" : `${first}–${last} of ${res.total.toLocaleString()} results`}
          </span>
          <div className="ml-auto flex items-center gap-3">
            <SortSelect value={params.get("sort") || "newest"} hasQuery={!!q} />
            <div className="flex overflow-hidden rounded border border-line" role="group" aria-label="View">
              <Link
                href={hrefWith(params, { view: null })}
                aria-current={view === "list" ? "true" : undefined}
                className={`px-3 py-1 ${view === "list" ? "bg-ink text-white" : "hover:bg-wash"}`}
              >
                List
              </Link>
              <Link
                href={hrefWith(params, { view: "grid" })}
                aria-current={view === "grid" ? "true" : undefined}
                className={`border-l border-line px-3 py-1 ${view === "grid" ? "bg-ink text-white" : "hover:bg-wash"}`}
              >
                Grid
              </Link>
            </div>
          </div>
        </div>

        {res.total === 0 ? (
          <div className="py-12 text-center">
            <p className="text-[17px] font-semibold">No cards match these filters.</p>
            <p className="mt-1 text-muted">Remove a filter or try a shorter search.</p>
            <Link href="/search" className="mt-4 inline-block text-link underline">Clear all filters</Link>
          </div>
        ) : view === "grid" ? (
          <div className="mt-4 grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 lg:grid-cols-4">
            {res.listings.map((l) => <ListingTile key={l.id} l={l} />)}
          </div>
        ) : (
          <ul>{res.listings.map((l) => <ListingRow key={l.id} l={l} />)}</ul>
        )}

        {pages > 1 && (
          <nav aria-label="Pages" className="mt-8 flex items-center justify-center gap-1 text-[14px]">
            {page > 1 && <PageLink params={params} n={page - 1} label="Previous" />}
            {pageNumbers(page, pages).map((n, i) =>
              n === 0 ? (
                <span key={`gap${i}`} className="px-2 text-muted">…</span>
              ) : (
                <PageLink key={n} params={params} n={n} current={n === page} />
              ),
            )}
            {page < pages && <PageLink params={params} n={page + 1} label="Next" />}
          </nav>
        )}
      </section>
    </div>
  );
}

function pageNumbers(page: number, pages: number): number[] {
  const set = new Set([1, pages, page - 1, page, page + 1].filter((n) => n >= 1 && n <= pages));
  const sorted = [...set].sort((a, b) => a - b);
  const out: number[] = [];
  sorted.forEach((n, i) => {
    if (i > 0 && n - sorted[i - 1] > 1) out.push(0);
    out.push(n);
  });
  return out;
}

function PageLink({ params, n, label, current }: { params: URLSearchParams; n: number; label?: string; current?: boolean }) {
  return (
    <Link
      href={hrefWith(params, { page: n === 1 ? null : String(n) })}
      aria-current={current ? "page" : undefined}
      className={`num rounded px-3 py-1.5 ${current ? "bg-ink text-white" : "text-link hover:bg-wash"}`}
    >
      {label ?? n}
    </Link>
  );
}

const FILTER_NAMES: Record<string, string> = {
  category: "Category", game: "Game or sport", grader: "Grading", min_grade: "Grade",
  condition: "Condition", attribute: "Feature", set: "Set", year: "Year", seller: "Seller",
};

function ActiveFilters({ params }: { params: URLSearchParams }) {
  const chips: { label: string; href: string }[] = [];
  for (const k of Object.keys(FILTER_NAMES)) {
    const v = params.get(k);
    if (!v) continue;
    let shown = v;
    if (k === "category") shown = v === "tcg" ? "Trading card games" : "Sports cards";
    if (k === "grader") shown = v === "RAW" ? "Raw (ungraded)" : v;
    if (k === "min_grade") shown = Number(v) <= 1 ? "Any graded" : `${v} and up`;
    if (k === "condition") shown = CONDITIONS[v] ?? v;
    if (k === "attribute") shown = ATTRIBUTES[v] ?? v;
    chips.push({ label: shown, href: hrefWith(params, { [k]: null }) });
  }
  const min = params.get("min_price"), max = params.get("max_price");
  if (min || max) {
    const label = min && max ? `$${min} to $${max}` : min ? `$${min} and up` : `Under $${max}`;
    chips.push({ label, href: hrefWith(params, { min_price: null, max_price: null }) });
  }
  if (chips.length === 0) return null;
  return (
    <ul className="mt-2 flex flex-wrap gap-2" aria-label="Active filters">
      {chips.map((c) => (
        <li key={c.label}>
          <Link
            href={c.href}
            className="inline-flex items-center gap-1 rounded-full border border-line bg-wash px-3 py-1 text-[13px] hover:border-ink"
            aria-label={`Remove filter ${c.label}`}
          >
            {c.label} <span aria-hidden="true">×</span>
          </Link>
        </li>
      ))}
      <li>
        <Link href={params.get("q") ? `/search?q=${encodeURIComponent(params.get("q")!)}` : "/search"} className="px-1 text-[13px] text-link underline">
          Clear all
        </Link>
      </li>
    </ul>
  );
}

const GAME_NAMES: Record<string, string> = { Pokemon: "Pokémon" };

function Sidebar({ params, facets }: { params: URLSearchParams; facets: Facets | null }) {
  return (
    <div className="space-y-6 text-[14px]">
      <FacetGroup
        title="Category"
        param="category"
        params={params}
        items={facets?.category}
        label={(v) => (v === "tcg" ? "Trading card games" : "Sports cards")}
      />
      <FacetGroup title="Game or sport" param="game" params={params} items={facets?.game} label={(v) => GAME_NAMES[v] ?? v} />
      <FacetGroup
        title="Grading"
        param="grader"
        params={params}
        items={facets?.grader}
        label={(v) => (v === "RAW" ? "Raw (ungraded)" : v)}
      />
      <div>
        <h2 className="mb-2 font-bold">Grade</h2>
        <ul className="space-y-1">
          {[["10", "10 only"], ["9", "9 and up"], ["8", "8 and up"], ["1", "Any graded"]].map(([v, l]) => (
            <li key={v}>
              <Option href={hrefWith(params, { min_grade: params.get("min_grade") === v ? null : v })} on={params.get("min_grade") === v}>
                {l}
              </Option>
            </li>
          ))}
        </ul>
      </div>
      <FacetGroup title="Condition (raw)" param="condition" params={params} items={facets?.condition} label={(v) => CONDITIONS[v] ?? v} />
      <FacetGroup title="Features" param="attribute" params={params} items={facets?.attribute} label={(v) => ATTRIBUTES[v] ?? v} />
      <div>
        <h2 className="mb-2 font-bold">Price</h2>
        <ul className="mb-3 space-y-1">
          {[[null, "5", "Under $5"], ["5", "20", "$5 to $20"], ["20", "100", "$20 to $100"], ["100", null, "Over $100"]].map(([lo, hi, l]) => {
            const on = (params.get("min_price") ?? null) === lo && (params.get("max_price") ?? null) === hi;
            return (
              <li key={l}>
                <Option href={hrefWith(params, on ? { min_price: null, max_price: null } : { min_price: lo, max_price: hi })} on={on}>
                  {l}
                </Option>
              </li>
            );
          })}
        </ul>
        <PriceFilter min={params.get("min_price") ?? ""} max={params.get("max_price") ?? ""} />
      </div>
      <p className="text-[12px] text-muted">
        Every card ships free, so the price shown is the price you pay. Cards are capped at {money(100000)}.
      </p>
    </div>
  );
}

function FacetGroup({
  title, param, params, items, label,
}: {
  title: string; param: string; params: URLSearchParams; items?: Facet[]; label: (v: string) => string;
}) {
  if (!items || items.length === 0) return null;
  const current = params.get(param);
  const shown = items.slice(0, 10);
  return (
    <div>
      <h2 className="mb-2 font-bold">{title}</h2>
      <ul className="space-y-1">
        {shown.map((f) => {
          const on = current?.toLowerCase() === f.value.toLowerCase();
          return (
            <li key={f.value}>
              <Option href={hrefWith(params, { [param]: on ? null : f.value })} on={on}>
                {label(f.value)} <span className="num text-muted">({f.count})</span>
              </Option>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

/** A filter choice that looks like a checkbox but is a plain link (works without JS). */
function Option({ href, on, children }: { href: string; on: boolean; children: React.ReactNode }) {
  return (
    <Link href={href} className="group flex items-center gap-2" aria-current={on ? "true" : undefined}>
      <span
        aria-hidden="true"
        className={`flex h-4 w-4 shrink-0 items-center justify-center rounded-[3px] border ${on ? "border-ink bg-ink text-white" : "border-muted group-hover:border-ink"}`}
      >
        {on && (
          <svg width="10" height="10" viewBox="0 0 10 10"><path d="M1.5 5.2 4 7.5 8.5 2.5" stroke="currentColor" strokeWidth="1.8" fill="none" /></svg>
        )}
      </span>
      <span className="group-hover:underline">{children}</span>
    </Link>
  );
}
