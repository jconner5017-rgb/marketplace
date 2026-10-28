"use client";

import Link from "next/link";
import { useState } from "react";
import { send } from "@/lib/client";

type Thread = { id: string; listing_id: string | null; listing_title: string; buyer: string; buyer_id: string; seller: string; seller_id: string };
type Msg = { id: number; sender: string; role: string; body: string; blocked: string | null; at: string };

/** Shows a conversation only after the staff member gives a reason (which is logged). */
export default function ThreadLog({ id }: { id: string }) {
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [data, setData] = useState<{ thread: Thread; messages: Msg[] } | null>(null);

  async function open(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    const r = await send<{ thread: Thread; messages: Msg[] }>("POST", `/v1/admin/messages/${id}/read`, { reason });
    setBusy(false);
    if (!r.ok) return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    setData(r.data);
  }

  if (!data) {
    return (
      <form onSubmit={open} className="mt-4 max-w-[480px] space-y-3 text-[14px]">
        <p className="text-muted">Why are you reading this conversation? Your name, the time and this reason go in the audit log.</p>
        <input value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Case 5003, or report about off-site payment"
          className="w-full rounded-md border border-line px-3 py-2" />
        {error && <p role="alert" className="text-slab">{error}</p>}
        <button type="submit" disabled={busy} className="rounded-full bg-link px-5 py-2 font-semibold text-white disabled:opacity-60">
          {busy ? "Opening…" : "Open conversation"}
        </button>
      </form>
    );
  }
  const t = data.thread;
  return (
    <div className="mt-2">
      <p className="text-[14px] text-muted">
        About {t.listing_id ? <Link href={`/item/${t.listing_id}`} className="underline">{t.listing_title}</Link> : t.listing_title} ·{" "}
        <Link href={`/admin/users/${t.buyer_id}`} className="underline">{t.buyer}</Link> (buyer) and{" "}
        <Link href={`/admin/users/${t.seller_id}`} className="underline">{t.seller}</Link> (seller)
      </p>
      <ol className="mt-4 space-y-3">
        {data.messages.map((m) => (
          <li key={m.id} className={`flex ${m.role === "seller" ? "justify-end" : "justify-start"}`}>
            <div className={`max-w-[85%] rounded-2xl px-4 py-2.5 ${m.blocked ? "border-2 border-slab bg-slab/5" : m.role === "seller" ? "bg-[#e6f0ff]" : "bg-wash"}`}>
              <p className="text-[12px] font-semibold">{m.sender} · {m.role}{m.blocked && <span className="text-slab"> · blocked ({m.blocked.replace("_", " ")}), never delivered</span>}</p>
              <p className="whitespace-pre-line text-[15px]">{m.body}</p>
              <p className="mt-1 text-[11px] text-muted">{new Date(m.at).toLocaleString("en-US")}</p>
            </div>
          </li>
        ))}
      </ol>
      {data.messages.length === 0 && <p className="text-muted">No messages.</p>}
    </div>
  );
}
