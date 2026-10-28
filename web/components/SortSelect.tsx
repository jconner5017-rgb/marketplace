"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";

export default function SortSelect({ value, hasQuery }: { value: string; hasQuery: boolean }) {
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();

  function change(v: string) {
    const next = new URLSearchParams(params.toString());
    if (v === "newest") next.delete("sort");
    else next.set("sort", v);
    next.delete("page");
    router.push(`${pathname}?${next.toString()}`);
  }

  return (
    <label className="flex items-center gap-2">
      <span className="text-muted">Sort</span>
      <select value={value} onChange={(e) => change(e.target.value)} className="rounded border border-line bg-paper px-2 py-1">
        {hasQuery && <option value="relevance">Best match</option>}
        <option value="newest">Newly listed</option>
        <option value="price_asc">Price: lowest first</option>
        <option value="price_desc">Price: highest first</option>
        <option value="ending">Ending soonest</option>
      </select>
    </label>
  );
}
