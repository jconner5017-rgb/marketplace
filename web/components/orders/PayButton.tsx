"use client";

import { useState } from "react";
import { send } from "@/lib/client";

/** Starts Stripe Checkout for one seller's cart cards, or for held cards. */
export default function PayButton({ seller, holdIds, label, className = "" }: {
  seller?: string;
  holdIds?: string[];
  label: string;
  className?: string;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function go() {
    setBusy(true);
    setError("");
    const r = await send<{ order_id: string; url: string }>("POST", "/v1/checkout", holdIds ? { hold_ids: holdIds } : { seller });
    if (!r.ok) {
      setBusy(false);
      return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    }
    window.location.assign(r.data.url);
  }

  return (
    <div className={className}>
      <button type="button" onClick={go} disabled={busy}
        className="w-full whitespace-nowrap rounded-full bg-link px-5 py-2.5 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
        {busy ? "Opening secure checkout…" : label}
      </button>
      {error && <p role="alert" className="mt-2 text-[13px] text-slab">{error}</p>}
    </div>
  );
}
