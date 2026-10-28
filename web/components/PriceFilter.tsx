"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";

export default function PriceFilter({ min, max }: { min: string; max: string }) {
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const [lo, setLo] = useState(min);
  const [hi, setHi] = useState(max);

  function apply(e: React.FormEvent) {
    e.preventDefault();
    const next = new URLSearchParams(params.toString());
    const clean = (v: string) => v.replace(/[^0-9.]/g, "");
    clean(lo) ? next.set("min_price", clean(lo)) : next.delete("min_price");
    clean(hi) ? next.set("max_price", clean(hi)) : next.delete("max_price");
    next.delete("page");
    router.push(`${pathname}?${next.toString()}`);
  }

  return (
    <form onSubmit={apply} className="flex items-center gap-1.5">
      <label className="sr-only" htmlFor="pmin">Minimum price</label>
      <input id="pmin" inputMode="decimal" placeholder="$ Min" value={lo} onChange={(e) => setLo(e.target.value)}
        className="w-[72px] rounded border border-line px-2 py-1 text-[13px]" />
      <span className="text-muted">to</span>
      <label className="sr-only" htmlFor="pmax">Maximum price</label>
      <input id="pmax" inputMode="decimal" placeholder="$ Max" value={hi} onChange={(e) => setHi(e.target.value)}
        className="w-[72px] rounded border border-line px-2 py-1 text-[13px]" />
      <button type="submit" aria-label="Apply price range" className="rounded-full border border-ink px-2.5 py-1 text-[13px] hover:bg-wash">
        Go
      </button>
    </form>
  );
}
