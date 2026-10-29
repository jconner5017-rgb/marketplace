"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { send } from "@/lib/client";

type Addr = { name: string; line1: string; line2?: string; city: string; state: string; zip: string; phone?: string };

export function ShipFromForm({ initial }: { initial: Addr | null }) {
  const router = useRouter();
  const [a, setA] = useState<Addr>(initial ?? { name: "", line1: "", line2: "", city: "", state: "", zip: "", phone: "" });
  const [msg, setMsg] = useState("");
  const [busy, setBusy] = useState(false);
  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setMsg("");
    const r = await send("PUT", "/v1/seller/shipping/from", a);
    setBusy(false);
    setMsg(r.ok ? "Saved and verified." : r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    if (r.ok) router.refresh();
  }
  const f = (k: keyof Addr, label: string, cls = "", extra: React.InputHTMLAttributes<HTMLInputElement> = {}) => (
    <label className={`block text-[14px] font-semibold ${cls}`}>{label}
      <input value={a[k] ?? ""} onChange={(e) => setA({ ...a, [k]: e.target.value })} {...extra}
        className="mt-1 block w-full rounded-md border border-line px-3 py-2 font-normal" />
    </label>
  );
  return (
    <form onSubmit={save} className="grid gap-3 sm:grid-cols-6">
      {f("name", "Name or business", "sm:col-span-6", { required: true, autoComplete: "name" })}
      {f("line1", "Street address", "sm:col-span-6", { required: true, autoComplete: "address-line1" })}
      {f("line2", "Apt, suite (optional)", "sm:col-span-6", { autoComplete: "address-line2" })}
      {f("city", "City", "sm:col-span-3", { required: true, autoComplete: "address-level2" })}
      {f("state", "State", "sm:col-span-1", { required: true, maxLength: 2, autoComplete: "address-level1" })}
      {f("zip", "ZIP", "sm:col-span-2", { required: true, inputMode: "numeric", autoComplete: "postal-code" })}
      {f("phone", "Phone (carriers may need it)", "sm:col-span-3", { autoComplete: "tel" })}
      <div className="sm:col-span-6">
        <button type="submit" disabled={busy} className="rounded-full bg-link px-5 py-2 font-semibold text-white hover:bg-link-dark disabled:opacity-60">{busy ? "Checking…" : "Save address"}</button>
        {msg && <span role="status" className="ml-3 text-[14px]">{msg}</span>}
      </div>
    </form>
  );
}

export function CardButton({ hasCard }: { hasCard: boolean }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function go() {
    setBusy(true);
    const r = await send<{ url: string }>("POST", "/v1/seller/shipping/card", {});
    if (!r.ok) {
      setBusy(false);
      return setError(r.message);
    }
    window.location.assign(r.data.url);
  }
  return (
    <div>
      <button type="button" onClick={go} disabled={busy} className="rounded-full border border-link px-5 py-2 font-semibold text-link hover:bg-wash disabled:opacity-60">
        {busy ? "Opening Stripe…" : hasCard ? "Replace card" : "Add a card"}
      </button>
      {error && <p role="alert" className="mt-1 text-[13px] text-slab">{error}</p>}
    </div>
  );
}

/** Back from Stripe's card page: record the saved card once. */
export function FinishCard({ session }: { session: string }) {
  const router = useRouter();
  const [msg, setMsg] = useState("Saving your card…");
  const done = useRef(false);
  useEffect(() => {
    if (done.current) return;
    done.current = true;
    send<{ card: string }>("POST", "/v1/seller/shipping/card/finish", { session_id: session }).then((r) => {
      setMsg(r.ok ? `Saved: ${r.data.card}.` : r.message);
      router.replace("/sell/shipping");
      router.refresh();
    });
  }, [session, router]);
  return <p role="status" className="mt-2 text-[14px]">{msg}</p>;
}
