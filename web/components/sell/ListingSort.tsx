"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { LISTING_SORTS } from "@/lib/listingSorts";


/** Sort dropdown for the seller's listings; keeps the view and search, starts at page 1. */
export default function ListingSort({ value }: { value: string }) {
  const router = useRouter();
  const path = usePathname();
  const params = useSearchParams();
  return (
    <label className="flex items-center gap-2 text-[14px]">
      <span className="text-muted">Sort</span>
      <select
        value={value || "newest"}
        onChange={(e) => {
          const next = new URLSearchParams(params.toString());
          if (e.target.value === "newest") next.delete("sort");
          else next.set("sort", e.target.value);
          next.delete("page");
          const s = next.toString();
          router.push(path + (s ? "?" + s : ""));
        }}
        className="rounded-lg border border-line bg-paper px-3 py-2"
      >
        {LISTING_SORTS.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
      </select>
    </label>
  );
}
