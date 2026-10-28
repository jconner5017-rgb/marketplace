"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";
import { money, toCents } from "@/lib/format";
import type { Offer } from "@/lib/types";

/** "Make offer" on a Buy It Now listing that accepts offers. */
export default function OfferBox({
  id, price, quantity, signedIn, offersLeft, pendingOfferId,
}: {
  id: string; price: number; quantity: number; signedIn: boolean; offersLeft: number; pendingOfferId: string | null;
}) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [amount, setAmount] = useState("");
  const [qty, setQty] = useState(1);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<Offer | null>(null);

  if (pendingOfferId && !result) {
    return (
      <p className="rounded-lg bg-wash p-3 text-center text-[14px]">
        Your offer is waiting for the seller. <Link href="/offers" className="text-link underline">See your offers</Link>
      </p>
    );
  }
  if (result) {
    const o = result;
    return (
      <div role="status" className={`rounded-lg p-3 text-[14px] ${o.status === "accepted" ? "bg-ship/10 text-ship" : "bg-wash"}`}>
        {o.status === "accepted" && <>Offer accepted at {money(o.amount_cents)}. <Link href="/purchases" className="font-semibold underline">Pay within 48 hours</Link>.</>}
        {o.status === "pending" && <>Offer of {money(o.amount_cents)} sent. The seller has 48 hours to reply. <Link href="/offers" className="text-link underline">Your offers</Link></>}
        {o.status === "declined" && <>The seller declined {money(o.amount_cents)}. {o.offers_left > 0 ? `You can make ${o.offers_left} more ${o.offers_left === 1 ? "offer" : "offers"}.` : "You've used all your offers on this card."}</>}
        {o.status === "declined" && o.offers_left > 0 && (
          <button type="button" onClick={() => { setResult(null); setAmount(""); }} className="ml-2 font-semibold text-link underline">Try again</button>
        )}
      </div>
    );
  }
  if (offersLeft <= 0) {
    return <p className="text-center text-[13px] text-muted">You&apos;ve made the most offers allowed on this card.</p>;
  }
  if (!open) {
    return (
      <button type="button" onClick={() => (signedIn ? setOpen(true) : router.push(`/signin?next=${encodeURIComponent(`/item/${id}`)}`))}
        className="rounded-full border border-link py-3 font-semibold text-link hover:bg-wash">
        Make offer
      </button>
    );
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    const cents = toCents(amount);
    if (cents == null || cents < 1) return setError("Enter a dollar amount, like 12.50.");
    if (cents >= price) return setError(`Offers must be under ${money(price)}. Use Buy It Now to pay the full price.`);
    setBusy(true);
    const r = await send<{ offer: Offer }>("POST", `/v1/listings/${id}/offers`, { amount_cents: cents, quantity: qty, message: note });
    setBusy(false);
    if (!r.ok) return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    setResult(r.data.offer);
    router.refresh();
  }

  return (
    <form onSubmit={submit} className="rounded-lg border border-ink p-4 text-[14px]">
      <p className="font-semibold">Make an offer</p>
      <p className="text-muted">Listed at {money(price)}. You can make up to {offersLeft} {offersLeft === 1 ? "offer" : "offers"}; each lasts 48 hours.</p>
      <div className="mt-3 flex gap-2">
        <label className="flex flex-1 items-center rounded-md border border-line px-3 focus-within:border-ink">
          <span className="sr-only">Your offer per card</span>
          <span className="text-muted">$</span>
          <input inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} placeholder="Your offer"
            className="num min-w-0 flex-1 bg-transparent px-1 py-2 outline-none" autoFocus />
        </label>
        {quantity > 1 && (
          <label className="flex items-center gap-1">
            <span className="text-muted">Qty</span>
            <select value={qty} onChange={(e) => setQty(Number(e.target.value))} className="rounded-md border border-line px-2 py-2">
              {Array.from({ length: Math.min(quantity, 20) }, (_, i) => i + 1).map((n) => <option key={n} value={n}>{n}</option>)}
            </select>
          </label>
        )}
      </div>
      <label className="mt-2 block">
        <span className="text-muted">Note to the seller (optional)</span>
        <textarea value={note} onChange={(e) => setNote(e.target.value)} rows={2} maxLength={500}
          className="mt-1 w-full rounded-md border border-line px-3 py-2" />
      </label>
      {error && <p role="alert" className="mt-2 text-slab">{error}</p>}
      <div className="mt-3 flex gap-2">
        <button type="submit" disabled={busy} className="rounded-full bg-link px-6 py-2 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
          {busy ? "Sending…" : "Send offer"}
        </button>
        <button type="button" onClick={() => setOpen(false)} className="rounded-full px-4 py-2 font-semibold text-link hover:bg-wash">Cancel</button>
      </div>
      <p className="mt-2 text-[12px] text-muted">If the seller accepts, the card is held for you and you pay within 48 hours.</p>
    </form>
  );
}
