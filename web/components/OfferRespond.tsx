"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";
import { money, toCents } from "@/lib/format";
import type { Offer } from "@/lib/types";

/** Accept, decline or counter an offer that's waiting on you. */
export default function OfferRespond({ o }: { o: Offer }) {
  const router = useRouter();
  const [mode, setMode] = useState<"" | "counter" | "decline">("");
  const [amount, setAmount] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const sellerSide = o.from_role === "buyer"; // the viewer is the seller

  async function act(action: "accept" | "decline" | "counter") {
    setError("");
    let cents = 0;
    if (action === "counter") {
      const c = toCents(amount);
      if (c == null) return setError("Enter a dollar amount, like 12.50.");
      cents = c;
    }
    setBusy(true);
    const r = await send<{ offer: Offer }>("POST", `/v1/offers/${o.id}/respond`, { action, amount_cents: cents });
    setBusy(false);
    if (!r.ok) return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    router.refresh();
  }

  const btn = "rounded-full px-4 py-1.5 text-[14px] font-semibold disabled:opacity-60";
  return (
    <div className="mt-3">
      {mode === "counter" ? (
        <form onSubmit={(e) => { e.preventDefault(); act("counter"); }} className="flex flex-wrap items-center gap-2">
          <label className="flex items-center rounded-md border border-line px-2 focus-within:border-ink">
            <span className="sr-only">Counteroffer amount</span>
            <span className="text-muted">$</span>
            <input inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} autoFocus
              placeholder={sellerSide ? `${(o.amount_cents / 100 + 0.01).toFixed(2)}–${(o.price_cents / 100).toFixed(2)}` : `under ${(o.amount_cents / 100).toFixed(2)}`}
              className="num w-36 bg-transparent px-1 py-1.5 outline-none" />
          </label>
          <button type="submit" disabled={busy} className={`${btn} bg-link text-white hover:bg-link-dark`}>Send counter</button>
          <button type="button" onClick={() => setMode("")} className={`${btn} text-link hover:bg-wash`}>Cancel</button>
        </form>
      ) : mode === "decline" ? (
        <div className="flex flex-wrap items-center gap-2 text-[14px]">
          <span>Decline {money(o.amount_cents)}?</span>
          <button type="button" onClick={() => act("decline")} disabled={busy} className={`${btn} border border-slab text-slab hover:bg-slab/5`}>Decline</button>
          <button type="button" onClick={() => setMode("")} className={`${btn} text-link hover:bg-wash`}>Keep it</button>
        </div>
      ) : (
        <div className="flex flex-wrap gap-2">
          <button type="button" onClick={() => act("accept")} disabled={busy} className={`${btn} bg-link text-white hover:bg-link-dark`}>
            Accept {money(o.amount_cents)}
          </button>
          {(sellerSide || o.offers_left > 0) && (
            <button type="button" onClick={() => setMode("counter")} disabled={busy} className={`${btn} border border-link text-link hover:bg-wash`}>Counter</button>
          )}
          <button type="button" onClick={() => setMode("decline")} disabled={busy} className={`${btn} border border-line hover:border-ink`}>Decline</button>
        </div>
      )}
      {error && <p role="alert" className="mt-2 text-[14px] text-slab">{error}</p>}
    </div>
  );
}
