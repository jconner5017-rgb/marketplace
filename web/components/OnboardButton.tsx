"use client";

import { useState } from "react";
import { send } from "@/lib/client";

/** Asks the API for a Stripe onboarding link and goes there. */
export default function OnboardButton({ label }: { label: string }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function go() {
    setBusy(true);
    setError("");
    const res = await send<{ url: string }>("POST", "/v1/seller/onboarding", {});
    if (res.ok) {
      window.location.assign(res.data.url);
      return;
    }
    setBusy(false);
    setError(res.status === 503 ? "Selling isn't switched on yet on this server." : res.message);
  }
  return (
    <div>
      <button type="button" onClick={go} disabled={busy} className="rounded-full bg-link px-6 py-2.5 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
        {busy ? "Opening Stripe…" : label}
      </button>
      {error && <p role="alert" className="mt-2 text-[14px] text-slab">{error}</p>}
    </div>
  );
}
