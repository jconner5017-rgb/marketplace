"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

const CARRIERS = ["USPS", "UPS", "FedEx", "DHL"];

/** Seller marks an order shipped, with tracking (required for label orders). */
export default function ShipForm({ orderId, method }: { orderId: string; method: "envelope" | "label" }) {
  const router = useRouter();
  const [carrier, setCarrier] = useState("USPS");
  const [tracking, setTracking] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function ship(withTracking: boolean) {
    setBusy(true);
    setError("");
    const r = await send("POST", `/v1/orders/${orderId}/ship`, withTracking ? { carrier, tracking_number: tracking } : { carrier: "", tracking_number: "" });
    setBusy(false);
    if (!r.ok) return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    router.refresh();
  }

  return (
    <form onSubmit={(e) => { e.preventDefault(); ship(true); }} className="mt-3 space-y-2">
      <div className="flex flex-wrap gap-2">
        <label className="sr-only" htmlFor={`car-${orderId}`}>Carrier</label>
        <select id={`car-${orderId}`} value={carrier} onChange={(e) => setCarrier(e.target.value)}
          className="rounded-md border border-line bg-white px-2 py-2 text-[14px]">
          {CARRIERS.map((c) => <option key={c}>{c}</option>)}
        </select>
        <label className="sr-only" htmlFor={`trk-${orderId}`}>Tracking number</label>
        <input id={`trk-${orderId}`} value={tracking} onChange={(e) => setTracking(e.target.value)} placeholder="Tracking number"
          autoComplete="off" className="num min-w-0 flex-1 rounded-md border border-line px-3 py-2 text-[14px] outline-none focus:border-ink" />
        <button type="submit" disabled={busy || tracking.trim() === ""}
          className="rounded-full bg-link px-4 py-2 text-[14px] font-semibold text-white hover:bg-link-dark disabled:opacity-50">
          Mark shipped
        </button>
      </div>
      {method === "envelope" && (
        <button type="button" onClick={() => ship(false)} disabled={busy} className="text-[14px] font-semibold text-link hover:underline">
          Sent in a stamped envelope without tracking
        </button>
      )}
      {error && <p role="alert" className="text-[13px] text-slab">{error}</p>}
    </form>
  );
}
