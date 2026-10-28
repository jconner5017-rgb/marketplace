"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { send } from "@/lib/client";
import { HeartIcon } from "./Icons";

type Me = { watching: boolean; in_cart: number; own: boolean; thread_id: string };

/** Buy It Now, Add to cart, Watch and Contact seller. Signed-out visitors are sent to sign in first. */
export default function ItemActions({ id, signedIn, quantity }: { id: string; signedIn: boolean; quantity: number }) {
  const router = useRouter();
  const [me, setMe] = useState<Me | null>(null);
  const [busy, setBusy] = useState<"" | "buy" | "cart" | "watch">("");
  const [error, setError] = useState("");
  const [qty, setQty] = useState(1);
  const signIn = `/signin?next=${encodeURIComponent(`/item/${id}`)}`;

  useEffect(() => {
    if (!signedIn) return;
    send<Me>("GET", `/v1/listings/${id}/me`).then((r) => r.ok && setMe(r.data));
  }, [id, signedIn]);

  async function addToCart(then: "stay" | "cart") {
    if (!signedIn) return router.push(signIn);
    setBusy(then === "cart" ? "buy" : "cart");
    setError("");
    const r = await send("PUT", `/v1/cart/${id}`, { quantity: Math.max(qty, me?.in_cart ?? 0) });
    setBusy("");
    if (!r.ok) return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    if (then === "cart") return router.push("/cart");
    setMe((m) => (m ? { ...m, in_cart: qty } : m));
    router.refresh();
  }

  async function toggleWatch() {
    if (!signedIn) return router.push(signIn);
    setBusy("watch");
    const r = await send(me?.watching ? "DELETE" : "PUT", `/v1/watchlist/${id}`);
    setBusy("");
    if (r.ok) {
      setMe((m) => (m ? { ...m, watching: !m.watching } : m));
      router.refresh();
    } else setError(r.message);
  }

  if (me?.own) {
    return (
      <div className="mt-5 rounded-lg bg-wash p-4 text-[14px]">
        This is your listing. <Link href="/sell" className="text-link underline">Manage it in the seller hub</Link>.
      </div>
    );
  }

  return (
    <div className="mt-5 grid gap-2">
      {quantity > 1 && (
        <label className="mb-1 flex items-center gap-2 text-[14px]">
          Quantity
          <select value={qty} onChange={(e) => setQty(Number(e.target.value))} className="rounded border border-line px-2 py-1">
            {Array.from({ length: Math.min(quantity, 20) }, (_, i) => i + 1).map((n) => <option key={n} value={n}>{n}</option>)}
          </select>
          <span className="text-muted">{quantity} available</span>
        </label>
      )}
      <button type="button" onClick={() => addToCart("cart")} disabled={busy !== ""}
        className="rounded-full bg-link py-3 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
        {busy === "buy" ? "Adding…" : "Buy It Now"}
      </button>
      {me && me.in_cart > 0 ? (
        <Link href="/cart" className="rounded-full border border-link py-3 text-center font-semibold text-link hover:bg-wash">
          In your cart · View cart
        </Link>
      ) : (
        <button type="button" onClick={() => addToCart("stay")} disabled={busy !== ""}
          className="rounded-full border border-link py-3 font-semibold text-link hover:bg-wash disabled:opacity-60">
          {busy === "cart" ? "Adding…" : "Add to cart"}
        </button>
      )}
      <button type="button" onClick={toggleWatch} disabled={busy !== ""} aria-pressed={me?.watching ?? false}
        className="flex items-center justify-center gap-2 rounded-full border border-line py-3 font-semibold hover:border-ink disabled:opacity-60">
        <span className={me?.watching ? "text-slab" : ""}><HeartIcon filled={me?.watching} /></span>
        {me?.watching ? "Watching" : "Add to Watchlist"}
      </button>
      {error && <p role="alert" className="text-center text-[14px] text-slab">{error}</p>}
      {!signedIn && <p className="text-center text-[13px] text-muted">You&apos;ll be asked to sign in or register first.</p>}
    </div>
  );
}
