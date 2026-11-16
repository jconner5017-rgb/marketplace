"use client";

import { useState } from "react";
import { send } from "@/lib/client";

export type Addr = { name?: string; line1?: string; line2?: string; city?: string; state?: string; zip?: string; phone?: string };
type Rate = { id: string; carrier: string; service: string; rate: string; delivery_days?: number | null };
type Shipment = { id: string; rates: Rate[]; messages: { carrier: string; type: string; message: string }[] };
type Label = { tracking_code: string; label_url: string; label_size: string; rate: Rate };

const EMPTY: Addr = { name: "", line1: "", line2: "", city: "", state: "", zip: "" };

function AddrFields({ title, v, set }: { title: string; v: Addr; set: (a: Addr) => void }) {
  const f = (k: keyof Addr, label: string, cls = "") => (
    <label className={`block text-[13px] font-semibold ${cls}`}>{label}
      <input className="mt-1 w-full rounded-lg border border-line px-3 py-2 font-normal" value={v[k] ?? ""} onChange={(e) => set({ ...v, [k]: e.target.value })} />
    </label>
  );
  return (
    <fieldset className="rounded-xl border border-line p-4">
      <legend className="px-1 font-bold">{title}</legend>
      <div className="grid gap-3 sm:grid-cols-2">
        {f("name", "Name", "sm:col-span-2")}
        {f("line1", "Street", "sm:col-span-2")}
        {f("line2", "Apt / suite")}
        {f("city", "City")}
        {f("state", "State")}
        {f("zip", "ZIP")}
      </div>
    </fieldset>
  );
}

export default function PostageTest({ from: initialFrom, canBuy }: { from?: Addr; canBuy: boolean }) {
  const [from, setFrom] = useState<Addr>({ ...EMPTY, ...initialFrom });
  const [to, setTo] = useState<Addr>({ ...EMPTY, name: "Test Buyer", line1: "1600 Pennsylvania Ave NW", city: "Washington", state: "DC", zip: "20500" });
  const [pkg, setPkg] = useState("Letter");
  const [weight, setWeight] = useState("1");
  const [size, setSize] = useState("4x6");
  const [nonMach, setNonMach] = useState(false);
  const [ship, setShip] = useState<Shipment | null>(null);
  const [label, setLabel] = useState<Label | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function rates() {
    setBusy(true); setError(""); setShip(null); setLabel(null);
    const r = await send<Shipment>("POST", "/v1/admin/postage-test/rates", { from, to, package: pkg, weight_oz: Number(weight), label_size: size, non_machinable: nonMach });
    setBusy(false);
    if (!r.ok) return setError(r.message);
    setShip(r.data);
  }

  async function buy(rate: Rate) {
    if (!ship) return;
    setBusy(true); setError("");
    const r = await send<Label>("POST", "/v1/admin/postage-test/buy", { shipment_id: ship.id, rate_id: rate.id });
    setBusy(false);
    if (!r.ok) return setError(r.message);
    setLabel(r.data);
  }

  return (
    <div className="mt-5 space-y-4">
      <AddrFields title="From (your ship-from address)" v={from} set={setFrom} />
      <AddrFields title="To" v={to} set={setTo} />
      <div className="flex flex-wrap gap-4">
        <label className="text-[13px] font-semibold">Envelope
          <select className="mt-1 block rounded-lg border border-line px-3 py-2" value={pkg} onChange={(e) => setPkg(e.target.value)}>
            <option value="Letter">Letter (#10 and smaller, up to 1/4 in thick)</option>
            <option value="Flat">Flat (large envelope, up to 3/4 in thick)</option>
            <option value="Card">Postcard</option>
          </select>
        </label>
        <label className="text-[13px] font-semibold">Weight (oz)
          <input className="mt-1 block w-24 rounded-lg border border-line px-3 py-2" inputMode="decimal" value={weight} onChange={(e) => setWeight(e.target.value)} />
        </label>
        <label className="text-[13px] font-semibold">Label size
          <select className="mt-1 block rounded-lg border border-line px-3 py-2" value={size} onChange={(e) => setSize(e.target.value)}>
            <option value="4x6">4 x 6 in</option>
            <option value="7x3">7 x 3 in (letter)</option>
            <option value="6x4">6 x 4 in</option>
          </select>
        </label>
        <label className="flex items-center gap-2 self-end pb-2 text-[13px] font-semibold">
          <input type="checkbox" checked={nonMach} onChange={(e) => setNonMach(e.target.checked)} />
          Non-machinable (rigid, e.g. toploader)
        </label>
      </div>
      <p className="text-[12px] text-muted">Letter and postcard: up to 3.5 oz. Flat: up to 13 oz.</p>
      <button type="button" onClick={rates} disabled={busy} className="rounded-full bg-link px-6 py-2.5 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
        {busy ? "Asking EasyPost…" : "Get envelope rates"}
      </button>
      {error && <p role="alert" className="text-red-700">{error}</p>}

      {ship && (
        <div className="rounded-xl border border-line p-4">
          <p className="font-bold">Rates ({ship.rates.length})</p>
          {ship.rates.length === 0 && <p className="text-muted">No rates came back.</p>}
          <ul className="mt-2 divide-y divide-line">
            {ship.rates.map((r) => (
              <li key={r.id} className="flex flex-wrap items-center gap-3 py-2 text-[14px]">
                <span className="font-semibold">{r.carrier} {r.service}</span>
                <span>${r.rate}</span>
                {r.delivery_days ? <span className="text-muted">{r.delivery_days} days</span> : null}
                {canBuy && (
                  <button type="button" onClick={() => buy(r)} disabled={busy} className="ml-auto rounded-full border border-ink px-3 py-1 text-[13px] font-semibold hover:bg-ink hover:text-white">
                    Buy test label
                  </button>
                )}
              </li>
            ))}
          </ul>
          {ship.messages.length > 0 && (
            <div className="mt-3 text-[12px] text-muted">
              <p className="font-semibold">Carrier messages</p>
              {ship.messages.map((m, i) => <p key={i}>{m.carrier}: {m.message}</p>)}
            </div>
          )}
        </div>
      )}

      {label && (
        <div className="rounded-xl border-2 border-green-700 p-4">
          <p className="font-bold">Test label bought ({label.rate.carrier} {label.rate.service}, ${label.rate.rate})</p>
          <p className="text-[14px]">Tracking: {label.tracking_code || "none"} · size {label.label_size}</p>
          <a href={label.label_url} target="_blank" rel="noreferrer" className="mt-2 inline-block font-semibold text-link underline">Open the label</a>
        </div>
      )}
    </div>
  );
}
