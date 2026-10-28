"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { send } from "@/lib/client";
import { endTime, money, timeLeft, toCents } from "@/lib/format";
import type { BidHistory, Listing } from "@/lib/types";
import { HeartIcon } from "./Icons";

type Me = { watching: boolean; own: boolean };
type BidResult = { leading: boolean; current_cents: number; min_next_cents: number; won: boolean; reserve_met: boolean };

/** Bidding box for an auction: price, time left, your status, bid and Buy It Now. */
export default function AuctionPanel({ l, history, signedIn }: { l: Listing; history: BidHistory; signedIn: boolean }) {
  const router = useRouter();
  const a = l.auction!;
  const [now, setNow] = useState(() => Date.now());
  const [me, setMe] = useState<Me | null>(null);
  const [amount, setAmount] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [note, setNote] = useState("");
  const [confirmBuy, setConfirmBuy] = useState(false);
  const signIn = `/signin?next=${encodeURIComponent(`/item/${l.id}`)}`;

  const ended = a.state !== "live" || Date.parse(a.ends_at) <= now;
  const min = history.min_next_cents;
  const left = timeLeft(a.ends_at, now);
  const urgent = !ended && Date.parse(a.ends_at) - now < 60 * 60 * 1000;

  // Tick every second; refresh the page when the auction ends, and keep the
  // price fresh during the last hour.
  useEffect(() => {
    if (ended) return;
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [ended]);
  useEffect(() => {
    if (!ended) return;
    if (a.state === "live") {
      const t = setTimeout(() => router.refresh(), 3000);
      return () => clearTimeout(t);
    }
  }, [ended, a.state, router]);
  useEffect(() => {
    if (!urgent) return;
    const t = setInterval(() => router.refresh(), 20000);
    return () => clearInterval(t);
  }, [urgent, router]);
  useEffect(() => {
    if (!signedIn) return;
    send<Me>("GET", `/v1/listings/${l.id}/me`).then((r) => r.ok && setMe(r.data));
  }, [l.id, signedIn]);

  async function placeBid(cents: number | null) {
    if (!signedIn) return router.push(signIn);
    setError("");
    setNote("");
    if (cents == null) return setError("Enter a dollar amount, like 12.50.");
    setBusy("bid");
    const r = await send<BidResult>("POST", `/v1/listings/${l.id}/bids`, { max_cents: cents });
    setBusy("");
    if (!r.ok) return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    setAmount("");
    if (r.data.won) setNote("You won! The price reached the seller's limit.");
    else if (r.data.leading) setNote(`You're the high bidder at ${money(r.data.current_cents)}.`);
    else setNote(`Another bidder's maximum is higher. The price is now ${money(r.data.current_cents)}.`);
    router.refresh();
  }

  async function buyNow() {
    if (!signedIn) return router.push(signIn);
    setBusy("buy");
    const r = await send("POST", `/v1/listings/${l.id}/buy-now`);
    setBusy("");
    if (!r.ok) {
      setConfirmBuy(false);
      return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    }
    router.push("/purchases");
  }

  async function toggleWatch() {
    if (!signedIn) return router.push(signIn);
    const r = await send(me?.watching ? "DELETE" : "PUT", `/v1/watchlist/${l.id}`);
    if (r.ok) setMe((m) => (m ? { ...m, watching: !m.watching } : m));
  }

  const priceLabel = ended ? (a.state === "won" ? "Winning bid" : "Final price") : a.bid_count ? "Current bid" : "Starting bid";

  return (
    <div className="mt-4">
      {history.won && (
        <div className="mb-3 rounded-lg bg-ship/10 p-3 text-[14px] font-semibold text-ship">
          You won this auction. <Link href="/purchases" className="underline">Pay from Purchases</Link> within 4 days.
        </div>
      )}
      {!ended && history.leading && (
        <div className="mb-3 rounded-lg bg-ship/10 p-3 text-[14px] font-semibold text-ship">You&apos;re the high bidder.</div>
      )}
      {!ended && history.outbid && (
        <div className="mb-3 rounded-lg bg-slab/10 p-3 text-[14px] font-semibold text-slab">You&apos;ve been outbid. Bid again to win.</div>
      )}

      <div className="rounded-lg border border-line p-4">
        <p className="text-[13px] text-muted">{priceLabel}</p>
        <p className="num text-[32px] font-extrabold leading-none">{money(a.current_cents)}</p>
        <p className="mt-1 text-[14px]">
          <a href="#bids" className="text-link hover:underline">{a.bid_count} {a.bid_count === 1 ? "bid" : "bids"}</a>
          {a.has_reserve && (
            <span className={`ml-2 ${a.reserve_met ? "text-ship" : "text-muted"}`}>
              · {a.reserve_met ? "Reserve met" : "Reserve not met"}
            </span>
          )}
        </p>
        {history.my_max_cents != null && !ended && (
          <p className="mt-1 text-[13px] text-muted">Your maximum bid: {money(history.my_max_cents)}</p>
        )}

        <div className="mt-3 border-t border-line pt-3 text-[14px]">
          {ended ? (
            <p className="font-semibold">
              {a.state === "won" ? "Sold" : a.state === "live" ? "Ending…" : "Ended without a sale"}
              {a.ended_at && <span suppressHydrationWarning className="font-normal text-muted"> · {endTime(a.ended_at)}</span>}
            </p>
          ) : (
            <p>
              <span suppressHydrationWarning className={`num font-semibold ${urgent ? "text-slab" : ""}`}>{left} left</span>
              <span suppressHydrationWarning className="text-muted"> · ends {endTime(a.ends_at)}</span>
            </p>
          )}
        </div>
      </div>

      {me?.own ? (
        <div className="mt-4 rounded-lg bg-wash p-4 text-[14px]">
          This is your auction. {a.reserve_cents != null && <>Your reserve is {money(a.reserve_cents)}. </>}
          <Link href="/sell" className="text-link underline">Manage it in the seller hub</Link>.
        </div>
      ) : !ended ? (
        <div className="mt-4 grid gap-2">
          <form
            onSubmit={(e) => {
              e.preventDefault();
              placeBid(toCents(amount));
            }}
            className="flex gap-2"
          >
            <label htmlFor="max-bid" className="sr-only">Your maximum bid</label>
            <div className="flex flex-1 items-center rounded-full border border-line px-4 focus-within:border-ink">
              <span className="text-muted">$</span>
              <input
                id="max-bid"
                inputMode="decimal"
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                placeholder={`${(min / 100).toFixed(2)} or more`}
                className="num min-w-0 flex-1 bg-transparent px-1 py-3 outline-none"
              />
            </div>
            <button type="submit" disabled={busy !== ""} className="rounded-full bg-link px-6 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
              {busy === "bid" ? "Bidding…" : "Place bid"}
            </button>
          </form>
          <button type="button" onClick={() => placeBid(min)} disabled={busy !== ""}
            className="rounded-full border border-link py-2.5 font-semibold text-link hover:bg-wash disabled:opacity-60">
            Bid {money(min)}
          </button>
          <p className="text-[12px] text-muted">
            Enter the most you&apos;ll pay. We bid for you, one step at a time, only as much as needed to keep you ahead.
            {history.cap_cents < 100000 && <> This seller&apos;s limit is {money(history.cap_cents)}; the auction ends if bidding reaches it.</>}
          </p>

          {a.buy_now_cents != null && (
            confirmBuy ? (
              <div className="rounded-lg border border-ink p-3 text-[14px]">
                <p>Buy it now for <span className="num font-bold">{money(a.buy_now_cents)}</span>? This ends the auction and you pay within 4 days.</p>
                <div className="mt-2 flex gap-2">
                  <button type="button" onClick={buyNow} disabled={busy !== ""} className="rounded-full bg-link px-5 py-2 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
                    {busy === "buy" ? "Buying…" : "Confirm"}
                  </button>
                  <button type="button" onClick={() => setConfirmBuy(false)} className="rounded-full px-4 py-2 font-semibold text-link hover:bg-wash">Cancel</button>
                </div>
              </div>
            ) : (
              <button type="button" onClick={() => (signedIn ? setConfirmBuy(true) : router.push(signIn))}
                className="rounded-full bg-ink py-2.5 font-semibold text-white hover:opacity-90">
                Buy It Now {money(a.buy_now_cents)}
              </button>
            )
          )}

          <button type="button" onClick={toggleWatch} aria-pressed={me?.watching ?? false}
            className="flex items-center justify-center gap-2 rounded-full border border-line py-2.5 font-semibold hover:border-ink">
            <span className={me?.watching ? "text-slab" : ""}><HeartIcon filled={me?.watching} /></span>
            {me?.watching ? "Watching" : "Add to Watchlist"}
          </button>
          {note && <p role="status" className="text-center text-[14px] text-ship">{note}</p>}
          {error && <p role="alert" className="text-center text-[14px] text-slab">{error}</p>}
          {!signedIn && <p className="text-center text-[13px] text-muted">You&apos;ll be asked to sign in or register first.</p>}
        </div>
      ) : null}
    </div>
  );
}

/** Public bid history under the item details. */
export function BidList({ history, auction }: { history: BidHistory; auction: NonNullable<Listing["auction"]> }) {
  return (
    <section id="bids" className="mt-8">
      <h2 className="border-b border-line pb-2 text-[20px] font-bold">Bid history</h2>
      <p className="mt-2 text-[14px] text-muted">
        {history.bidders} {history.bidders === 1 ? "bidder" : "bidders"} · {auction.bid_count} {auction.bid_count === 1 ? "bid" : "bids"} ·
        starting bid {money(auction.start_cents)}
        {auction.round > 0 && " · relisted once"}
      </p>
      {history.bids.length === 0 ? (
        <p className="mt-3 text-[14px]">No bids yet. Be the first.</p>
      ) : (
        <table className="mt-3 w-full max-w-[560px] text-[14px]">
          <thead>
            <tr className="border-b border-line text-left text-muted">
              <th className="py-2 font-normal">Bidder</th>
              <th className="py-2 text-right font-normal">Bid</th>
              <th className="py-2 text-right font-normal">Time</th>
            </tr>
          </thead>
          <tbody>
            {history.bids.map((b, i) => (
              <tr key={i} className="border-b border-wash">
                <td className="py-2">{b.bidder}{b.you && <span className="ml-1 text-muted">(you)</span>}</td>
                <td className="num py-2 text-right">{money(b.cents)}</td>
                <td suppressHydrationWarning className="py-2 text-right text-muted">{endTime(b.at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <p className="mt-2 text-[12px] text-muted">Bidder names are shortened for privacy.</p>
    </section>
  );
}
