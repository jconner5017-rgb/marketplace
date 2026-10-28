"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

export default function ListingActions({ id, status }: { id: string; status: string }) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [confirming, setConfirming] = useState(false);

  async function patch(body: object) {
    setBusy(true);
    await send("PATCH", `/v1/seller/listings/${id}`, body);
    setBusy(false);
    router.refresh();
  }
  async function remove() {
    setBusy(true);
    await send("DELETE", `/v1/seller/listings/${id}`);
    setBusy(false);
    setConfirming(false);
    router.refresh();
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
