"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

function useAction() {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function run(path: string) {
    setBusy(true);
    setError("");
    const r = await send("POST", path, {});
    setBusy(false);
    if (!r.ok) return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    router.refresh();
  }
  return { busy, error, run };
}

/** Seller: trust the buyer and offer a stamped envelope instead of a label. */
export function OfferEnvelope({ orderId, buyer }: { orderId: string; buyer: string }) {
  const { busy, error, run } = useAction();
  return (
    <div className="mt-4 rounded-lg bg-wash p-3 text-[13px]">
      <p className="font-semibold">Know {buyer}? Save on postage</p>
      <p className="mt-1 text-muted">
        Add {buyer} as a trusted buyer and offer to send this card in a stamped envelope instead. The order switches only if
        they accept; a missing envelope then isn&apos;t covered by Slabs &amp; Singles. Your ship-by date stays the same, so ship by
        label if they don&apos;t answer in time.
      </p>
      <button type="button" disabled={busy} onClick={() => run(`/v1/orders/${orderId}/envelope-offer`)}
        className="mt-2 rounded-full border border-link px-4 py-1.5 font-semibold text-link hover:bg-white disabled:opacity-60">
        {busy ? "Sending…" : "Trust this buyer and offer an envelope"}
      </button>
      {error && <p role="alert" className="mt-1 text-slab">{error}</p>}
    </div>
  );
}

/** Buyer: accept or decline the seller's envelope offer. */
export function AnswerEnvelope({ orderId, seller }: { orderId: string; seller: string }) {
  const { busy, error, run } = useAction();
  return (
    <div className="mt-4 max-w-[560px] rounded-lg border border-ink p-4 text-[14px]">
      <h2 className="font-semibold">{seller} offered to ship in a stamped envelope</h2>
      <p className="mt-1 text-muted">
        Instead of a tracked package. Envelopes get USPS sorting scans but no delivery confirmation, and if the envelope goes
        missing it isn&apos;t covered by Slabs &amp; Singles. You pay the same either way.
      </p>
      <div className="mt-3 flex flex-wrap gap-2">
        <button type="button" disabled={busy} onClick={() => run(`/v1/orders/${orderId}/envelope-offer/accept`)}
          className="rounded-full bg-link px-4 py-1.5 font-semibold text-white hover:bg-link-dark disabled:opacity-60">Accept the envelope</button>
        <button type="button" disabled={busy} onClick={() => run(`/v1/orders/${orderId}/envelope-offer/decline`)}
          className="rounded-full border border-link px-4 py-1.5 font-semibold text-link hover:bg-wash disabled:opacity-60">Keep the tracked package</button>
      </div>
      {error && <p role="alert" className="mt-1 text-[13px] text-slab">{error}</p>}
    </div>
  );
}
