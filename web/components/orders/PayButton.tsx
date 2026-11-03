"use client";

import { useState } from "react";
import { send } from "@/lib/client";

/** Starts Stripe Checkout for one seller's cart cards, or for held cards. */
export default function PayButton({ seller, holdIds, label, test = false, className = "", envelopeChoice = false }: {
  seller?: string;
  /** The seller trusts this buyer: offer a stamped envelope (one raw card up to $100). */
  envelopeChoice?: boolean;
  holdIds?: string[];
  label: string;
  /** Admin test mode: the order is placed with no payment. */
  test?: boolean;
  className?: string;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [envelope, setEnvelope] = useState(false);

  async function go() {
    setBusy(true);
    setError("");
    const r = await send<{ order_id: string; url: string }>("POST", "/v1/checkout", holdIds ? { hold_ids: holdIds, envelope } : { seller, envelope });
    if (!r.ok) {
      setBusy(false);
      return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    }
    window.location.assign(r.data.url);
  }

  return (
    <div className={className}>
      {envelopeChoice && (
        <fieldset className="mb-3 rounded-lg border border-line p-3 text-left text-[13px]">
          <legend className="px-1 font-semibold">How should it ship?</legend>
          <label className="flex items-start gap-2">
            <input type="radio" name="ship" checked={!envelope} onChange={() => setEnvelope(false)} className="mt-1" />
            <span>Tracked package (recommended)</span>
          </label>
          <label className="mt-1.5 flex items-start gap-2">
            <input type="radio" name="ship" checked={envelope} onChange={() => setEnvelope(true)} className="mt-1" />
            <span>
              Stamped envelope, approved by the seller.{" "}
              <span className="text-muted">You&apos;re on this seller&apos;s trusted buyer list. Envelopes get USPS sorting scans but no delivery
              confirmation, and a missing envelope isn&apos;t covered by Slabs &amp; Singles.</span>
            </span>
          </label>
        </fieldset>
      )}
      <button type="button" onClick={go} disabled={busy}
        className="w-full whitespace-nowrap rounded-full bg-link px-5 py-2.5 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
        {busy ? (test ? "Placing test order…" : "Opening secure checkout…") : label}
      </button>
      {test && <p className="mt-1 text-center text-[12px] text-muted">Test order · no payment taken</p>}
      {error && <p role="alert" className="mt-2 text-[13px] text-slab">{error}</p>}
    </div>
  );
}
