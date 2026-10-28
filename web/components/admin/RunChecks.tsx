"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

export default function RunChecks() {
  const router = useRouter();
  const [msg, setMsg] = useState("");
  const [busy, setBusy] = useState(false);
  async function run() {
    setBusy(true);
    const r = await send<{ new_alerts: number }>("POST", "/v1/admin/fraud/run");
    setBusy(false);
    setMsg(r.ok ? `${r.data.new_alerts} new ${r.data.new_alerts === 1 ? "alert" : "alerts"}.` : r.message);
    router.refresh();
  }
  return (
    <div className="text-right text-[14px]">
      <button type="button" onClick={run} disabled={busy} className="rounded-full border border-ink px-4 py-1.5 font-semibold hover:bg-wash disabled:opacity-60">
        {busy ? "Checking…" : "Run checks now"}
      </button>
      {msg && <p role="status" className="mt-1 text-muted">{msg}</p>}
    </div>
  );
}
