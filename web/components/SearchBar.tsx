"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
import Link from "next/link";

const SCOPES = [
  { value: "", label: "All categories" },
  { value: "sports", label: "Sports cards" },
  { value: "tcg", label: "Trading card games" },
];

export default function SearchBar() {
  const router = useRouter();
  const params = useSearchParams();
  const [q, setQ] = useState(params.get("q") ?? "");
  const [scope, setScope] = useState(params.get("category") ?? "");

  function submit(e: React.FormEvent) {
    e.preventDefault();
    const next = new URLSearchParams();
    if (q.trim()) next.set("q", q.trim());
    if (scope) next.set("category", scope);
    if (q.trim()) next.set("sort", "relevance");
    router.push("/search?" + next.toString());
  }

  return (
    <div className="flex min-w-[280px] flex-1 items-center gap-3">
    <form role="search" onSubmit={submit} className="flex flex-1 items-stretch">
      <div className="flex flex-1 items-stretch rounded-l-full border-2 border-ink">
        <label htmlFor="q" className="sr-only">Search for cards</label>
        <input
          id="q"
          type="search"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Search for a player, card or set"
          maxLength={200}
          className="min-w-0 flex-1 rounded-l-full bg-transparent px-4 py-2 outline-none"
        />
        <label htmlFor="scope" className="sr-only">Category</label>
        <select
          id="scope"
          value={scope}
          onChange={(e) => setScope(e.target.value)}
          className="hidden border-l border-line bg-transparent px-3 text-[13px] sm:block"
        >
          {SCOPES.map((s) => (
            <option key={s.value} value={s.value}>{s.label}</option>
          ))}
        </select>
      </div>
      <button type="submit" className="rounded-r-full bg-link px-6 font-semibold text-white hover:bg-link-dark">
        Search
      </button>
    </form>
    <Link href="/search/advanced" className="hidden text-[13px] text-muted hover:text-ink hover:underline sm:inline">Advanced</Link>
    </div>
  );
}
