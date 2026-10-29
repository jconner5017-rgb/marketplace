"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

/** Seller switch: have Stripe Tax add sales tax to their checkouts. */
export default function TaxToggle({ on }: { on: boolean }) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function flip() {
    setBusy(true);
    setError("");
    const r = await send("PATCH", "/v1/seller/tax", { collect_tax: !on });
    setBusy(false);
    if (!r.ok) return setError(r.message);
    router.refresh();
  }
  return (
    <div className="mt-3">
      <label className="flex items-start gap-2 text-[14px]">
        <input type="checkbox" checked={on} onChange={flip} disabled={busy} className="mt-1" />
        <span>Add sales tax at checkout with Stripe Tax. Turn on Stripe Tax in your own Stripe dashboard first, or buyers won&apos;t be able to pay.</span>
      </label>
      {error && <p role="alert" className="mt-1 text-[13px] text-slab">{error}</p>}
    </div>
  );
}
