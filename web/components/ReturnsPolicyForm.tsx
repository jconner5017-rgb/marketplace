"use client";

import { useState } from "react";
import { send } from "@/lib/client";

type Policy = "damaged_only" | "30_days";

const OPTIONS: { value: Policy; label: string; help: string }[] = [
  { value: "damaged_only", label: "Returns only if defective or not as described", help: "Within 30 days of delivery. No returns for a change of mind. The buyer pays return shipping." },
  { value: "30_days", label: "30-day returns, no questions asked", help: "Any reason, within 30 days of delivery. The buyer pays return shipping." },
];

/** The seller's return policy, shown on every one of their listings. */
export default function ReturnsPolicyForm({ initial }: { initial: Policy }) {
  const [v, setV] = useState<Policy>(initial);
  const [saved, setSaved] = useState<Policy>(initial);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  async function save() {
    setBusy(true);
    setMsg("");
    const r = await send("PUT", "/v1/seller/returns", { returns_policy: v });
    setBusy(false);
    if (!r.ok) return setMsg(r.message);
    setSaved(v);
    setMsg("Saved. Listings without their own policy show it now.");
  }

  return (
    <fieldset className="space-y-2">
      <legend className="sr-only">Return policy</legend>
      {OPTIONS.map((o) => (
        <label key={o.value} className="flex cursor-pointer gap-3 rounded-lg border border-line p-3 has-[:checked]:border-ink">
          <input type="radio" name="returns_policy" value={o.value} checked={v === o.value} onChange={() => setV(o.value)} className="mt-1" />
          <span>
            <span className="block font-semibold">{o.label}</span>
            <span className="block text-[13px] text-muted">{o.help}</span>
          </span>
        </label>
      ))}
      <div className="flex items-center gap-3 pt-1">
        <button type="button" onClick={save} disabled={busy || v === saved}
          className="rounded-full bg-link px-5 py-2 font-semibold text-white hover:bg-link-dark disabled:opacity-50">
          {busy ? "Saving…" : "Save return policy"}
        </button>
        {msg && <p role="status" className="text-[14px]">{msg}</p>}
      </div>
    </fieldset>
  );
}
