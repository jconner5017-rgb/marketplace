"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";
import { money } from "@/lib/format";

type Rate = { id: string; carrier: string; service: string; amount_cents: number; days: number; charge_cents: number; processing_cents: number; signature: boolean };

function msg(m: string) {
  return m.charAt(0).toUpperCase() + m.slice(1) + ".";
}

type Parcel = { length: number; width: number; height: number; weight_oz: number };

// Common packages for cards. "Suggested" is what the site worked out from the order.
const BOXES: { key: string; label: string; dims?: [number, number, number] }[] = [
  { key: "suggested", label: "Suggested for this order" },
  { key: "bubble", label: "Bubble mailer (7¼ × 5¼ × ¾ in)", dims: [7.25, 5.25, 0.75] },
  { key: "small", label: "Small box (6 × 4 × 2 in)", dims: [6, 4, 2] },
  { key: "medium", label: "Medium box (8 × 6 × 4 in)", dims: [8, 6, 4] },
  { key: "large", label: "Large box (12 × 9 × 6 in)", dims: [12, 9, 6] },
  { key: "custom", label: "Custom size…" },
];
const SAVED = "ss-label-box";

function inches(v: number) {
  return String(Math.round(v * 100) / 100);
}

/** Buy a shipping label for an order: pick the box and weight, see live rates, then charge the saved card. */
export function BuyLabel({ orderId }: { orderId: string }) {
  const router = useRouter();
  const [rates, setRates] = useState<Rate[] | null>(null);
  const [pick, setPick] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [suggested, setSuggested] = useState<Parcel | null>(null);
  const [box, setBox] = useState("suggested");
  const [dims, setDims] = useState({ l: "", w: "", h: "" });
  const [lb, setLb] = useState("0");
  const [oz, setOz] = useState("0");
  const [dirty, setDirty] = useState(false);

  function setWeight(totalOz: number) {
    const whole = Math.floor(totalOz / 16);
    setLb(String(whole));
    setOz(String(Math.round((totalOz - whole * 16) * 10) / 10));
  }

  function current(): Parcel | null {
    const weight = (Number(lb) || 0) * 16 + (Number(oz) || 0);
    if (box === "suggested") return suggested ? { ...suggested, weight_oz: weight } : null;
    const preset = BOXES.find((b) => b.key === box)?.dims;
    const [l, w, h] = preset ?? [Number(dims.l), Number(dims.w), Number(dims.h)];
    return { length: l, width: w, height: h, weight_oz: weight };
  }

  async function load(parcel?: Parcel | null) {
    setBusy(true);
    setError("");
    const r = await send<{ rates: Rate[]; parcel: Parcel }>("POST", `/v1/orders/${orderId}/label/rates`, parcel ? { parcel } : {});
    setBusy(false);
    if (!r.ok) return setError(msg(r.message));
    if (!parcel) {
      setSuggested(r.data.parcel);
      setWeight(r.data.parcel.weight_oz);
      try {
        const saved = JSON.parse(localStorage.getItem(SAVED) || "null");
        if (saved?.l) setDims(saved);
      } catch {
        /* no saved box */
      }
    }
    setRates(r.data.rates);
    setPick(r.data.rates[0]?.id ?? "");
    setDirty(false);
  }

  function update() {
    const p = current();
    if (!p || !(p.length > 0 && p.width > 0 && p.height > 0)) return setError("Enter the box's length, width and height in inches.");
    if (!(p.weight_oz > 0)) return setError("Enter the package weight.");
    if (box === "custom") {
      try {
        localStorage.setItem(SAVED, JSON.stringify(dims));
      } catch {
        /* ignore */
      }
    }
    load(p);
  }

  async function buy() {
    setBusy(true);
    setError("");
    const r = await send("POST", `/v1/orders/${orderId}/label`, { rate_id: pick });
    setBusy(false);
    if (!r.ok) return setError(msg(r.message));
    router.refresh();
  }
  const chosen = rates?.find((x) => x.id === pick);
  const needsSetup = /ship-from|card for labels/.test(error);
  const change = <T,>(f: (v: T) => void) => (v: T) => { f(v); setDirty(true); };
  const num = "w-[72px] rounded-md border border-line bg-paper px-2 py-1.5 text-[16px] sm:text-[14px]";

  return (
    <div className="mt-3 text-[14px]">
      {!rates ? (
        <button type="button" onClick={() => load()} disabled={busy} className="rounded-full bg-link px-4 py-2 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
          {busy ? "Getting rates…" : "Buy label"}
        </button>
      ) : (
        <div className="space-y-4">
          <fieldset className="space-y-2 rounded-lg border border-line p-3">
            <legend className="px-1 font-semibold">Package</legend>
            <label htmlFor={`box-${orderId}`} className="sr-only">Box</label>
            <select id={`box-${orderId}`} value={box} onChange={(e) => change(setBox)(e.target.value)} className="w-full max-w-[360px] rounded-md border border-line bg-paper px-2 py-1.5">
              {BOXES.map((b) => (
                <option key={b.key} value={b.key}>
                  {b.key === "suggested" && suggested ? `${b.label} (${inches(suggested.length)} × ${inches(suggested.width)} × ${inches(suggested.height)} in)` : b.label}
                </option>
              ))}
            </select>
            {box === "custom" && (
              <div className="flex flex-wrap items-center gap-2">
                {(["l", "w", "h"] as const).map((k, i) => (
                  <label key={k} className="flex items-center gap-1">
                    <span className="text-muted">{["Length", "Width", "Height"][i]}</span>
                    <input inputMode="decimal" value={dims[k]} onChange={(e) => change(setDims)({ ...dims, [k]: e.target.value.replace(/[^0-9.]/g, "") })} className={num} />
                  </label>
                ))}
                <span className="text-muted">inches</span>
              </div>
            )}
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-muted">Weight packed</span>
              <label className="flex items-center gap-1">
                <input inputMode="numeric" value={lb} onChange={(e) => change(setLb)(e.target.value.replace(/[^0-9]/g, ""))} className={num} aria-label="Pounds" /> lb
              </label>
              <label className="flex items-center gap-1">
                <input inputMode="decimal" value={oz} onChange={(e) => change(setOz)(e.target.value.replace(/[^0-9.]/g, ""))} className={num} aria-label="Ounces" /> oz
              </label>
            </div>
            <p className="text-[12px] text-muted">Weigh the sealed package if you can. Carriers charge the difference if it&apos;s heavier than the label says.</p>
            {dirty && (
              <button type="button" onClick={update} disabled={busy} className="rounded-full border border-link px-4 py-1.5 font-semibold text-link hover:bg-wash disabled:opacity-60">
                {busy ? "Getting rates…" : "Update rates"}
              </button>
            )}
          </fieldset>

          <fieldset className={`space-y-2 ${dirty ? "opacity-50" : ""}`}>
            <legend className="font-semibold">Choose a service{rates[0]?.signature ? " (signature required: order over $750)" : ""}</legend>
            {rates.map((x) => (
              <label key={x.id} className="flex items-start gap-2">
                <input type="radio" name={`rate-${orderId}`} checked={pick === x.id} onChange={() => setPick(x.id)} disabled={dirty} className="mt-1" />
                <span>{x.carrier} {x.service}{x.days ? ` · about ${x.days} day${x.days === 1 ? "" : "s"}` : ""} · <span className="num font-semibold">{money(x.amount_cents)}</span></span>
              </label>
            ))}
            {chosen && !dirty && (
              <p className="num text-muted">Label {money(chosen.amount_cents)} + card processing {money(chosen.processing_cents)} = <strong className="text-ink">{money(chosen.charge_cents)}</strong> on your saved card</p>
            )}
            {dirty ? (
              <p className="text-[13px] text-muted">You changed the package. Choose Update rates to see prices for it.</p>
            ) : (
              <button type="button" onClick={buy} disabled={busy || !pick} className="rounded-full bg-link px-4 py-2 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
                {busy ? "Buying…" : `Buy for ${chosen ? money(chosen.charge_cents) : ""}`}
              </button>
            )}
          </fieldset>
        </div>
      )}
      {error && <p role="alert" className="mt-2 text-[13px] text-slab">{error} {needsSetup && <Link href="/sell/shipping" className="font-semibold underline">Shipping settings</Link>}</p>}
    </div>
  );
}

export function VoidLabel({ orderId }: { orderId: string }) {
  const router = useRouter();
  const [confirm, setConfirm] = useState(false);
  const [error, setError] = useState("");
  async function go() {
    const r = await send("POST", `/v1/orders/${orderId}/label/void`, {});
    if (!r.ok) {
      setConfirm(false);
      return setError(msg(r.message));
    }
    router.refresh();
  }
  return (
    <span className="text-[14px]">
      {confirm ? (
        <>Void it and refund your card? <button type="button" onClick={go} className="font-semibold text-slab underline">Void label</button>{" "}
          <button type="button" onClick={() => setConfirm(false)} className="text-muted underline">Keep</button></>
      ) : (
        <button type="button" onClick={() => setConfirm(true)} className="text-link underline">Void label</button>
      )}
      {error && <span role="alert" className="block text-[13px] text-slab">{error}</span>}
    </span>
  );
}
