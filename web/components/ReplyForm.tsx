"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

export default function ReplyForm({ threadId }: { threadId: string }) {
  const router = useRouter();
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    const r = await send("POST", `/v1/messages/${threadId}`, { body });
    setBusy(false);
    if (!r.ok) return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    setBody("");
    router.refresh();
  }
  return (
    <form onSubmit={submit} className="sticky bottom-0 border-t border-line bg-paper py-3">
      <label htmlFor="reply" className="sr-only">Reply</label>
      <textarea id="reply" value={body} onChange={(e) => setBody(e.target.value)} rows={3} maxLength={2000}
        placeholder="Write a reply" className="w-full rounded-md border border-line px-3 py-2 text-[15px]" />
      <div className="mt-2 flex items-center justify-between gap-3">
        <p className="text-[12px] text-muted">Keep payment on Slabs &amp; Singles. Contact details and payment apps are blocked.</p>
        <button type="submit" disabled={busy || !body.trim()} className="shrink-0 rounded-full bg-link px-5 py-2 font-semibold text-white disabled:opacity-50">
          {busy ? "Sending…" : "Send"}
        </button>
      </div>
      {error && <p role="alert" className="mt-1 text-[13px] text-slab">{error}</p>}
    </form>
  );
}
