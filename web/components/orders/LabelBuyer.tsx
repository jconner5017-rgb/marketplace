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

/** Buy a shipping label for an order: live rates, then charge the saved card. */
export function BuyLabel({ orderId }: { orderId: string }) {
  const router = useRouter();
  const [rates, setRates] = useState<Rate[] | null>(null);
  const [pick, setPick] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function load() {
    setBusy(true);
    setError("");
    const r = await send<{ rates: Rate[] }>("POST", `/v1/orders/${orderId}/label/rates`, {});
    setBusy(false);
    if (!r.ok) return setError(msg(r.message));
    setRates(r.data.rates);
    setPick(r.data.rates[0]?.id ?? "");
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

  return (
    <div className="mt-3 text-[14px]">
      {!rates ? (
        <button type="button" onClick={load} disabled={busy} className="rounded-full bg-link px-4 py-2 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
          {busy ? "Getting rates…" : "Buy label"}
        </button>
      ) : (
        <fieldset className="space-y-2">
          <legend className="font-semibold">Choose a service{rates[0]?.signature ? " (signature required: order over $750)" : ""}</legend>
          {rates.map((x) => (
            <label key={x.id} className="flex items-start gap-2">
              <input type="radio" name={`rate-${orderId}`} checked={pick === x.id} onChange={() => setPick(x.id)} className="mt-1" />
              <span>{x.carrier} {x.service}{x.days ? ` · about ${x.days} day${x.days === 1 ? "" : "s"}` : ""} · <span className="num font-semibold">{money(x.amount_cents)}</span></span>
            </label>
          ))}
          {chosen && (
            <p className="num text-muted">Label {money(chosen.amount_cents)} + card processing {money(chosen.processing_cents)} = <strong className="text-ink">{money(chosen.charge_cents)}</strong> on your saved card</p>
          )}
          <button type="button" onClick={buy} disabled={busy || !pick} className="rounded-full bg-link px-4 py-2 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
            {busy ? "Buying…" : `Buy for ${chosen ? money(chosen.charge_cents) : ""}`}
          </button>
        </fieldset>
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
