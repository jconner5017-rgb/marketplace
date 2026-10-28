import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import type { WatchItem } from "@/lib/types";
import { GradeBadge, Photo } from "@/components/CardBits";
import { money } from "@/lib/format";
import WatchRowActions from "@/components/WatchRowActions";

export const dynamic = "force-dynamic";
export const metadata = { title: "Watchlist" };

export default async function WatchlistPage() {
  await requireUser("/watchlist");
  const r = await apiGet<{ items: WatchItem[] }>("/v1/watchlist");
  const items = r.data?.items ?? [];
  return (
    <div className="pt-6">
      <h1 className="text-[26px] font-bold">Watchlist</h1>
      <p className="text-muted">Cards you&apos;re keeping an eye on. {items.length > 0 && `${items.length} watched.`}</p>
      {items.length === 0 ? (
        <div className="py-14 text-center">
          <p className="text-[17px] font-semibold">You&apos;re not watching any cards yet.</p>
          <p className="mt-1 text-muted">Tap “Add to Watchlist” on any listing to save it here.</p>
          <Link href="/search?sort=newest" className="mt-5 inline-block rounded-full bg-link px-5 py-2 font-semibold text-white">Browse cards</Link>
        </div>
      ) : (
        <ul className="mt-4">
          {items.map(({ listing: l, available, in_cart }) => (
            <li key={l.id} className={`flex gap-4 border-b border-line py-4 ${available ? "" : "opacity-60"}`}>
              <Link href={`/item/${l.id}`} className="shrink-0"><Photo l={l} className="h-[140px] w-[100px] rounded-md" /></Link>
              <div className="min-w-0 flex-1">
                <Link href={`/item/${l.id}`} className="title-tight text-[17px] hover:underline">{l.title}</Link>
                <p className="mt-1 text-[13px] text-muted">{[l.year, l.set_name].filter(Boolean).join(" ")} · {l.game_or_sport} · {l.seller_username}</p>
                <div className="mt-2"><GradeBadge l={l} /></div>
                <p className="num mt-2 text-[20px] font-bold">{money(l.price_cents)}</p>
                {available ? <p className="text-[13px] text-ship">Free shipping</p> : <p className="text-[13px] font-semibold text-slab">No longer available</p>}
              </div>
              <WatchRowActions id={l.id} available={available} inCart={in_cart} />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
