"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

export default function CartLineActions({ id, quantity, max, available }: { id: string; quantity: number; max: number; available: boolean }) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  async function run(method: string, body?: object) {
    setBusy(true);
    await send(method, `/v1/cart/${id}`, body);
    setBusy(false);
    router.refresh();
  }
  return (
    <div className="mt-2 flex items-center gap-4 text-[13px]">
      {available && max > 1 && (
        <label className="flex items-center gap-1.5">
          Qty
          <select value={quantity} disabled={busy} onChange={(e) => run("PUT", { quantity: Number(e.target.value) })}
            className="rounded border border-line px-1.5 py-0.5">
            {Array.from({ length: Math.min(max, 20) }, (_, i) => i + 1).map((n) => <option key={n} value={n}>{n}</option>)}
          </select>
        </label>
      )}
      <button type="button" disabled={busy} onClick={() => run("DELETE")} className="text-link underline disabled:opacity-60">Remove</button>
    </div>
  );
}
