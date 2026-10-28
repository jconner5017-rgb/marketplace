import Link from "next/link";
import { CONDITIONS } from "@/lib/format";
import { getCatalog } from "@/lib/catalog";

export const metadata = { title: "Advanced search" };

type SP = Record<string, string | string[] | undefined>;
const GRADERS = ["PSA", "BGS", "SGC", "CGC", "TAG", "OTHER"];

// A plain GET form: it works without JavaScript and every result page is a shareable link.
export default async function AdvancedSearch({ searchParams }: { searchParams: Promise<SP> }) {
  const catalog = await getCatalog();
  const sp = await searchParams;
  const one = (k: string) => {
    const v = sp[k];
    return (Array.isArray(v) ? v[0] : v) ?? "";
  };
  const all = (k: string) => {
    const v = sp[k];
    return (Array.isArray(v) ? v : v ? [v] : []).flatMap((x) => x.split(","));
  };
  const cat = one("game") ? `${one("category") || (catalog.sports.some((s) => s.value === one("game")) ? "sports" : "tcg")}:${one("game")}` : one("category");

  const label = "block text-[14px] font-semibold";
  const input = "mt-1 w-full rounded-md border border-line bg-paper px-3 py-2 text-[15px] focus:border-ink";
  const box = "border-t border-line pt-6";

  return (
    <div className="mx-auto max-w-[760px] pt-6">
      <div className="flex items-baseline justify-between">
        <h1 className="text-[28px] font-bold">Advanced search</h1>
        <Link href="/help/buying" className="text-[14px] text-link hover:underline">Search tips</Link>
      </div>

      <form action="/search" method="get" className="mt-6 space-y-8">
        {/* Find items */}
        <section className="space-y-4">
          <h2 className="text-[19px] font-bold">Find cards</h2>
          <div>
            <label htmlFor="adv-q" className={label}>Keywords or item number</label>
            <input id="adv-q" name="q" defaultValue={one("q")} maxLength={200} placeholder="Player, card name, set, or item number" className={input} />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <label htmlFor="mode" className={label}>Keyword options</label>
              <select id="mode" name="mode" defaultValue={one("mode") || "all"} className={input}>
                <option value="all">All words, any order</option>
                <option value="any">Any of the words</option>
                <option value="exact">Exact phrase, exact order</option>
              </select>
            </div>
            <div>
              <label htmlFor="exclude" className={label}>Exclude words</label>
              <input id="exclude" name="exclude" defaultValue={one("exclude")} maxLength={200} placeholder="reprint proxy custom" className={input} />
            </div>
          </div>
          <div>
            <label htmlFor="cat" className={label}>In this category</label>
            <select id="cat" name="cat" defaultValue={cat} className={input}>
              <option value="">All categories</option>
              <option value="sports">All sports cards</option>
              <optgroup label="Sports cards">
                {catalog.sports.map((s) => <option key={s.value} value={`sports:${s.value}`}>{s.label}</option>)}
              </optgroup>
              <option value="tcg">All trading card games</option>
              <optgroup label="Trading card games">
                {catalog.games.map((g) => <option key={g.value} value={`tcg:${g.value}`}>{g.label}</option>)}
              </optgroup>
            </select>
          </div>
          <fieldset>
            <legend className={label}>Search in</legend>
            <div className="mt-2 flex flex-wrap gap-x-6 gap-y-2 text-[15px]">
              <label className="flex items-center gap-2">
                <input type="radio" name="in" value="" defaultChecked={one("in") !== "title"} /> Title and description
              </label>
              <label className="flex items-center gap-2">
                <input type="radio" name="in" value="title" defaultChecked={one("in") === "title"} /> Titles only
              </label>
            </div>
          </fieldset>
        </section>

        {/* Card details */}
        <section className={`${box} space-y-4`}>
          <h2 className="text-[19px] font-bold">Card details</h2>
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <label htmlFor="set" className={label}>Set</label>
              <input id="set" name="set" defaultValue={one("set")} placeholder="Base Set, Topps Chrome…" className={input} />
            </div>
            <div>
              <label htmlFor="number" className={label}>Card number</label>
              <input id="number" name="number" defaultValue={one("number")} placeholder="4/102" className={input} />
            </div>
          </div>
          <fieldset>
            <legend className={label}>Year</legend>
            <div className="mt-1 flex items-center gap-2">
              <label htmlFor="year_min" className="sr-only">From year</label>
              <input id="year_min" name="year_min" inputMode="numeric" defaultValue={one("year_min")} placeholder="From" className={`${input} mt-0 w-28`} />
              <span className="text-muted">to</span>
              <label htmlFor="year_max" className="sr-only">To year</label>
              <input id="year_max" name="year_max" inputMode="numeric" defaultValue={one("year_max")} placeholder="To" className={`${input} mt-0 w-28`} />
            </div>
          </fieldset>
        </section>

        {/* Grading and condition */}
        <section className={`${box} space-y-5`}>
          <h2 className="text-[19px] font-bold">Grading and condition</h2>
          <fieldset>
            <legend className={label}>Grading</legend>
            <div className="mt-2 grid grid-cols-2 gap-2 text-[15px] sm:grid-cols-4">
              <label className="flex items-center gap-2"><input type="checkbox" name="grader" value="RAW" defaultChecked={all("grader").includes("RAW")} /> Raw (ungraded)</label>
              {GRADERS.map((g) => (
                <label key={g} className="flex items-center gap-2">
                  <input type="checkbox" name="grader" value={g} defaultChecked={all("grader").includes(g)} /> {g === "OTHER" ? "Other grader" : g}
                </label>
              ))}
            </div>
          </fieldset>
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <label htmlFor="min_grade" className={label}>Minimum grade</label>
              <select id="min_grade" name="min_grade" defaultValue={one("min_grade")} className={input}>
                <option value="">Any</option>
                <option value="1">Any graded card</option>
                {[10, 9.5, 9, 8.5, 8, 7, 6, 5].map((g) => <option key={g} value={g}>{g === 10 ? "10 only" : `${g} and up`}</option>)}
              </select>
            </div>
            <div>
              <label htmlFor="cert" className={label}>Cert number</label>
              <input id="cert" name="cert" defaultValue={one("cert")} placeholder="Grading company cert #" className={input} />
            </div>
          </div>
          <fieldset>
            <legend className={label}>Condition (raw cards)</legend>
            <div className="mt-2 grid grid-cols-2 gap-2 text-[15px] sm:grid-cols-3">
              {Object.entries(CONDITIONS).map(([k, v]) => (
                <label key={k} className="flex items-center gap-2">
                  <input type="checkbox" name="condition" value={k} defaultChecked={all("condition").includes(k)} /> {v}
                </label>
              ))}
            </div>
          </fieldset>
          <fieldset>
            <legend className={label}>Features <span className="font-normal text-muted">(cards must have all you pick)</span></legend>
            <div className="mt-2 grid grid-cols-2 gap-2 text-[15px] sm:grid-cols-3">
              {Object.entries(catalog.features).map(([k, v]) => (
                <label key={k} className="flex items-center gap-2">
                  <input type="checkbox" name="attribute" value={k} defaultChecked={all("attribute").includes(k)} /> {v}
                </label>
              ))}
            </div>
          </fieldset>
        </section>

        {/* Price and seller */}
        <section className={`${box} space-y-4`}>
          <h2 className="text-[19px] font-bold">Price and seller</h2>
          <fieldset>
            <legend className={label}>Price</legend>
            <div className="mt-1 flex items-center gap-2">
              <span className="text-muted">$</span>
              <label htmlFor="min_price" className="sr-only">Minimum price</label>
              <input id="min_price" name="min_price" inputMode="decimal" defaultValue={one("min_price")} placeholder="Min" className={`${input} mt-0 w-28`} />
              <span className="text-muted">to $</span>
              <label htmlFor="max_price" className="sr-only">Maximum price</label>
              <input id="max_price" name="max_price" inputMode="decimal" defaultValue={one("max_price")} placeholder="Max" className={`${input} mt-0 w-28`} />
            </div>
            <p className="mt-1 text-[13px] text-muted">Prices include shipping. Cards are capped at $1,000.</p>
          </fieldset>
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <label htmlFor="seller" className={label}>Seller</label>
              <input id="seller" name="seller" defaultValue={one("seller")} placeholder="Username" className={input} />
            </div>
          </div>
          <fieldset>
            <legend className={label}>Buying format</legend>
            <div className="mt-2 flex flex-wrap gap-x-5 gap-y-2 text-[15px]">
              {[["", "All listings"], ["auction", "Auction"], ["fixed", "Buy It Now"], ["offers", "Accepts offers"]].map(([v, l]) => (
                <label key={v || "all"} className="flex items-center gap-2">
                  <input type="radio" name="format" value={v} defaultChecked={(one("format") || "") === v} /> {l}
                </label>
              ))}
            </div>
          </fieldset>
          <label className="flex items-center gap-2 text-[15px]">
            <input type="checkbox" name="discount" value="1" defaultChecked={one("discount") === "1"} /> Seller offers a multi-card discount
          </label>
        </section>

        {/* Results */}
        <section className={`${box} space-y-4`}>
          <h2 className="text-[19px] font-bold">Results</h2>
          <div className="grid gap-4 sm:grid-cols-3">
            <div>
              <label htmlFor="sort" className={label}>Sort by</label>
              <select id="sort" name="sort" defaultValue={one("sort") || "relevance"} className={input}>
                <option value="relevance">Best match</option>
                <option value="newest">Newly listed</option>
                <option value="price_asc">Price: lowest first</option>
                <option value="price_desc">Price: highest first</option>
                <option value="ending">Ending soonest</option>
              </select>
            </div>
            <div>
              <label htmlFor="view" className={label}>View</label>
              <select id="view" name="view" defaultValue={one("view") || "list"} className={input}>
                <option value="list">List</option>
                <option value="grid">Grid</option>
              </select>
            </div>
            <div>
              <label htmlFor="per" className={label}>Results per page</label>
              <select id="per" name="per" defaultValue={one("per") || "48"} className={input}>
                <option value="24">24</option>
                <option value="48">48</option>
                <option value="96">96</option>
              </select>
            </div>
          </div>
        </section>

        <div className="flex items-center gap-4 border-t border-line pt-6">
          <button type="submit" className="rounded-full bg-link px-10 py-3 font-semibold text-white hover:bg-link-dark">Search</button>
          <Link href="/search/advanced" className="text-[14px] text-link hover:underline">Clear options</Link>
        </div>
      </form>
    </div>
  );
}
