"use client";

import { useState } from "react";
import { send } from "@/lib/client";

/** Recomputes the audit log's hash chain on the server. */
export default function VerifyChain() {
  const [state, setState] = useState<{ ok: boolean; checked: number; broken_at?: number } | null>(null);
  const [busy, setBusy] = useState(false);
  async function run() {
    setBusy(true);
    const r = await send<{ ok: boolean; checked: number; broken_at?: number }>("GET", "/v1/admin/audit/verify");
    setBusy(false);
    if (r.ok) setState(r.data);
  }
  return (
    <div className="text-right text-[14px]">
      <button type="button" onClick={run} disabled={busy} className="rounded-full border border-ink px-4 py-1.5 font-semibold hover:bg-wash disabled:opacity-60">
        {busy ? "Checking…" : "Verify the log"}
      </button>
      {state && (
        <p role="status" className={`mt-1 ${state.ok ? "text-ship" : "font-semibold text-slab"}`}>
          {state.ok ? `All ${state.checked} entries check out.` : `Entry ${state.broken_at} was changed or removed.`}
        </p>
      )}
    </div>
  );
}
