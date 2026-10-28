"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

export default function BundleDiscountForm({ type, value }: { type: string | null; value: number | null }) {
  const router = useRouter();
  const [kind, setKind] = useState(type ?? "");
  const [amount, setAmount] = useState(
    value == null ? "" : type === "percent_3plus" ? String(value / 100) : (value / 100).toFixed(2),
  );
  const [msg, setMsg] = useState("");

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setMsg("");
    const n = Number(amount);
    const body =
      kind === ""
        ? {}
        : { bundle_discount_type: kind, bundle_discount_val: kind === "percent_3plus" ? Math.round(n * 100) : Math.round(n * 100) };
    const res = await send("PATCH", "/v1/seller/settings", body);
    if (!res.ok) {
      setMsg(Object.values(res.fields)[0] ?? res.message);
      return;
    }
    setMsg("Saved");
    router.refresh();
  }

  return (
    <form onSubmit={save} className="mt-3 flex flex-wrap items-end gap-2 text-[14px]">
      <label className="flex flex-col">
        <span className="sr-only">Discount type</span>
        <select value={kind} onChange={(e) => setKind(e.target.value)} className="rounded border border-line px-2 py-1.5">
          <option value="">No discount</option>
          <option value="per_extra_item">$ off each extra card</option>
          <option value="percent_3plus">% off orders of 3+ cards</option>
        </select>
      </label>
      {kind && (
        <label className="flex items-center gap-1">
          <span>{kind === "percent_3plus" ? "" : "$"}</span>
          <input
            inputMode="decimal"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            aria-label={kind === "percent_3plus" ? "Percent off" : "Dollars off each extra card"}
            className="w-20 rounded border border-line px-2 py-1.5"
          />
          <span>{kind === "percent_3plus" ? "%" : ""}</span>
        </label>
      )}
      <button type="submit" className="rounded-full border border-ink px-4 py-1.5 font-semibold hover:bg-wash">Save discount</button>
      {msg && <span role="status" className={msg === "Saved" ? "text-ship" : "text-slab"}>{msg}</span>}
    </form>
  );
}
