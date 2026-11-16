"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";
import { money } from "@/lib/format";

type Quote = {
  rate_id: string; cost_cents: number; card_charge_cents: number; balance_cents: number; card: string;
  weight_oz: number; rigid: boolean; test_mode: boolean;
};

function msg(m: string) {
  return m.charAt(0).toUpperCase() + m.slice(1) + ".";
}

/**
 * Skip the stamp: buy USPS First-Class letter postage for an envelope order,
 * paid from the postage balance or the saved card, then print it on the
 * envelope.
 */
export default function BuyPostage({ orderId }: { orderId: string }) {
  const router = useRouter();
  const [rigid, setRigid] = useState(false);
  const [quote, setQuote] = useState<Quote | null>(null);
  const [pay, setPay] = useState<"balance" | "card">("balance");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function price(r = rigid) {
    setBusy(true);
    setError("");
    const res = await send<{ quote: Quote }>("POST", `/v1/orders/${orderId}/postage/quote`, { rigid: r });
    setBusy(false);
    if (!res.ok) return setError(msg(res.message));
    const q = res.data.quote;
    setQuote(q);
    setPay(q.balance_cents >= q.cost_cents || !q.card ? "balance" : "card");
  }

  async function buy() {
    if (!quote) return;
    setBusy(true);
    setError("");
    const res = await send("POST", `/v1/orders/${orderId}/postage`, { rate_id: quote.rate_id, pay });
    if (!res.ok) {
      setBusy(false);
      return setError(msg(res.message));
    }
    router.push(`/sell/orders/${orderId}/envelope`);
  }

  const lowBalance = quote ? quote.balance_cents < quote.cost_cents : false;
  const canBuy = quote && (quote.test_mode || (pay === "balance" ? !lowBalance : !!quote.card));

  return (
    <div className="mt-4 rounded-lg bg-wash p-3 text-[14px]">
      <p className="font-semibold">Skip the stamp: buy postage here</p>
      <p className="text-[13px] text-muted">
        USPS First-Class letter postage printed right on the envelope, at USPS&apos;s price (about 78¢ for 1 oz, 4¢ less than a stamp).
        A stamp works just as well; this is optional.{" "}
        <Link href="/help/envelope-postage" className="text-link underline">How it works</Link>
      </p>
      <label className="mt-2 flex items-center gap-2 text-[13px]">
        <input type="checkbox" checked={rigid} onChange={(e) => { setRigid(e.target.checked); if (quote) price(e.target.checked); }} />
        Rigid envelope (a toploader that won&apos;t bend, or a thick card): USPS adds 49¢
      </label>
      {!quote ? (
        <button type="button" onClick={() => price()} disabled={busy}
          className="mt-2 rounded-full border border-ink px-4 py-1.5 text-[14px] font-semibold hover:bg-ink hover:text-white disabled:opacity-60">
          {busy ? "Asking USPS…" : "See postage price"}
        </button>
      ) : (
        <div className="mt-2 space-y-2">
          <p>
            Postage: <b>{money(quote.cost_cents)}</b> <span className="text-muted">({quote.weight_oz} oz letter{quote.rigid ? ", rigid" : ""})</span>
          </p>
          {quote.test_mode && (
            <p className="rounded-md border border-dashed border-slab p-2 text-[13px]">
              Test mode: the postage prints marked VOID and nothing is charged.
            </p>
          )}
          <fieldset className="space-y-1 text-[14px]">
            <legend className="sr-only">Pay with</legend>
            <label className="flex items-start gap-2">
              <input type="radio" name={`pay-${orderId}`} checked={pay === "balance"} onChange={() => setPay("balance")} className="mt-1" />
              <span>
                Postage balance: {money(quote.cost_cents)}{" "}
                <span className="text-muted">(you have {money(quote.balance_cents)})</span>
                {lowBalance && !quote.test_mode && (
                  <> · <Link href="/sell/shipping#postage" className="text-link underline">Add funds</Link></>
                )}
              </span>
            </label>
            <label className="flex items-start gap-2">
              <input type="radio" name={`pay-${orderId}`} checked={pay === "card"} onChange={() => setPay("card")} className="mt-1" disabled={!quote.card && !quote.test_mode} />
              <span>
                {quote.card ? `${quote.card}: ` : "Card: "}{money(quote.card_charge_cents)}{" "}
                <span className="text-muted">(postage plus card processing)</span>
                {!quote.card && <> · <Link href="/sell/shipping" className="text-link underline">Save a card</Link></>}
              </span>
            </label>
          </fieldset>
          <button type="button" onClick={buy} disabled={busy || !canBuy}
            className="rounded-full bg-link px-4 py-2 text-[14px] font-semibold text-white hover:bg-link-dark disabled:opacity-50">
            {busy ? "Buying…" : quote.test_mode ? "Buy test postage" : `Buy postage for ${money(pay === "card" ? quote.card_charge_cents : quote.cost_cents)}`}
          </button>
          <p className="text-[12px] text-muted">Buying marks the order shipped and gives the buyer the tracking number. You can void unused postage for a refund.</p>
        </div>
      )}
      {error && <p role="alert" className="mt-2 text-[13px] text-slab">{error}</p>}
    </div>
  );
}
