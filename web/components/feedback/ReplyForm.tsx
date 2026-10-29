"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

/** A seller's one public reply (300 characters) under neutral or negative feedback. */
export default function ReplyForm({ id }: { id: string }) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  if (!open) return <button type="button" onClick={() => setOpen(true)} className="mt-1 text-[13px] font-semibold text-link hover:underline">Reply publicly</button>;
  async function go(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    const r = await send("POST", `/v1/feedback/${id}/reply`, { reply: text });
    setBusy(false);
    if (!r.ok) return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    router.refresh();
  }
  return (
    <form onSubmit={go} className="mt-2 text-[14px]">
      <label className="block">
        <span className="font-semibold">Your reply</span>
        <textarea value={text} onChange={(e) => setText(e.target.value)} maxLength={300} rows={2} required
          className="mt-1 w-full rounded-md border border-line px-3 py-2 outline-none focus:border-ink" />
      </label>
      <p className="text-[12px] text-muted">{300 - text.length} characters left. Keep it calm and factual; you can reply once.</p>
      {error && <p role="alert" className="text-[13px] text-slab">{error}</p>}
      <button type="submit" disabled={busy} className="mt-1 rounded-full bg-link px-4 py-1.5 font-semibold text-white disabled:opacity-60">Post reply</button>
    </form>
  );
}
