"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { send } from "@/lib/client";

/** "Save this search" on the results page, with an alert for new matches. */
export default function SaveSearchButton({ query, savedId }: { query: string; savedId?: string }) {
  const router = useRouter();
  const [state, setState] = useState<"" | "busy" | "saved" | "exists">("");
  const [error, setError] = useState("");

  // Opened from an alert: reset that search's "new" count.
  useEffect(() => {
    if (savedId) send("PATCH", `/v1/saved-searches/${savedId}`, { seen: true });
  }, [savedId]);

  async function save() {
    setState("busy");
    setError("");
    const r = await send("POST", "/v1/saved-searches", { query, notify: true });
    if (r.ok) return setState("saved");
    if (r.status === 401) {
      router.push(`/signin?next=${encodeURIComponent(window.location.pathname + window.location.search)}`);
      return;
    }
    if (r.status === 409) return setState("exists");
    setState("");
    setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
  }

  if (state === "saved" || state === "exists") {
    return (
      <span role="status" className="text-ship">
        {state === "saved" ? "Saved. We'll let you know about new matches." : "You've saved this search."}{" "}
        <Link href="/saved-searches" className="text-link hover:underline">Manage</Link>
      </span>
    );
  }
  return (
    <span>
      <button type="button" onClick={save} disabled={state === "busy"} className="font-semibold text-link hover:underline disabled:opacity-60">
        {state === "busy" ? "Saving…" : "Save this search"}
      </button>
      {error && <span role="alert" className="ml-2 text-slab">{error}</span>}
    </span>
  );
}
