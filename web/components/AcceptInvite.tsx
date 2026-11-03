"use client";

import Link from "next/link";
import { useState } from "react";
import { send } from "@/lib/client";

/** Buyer: join a seller's trusted buyer list from their invite link. */
export default function AcceptInvite({ code, seller }: { code: string; seller: string }) {
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState("");
  async function go() {
    setBusy(true);
    setError("");
    const r = await send("POST", `/v1/trust/${encodeURIComponent(code)}`, {});
    setBusy(false);
    if (!r.ok) return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    setDone(true);
  }
  if (done) {
    return (
      <div className="mt-5 rounded-lg bg-wash p-4 text-[14px]">
        <p className="font-semibold">You&apos;re on {seller}&apos;s trusted buyer list.</p>
        <p className="mt-1 text-muted">When you buy one raw card up to $100 from {seller}, you&apos;ll be able to choose a stamped envelope at checkout.</p>
        <Link href={`/search?seller=${encodeURIComponent(seller)}`} className="mt-3 inline-block rounded-full bg-link px-4 py-2 font-semibold text-white hover:bg-link-dark">See {seller}&apos;s cards</Link>
      </div>
    );
  }
  return (
    <div className="mt-5">
      <button type="button" onClick={go} disabled={busy} className="rounded-full bg-link px-5 py-2.5 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
        {busy ? "Joining…" : `Accept and join ${seller}'s list`}
      </button>
      {error && <p role="alert" className="mt-2 text-[13px] text-slab">{error}</p>}
    </div>
  );
}
