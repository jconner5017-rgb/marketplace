"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { HeartIcon } from "./Icons";
import { toggleWatch, useWatched } from "@/lib/watch";

/** Watchlist heart laid over a card photo. */
export default function HeartButton({ id, title, className = "" }: { id: string; title: string; className?: string }) {
  const w = useWatched();
  const router = useRouter();
  const [note, setNote] = useState("");
  const on = w.ids.has(id);

  async function click(e: React.MouseEvent) {
    e.preventDefault();
    e.stopPropagation();
    if (!w.signedIn) {
      const here = window.location.pathname + window.location.search;
      router.push(`/signin?next=${encodeURIComponent(here)}`);
      return;
    }
    const r = await toggleWatch(id);
    if (!r.ok) {
      setNote(r.message ?? "Couldn't update your watchlist");
      setTimeout(() => setNote(""), 3000);
    } else router.refresh();
  }

  return (
    <span className={className}>
      <button
        type="button"
        onClick={click}
        aria-pressed={on}
        aria-label={on ? `Remove ${title} from your watchlist` : `Add ${title} to your watchlist`}
        title={on ? "Watching" : "Add to Watchlist"}
        className={`flex h-9 w-9 items-center justify-center rounded-full bg-white/95 shadow-sm ring-1 ring-black/10 transition hover:scale-105 ${on ? "text-slab" : "text-ink"}`}
      >
        <HeartIcon size={18} filled={on} />
      </button>
      {note && <span role="status" className="absolute right-0 top-10 z-10 w-48 rounded bg-ink px-2 py-1 text-[12px] text-white">{note}</span>}
    </span>
  );
}
