"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";
import { money } from "@/lib/format";

export type PostageAccount = {
  enabled: boolean; test_mode: boolean; balance_cents: number; card: string; min_cents: number; max_cents: number;
  history: { kind: string; amount_cents: number; balance_after: number; order_number: number | null; note: string; created_at: string }[];
};

// What the card pays to add cents: the amount plus Stripe's 2.9% + 30¢,
// the same as the API works it out.
function cardCharge(cents: number) {
  return Math.ceil(((cents + 30) * 10000) / (10000 - 290));
}

const PRESETS = [500, 1000, 2000, 5000];

/** The postage balance: what's left, add funds, and recent activity. */
export default function PostageBalance({ a }: { a: PostageAccount }) {
  const router = useRouter();
  const [dollars, setDollars] = useState("10");
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const cents = Math.round((parseFloat(dollars) || 0) * 100);
  const valid = cents >= a.min_cents && cents <= a.max_cents;

  async function add() {
    setBusy(true);
    setError("");
    setNote("");
    const r = await send<{ balance_cents: number }>("POST", "/v1/seller/postage/topup", { amount_cents: cents });
    setBusy(false);
    if (!r.ok) return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    setNote(`Added ${money(cents)}. Your balance is ${money(r.data.balance_cents)}.`);
    router.refresh();
  }

  return (
    <div>
      <p className="text-[15px]">Balance: <b className="text-[18px]">{money(a.balance_cents)}</b></p>
      {a.test_mode && (
        <p className="mt-2 rounded-md border border-dashed border-slab p-2 text-[13px]">
          Envelope postage is in test mode: test postage is free and doesn&apos;t use your balance.
        </p>
      )}
      {a.card ? (
        <div className="mt-3">
          <p className="text-[14px] font-semibold">Add funds</p>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            {PRESETS.map((p) => (
              <button key={p} type="button" onClick={() => setDollars(String(p / 100))}
                className={`rounded-full border px-3 py-1 text-[14px] ${cents === p ? "border-ink bg-ink text-white" : "border-line"}`}>
                {money(p)}
              </button>
            ))}
            <label className="flex items-center gap-1 text-[14px]">
              <span className="text-muted">or $</span>
              <span className="sr-only">Amount in dollars</span>
              <input value={dollars} onChange={(e) => setDollars(e.target.value)} inputMode="decimal"
                className="num w-20 rounded-md border border-line px-2 py-1" />
            </label>
          </div>
          <p className="mt-2 text-[13px] text-muted">
            {valid
              ? <>Adds {money(cents)} to your balance. {a.card} is charged {money(cardCharge(cents))} (includes card processing, paid once per top-up).</>
              : <>Choose {money(a.min_cents)} to {money(a.max_cents)}.</>}
          </p>
          <button type="button" onClick={add} disabled={busy || !valid}
            className="mt-2 rounded-full bg-link px-4 py-2 text-[14px] font-semibold text-white hover:bg-link-dark disabled:opacity-50">
            {busy ? "Adding…" : valid ? `Add ${money(cents)}` : "Add funds"}
          </button>
          {note && <p role="status" className="mt-2 text-[14px]">{note}</p>}
          {error && <p role="alert" className="mt-2 text-[13px] text-slab">{error}</p>}
        </div>
      ) : (
        <p className="mt-2 text-[14px] text-muted">Save a card below to add funds.</p>
      )}
      {a.history.length > 0 && (
        <table className="mt-4 w-full text-[13px]">
          <thead>
            <tr className="border-b border-line text-left text-muted">
              <th className="py-1.5 pr-2 font-semibold">Date</th>
              <th className="py-1.5 pr-2 font-semibold">What</th>
              <th className="py-1.5 pr-2 text-right font-semibold">Amount</th>
              <th className="py-1.5 text-right font-semibold">Balance</th>
            </tr>
          </thead>
          <tbody>
            {a.history.map((h, i) => (
              <tr key={i} className="border-b border-wash">
                <td className="py-1.5 pr-2">{new Date(h.created_at).toLocaleDateString()}</td>
                <td className="py-1.5 pr-2">{h.note}</td>
                <td className={`num py-1.5 pr-2 text-right ${h.amount_cents < 0 ? "" : "text-green-700"}`}>
                  {h.amount_cents < 0 ? "−" : "+"}{money(Math.abs(h.amount_cents))}
                </td>
                <td className="num py-1.5 text-right">{money(h.balance_after)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
