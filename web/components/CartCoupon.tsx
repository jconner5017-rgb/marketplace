"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

/** "Have a coupon?" for one seller in the cart (2026-11-20). */
export default function CartCoupon({ seller, code, note }: { seller: string; code?: string; note?: string }) {
  const router = useRouter();
  const [open, setOpen] = useState(!!code);
  const [value, setValue] = useState(code ?? "");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  async function apply(c: string) {
    setBusy(true);
    setErr("");
    const r = await send("PUT", "/v1/cart/coupon", { seller, code: c });
    setBusy(false);
    if (!r.ok) return setErr(r.fields.coupon ?? r.message);
    if (!c) setValue("");
    router.refresh();
  }

  if (!open) {
    return (
      <button type="button" className="text-[13px] font-semibold text-link hover:underline" onClick={() => setOpen(true)}>
        Have a coupon from {seller}?
      </button>
    );
  }
  return (
    <div className="text-[13px]">
      <form className="flex flex-wrap items-center gap-2" onSubmit={(e) => { e.preventDefault(); apply(value); }}>
        <label htmlFor={`coupon-${seller}`} className="sr-only">Coupon code for {seller}</label>
        <input id={`coupon-${seller}`} value={value} maxLength={20} placeholder="Coupon code" autoComplete="off"
          onChange={(e) => setValue(e.target.value.toUpperCase())}
          className="w-40 rounded-md border border-line px-2.5 py-1.5 font-mono uppercase" />
        <button disabled={busy || !value.trim()} className="rounded-md border border-ink px-3 py-1.5 font-semibold hover:bg-wash disabled:opacity-50">
          {busy ? "Checking…" : code && code === value ? "Applied" : "Apply"}
        </button>
        {code && <button type="button" disabled={busy} className="text-muted hover:underline" onClick={() => apply("")}>Remove</button>}
      </form>
      {err && <p className="mt-1 text-slab" role="alert">{err}</p>}
      {!err && note && <p className="mt-1 text-muted">{note}</p>}
    </div>
  );
}
