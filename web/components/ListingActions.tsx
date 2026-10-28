"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

export default function ListingActions({ id, status, auction }: {
  id: string; status: string; auction?: { bid_count: number; state: string };
}) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState("");

  async function patch(body: object) {
    setBusy(true);
    await send("PATCH", `/v1/seller/listings/${id}`, body);
    setBusy(false);
    router.refresh();
  }
  async function remove() {
    setBusy(true);
    const r = await send("DELETE", `/v1/seller/listings/${id}`);
    setBusy(false);
    setConfirming(false);
    if (!r.ok) return setError(r.message);
    router.refresh();
  }

  if (auction) {
    if (auction.state !== "live" || auction.bid_count > 0) {
      return <span className="text-[12px] text-muted">{auction.state === "live" ? "Has bids" : ""}</span>;
    }
    return confirming ? (
      <span className="inline-flex gap-2 text-[13px]">
        <button type="button" onClick={remove} disabled={busy} className="font-semibold text-slab underline">End auction</button>
        <button type="button" onClick={() => setConfirming(false)} className="text-muted underline">Keep</button>
      </span>
    ) : (
      <span className="text-[13px]">
        <button type="button" onClick={() => setConfirming(true)} className="text-muted underline">End early</button>
        {error && <span role="alert" className="block text-slab">{error}</span>}
      </span>
    );
  }

  if (confirming) {
    return (
      <span className="inline-flex gap-2 text-[13px]">
        <button type="button" onClick={remove} disabled={busy} className="font-semibold text-slab underline">Remove listing</button>
        <button type="button" onClick={() => setConfirming(false)} className="text-muted underline">Keep</button>
      </span>
    );
  }
  return (
    <span className="inline-flex gap-3 text-[13px]">
      {status === "hidden" ? (
        <button type="button" onClick={() => patch({ status: "active" })} disabled={busy} className="text-link underline">Show</button>
      ) : (
        <button type="button" onClick={() => patch({ status: "hidden" })} disabled={busy} className="text-link underline">Hide</button>
      )}
      <button type="button" onClick={() => setConfirming(true)} className="text-muted underline">Remove</button>
    </span>
  );
}
