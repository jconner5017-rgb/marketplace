import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import type { Store } from "@/lib/types";

export const dynamic = "force-dynamic";
export const metadata = { title: "Followed stores", robots: { index: false } };

export default async function FollowingPage() {
  await requireUser("/following");
  const r = await apiGet<{ stores: Store[] }>("/v1/me/stores");
  const list = r.data?.stores ?? [];
  return (
    <div className="max-w-[860px] pb-10 pt-6">
      <h1 className="text-[26px] font-bold">Followed stores</h1>
      <p className="mt-1 text-muted">You&apos;ll get a notification when these stores list new cards or send an update.</p>
      {list.length === 0 ? (
        <p className="py-12 text-center text-muted">You aren&apos;t following any stores yet. Look for Follow store on a store&apos;s page.</p>
      ) : (
        <ul className="mt-5 grid gap-3 sm:grid-cols-2">
          {list.map((s) => (
            <li key={s.slug}>
              <Link href={`/store/${s.slug}`} className="flex items-center gap-3 rounded-xl border border-line p-3 hover:bg-wash">
                <span className="h-14 w-14 shrink-0 overflow-hidden rounded-lg bg-wash">
                  {s.logo_url
                    ? /* eslint-disable-next-line @next/next/no-img-element */ <img src={s.logo_url} alt="" className="h-full w-full object-cover" />
                    : <span className="flex h-full items-center justify-center text-[22px] font-bold text-muted">{s.name.slice(0, 1).toUpperCase()}</span>}
                </span>
                <span className="min-w-0">
                  <span className="block truncate font-semibold">{s.name}</span>
                  <span className="block truncate text-[13px] text-muted">{s.tagline || `${s.listings} cards for sale`}</span>
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
