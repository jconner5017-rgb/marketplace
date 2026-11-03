"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";
import type { Redemption } from "@/lib/promos";

/** Seller hub: enter a promo code for fee-free sales, and see what's left. */
export default function PromoBox({ left, redemptions, initial }: { left: number; redemptions: Redemption[]; initial?: string }) {
  const router = useRouter();
  const [code, setCode] = useState(initial ?? "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [ok, setOk] = useState("");
  async function apply(e: React.FormEvent) {
    e.preventDefault();
    if (!code.trim()) return;
    setBusy(true);
    setError("");
    setOk("");
    const r = await send<{ fee_free_sales_left: number }>("POST", "/v1/seller/promos", { code });
    setBusy(false);
    if (!r.ok) return setError(r.message);
    setOk(`Applied. You have ${r.data.fee_free_sales_left} fee-free ${r.data.fee_free_sales_left === 1 ? "sale" : "sales"}.`);
    setCode("");
    router.refresh();
  }
  const active = redemptions.filter((r) => (r.remaining ?? 0) > 0);
  return (
    <section className="rounded-lg border border-line p-4 text-[14px]">
      <h2 className="font-bold">{left > 0 ? `${left} fee-free ${left === 1 ? "sale" : "sales"} left` : "Promo code"}</h2>
      <p className="mt-1 text-muted">
        {left > 0
          ? "No marketplace fee on your next cards sold, one sale per card. Stripe's card processing still applies."
          : "Have a promo code? Enter it for sales with no marketplace fee."}
      </p>
      {active.length > 0 && (
        <ul className="mt-2 space-y-1">
          {active.map((r) => (
            <li key={r.id} className="text-[13px]">
              <span className="font-mono font-semibold">{r.code}</span>: {r.remaining} of {r.sales_granted} left
              {r.expires_at && <span className="text-muted"> · until {new Date(r.expires_at).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" })}</span>}
            </li>
          ))}
        </ul>
      )}
      <form onSubmit={apply} className="mt-3 flex flex-wrap gap-2">
        <label htmlFor="promo" className="sr-only">Promo code</label>
        <input id="promo" value={code} onChange={(e) => setCode(e.target.value)} placeholder="Promo code" autoCapitalize="characters"
          className="w-[200px] rounded-md border border-line px-3 py-1.5 font-mono uppercase" />
        <button type="submit" disabled={busy || !code.trim()} className="rounded-full bg-link px-4 py-1.5 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
          {busy ? "Applying…" : "Apply"}
        </button>
      </form>
      {error && <p role="alert" className="mt-1 text-[13px] text-slab">{error}</p>}
      {ok && <p role="status" className="mt-1 text-[13px] text-ship">{ok}</p>}
    </section>
  );
}
