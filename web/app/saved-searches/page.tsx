import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import SavedSearchRow, { type Saved } from "@/components/SavedSearchRow";

export const dynamic = "force-dynamic";
export const metadata = { title: "Saved searches", robots: { index: false } };

export default async function SavedSearchesPage() {
  await requireUser("/saved-searches");
  const r = await apiGet<{ searches: Saved[] }>("/v1/saved-searches");
  const list = r.data?.searches ?? [];
  return (
    <div className="mx-auto max-w-[820px] pt-6">
      <h1 className="text-[26px] font-bold">Saved searches</h1>
      <p className="mt-1 text-muted">
        We check your saved searches every few minutes and send a notification when new cards match, at most once every 6 hours per search.
      </p>
      {!r.data ? (
        <p role="alert" className="mt-6 rounded-lg bg-wash p-4 text-[14px]">We couldn&apos;t load your saved searches just now. Reload the page in a moment.</p>
      ) : list.length === 0 ? (
        <div className="py-14 text-center">
          <p className="text-[17px] font-semibold">No saved searches yet.</p>
          <p className="mt-1 text-muted">Search for a card, then choose <strong>Save this search</strong> above the results.</p>
          <Link href="/search?sort=newest" className="mt-5 inline-block rounded-full bg-link px-5 py-2 font-semibold text-white">Browse cards</Link>
        </div>
      ) : (
        <ul className="mt-6 divide-y divide-line border-y border-line">
          {list.map((s) => <SavedSearchRow key={s.id} s={s} />)}
        </ul>
      )}
    </div>
  );
}
