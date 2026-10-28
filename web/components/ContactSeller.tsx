"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

export default function ContactSeller({ id, seller, signedIn }: { id: string; seller: string; signedIn: boolean }) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  if (!signedIn) {
    return (
      <Link href={`/signin?next=${encodeURIComponent(`/item/${id}`)}`} className="mt-2 inline-block text-[14px] text-link underline">
        Contact seller
      </Link>
    );
  }
  if (!open) {
    return (
      <button type="button" onClick={() => setOpen(true)} className="mt-2 text-[14px] text-link underline">
        Contact seller
      </button>
    );
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    const r = await send<{ thread_id: string }>("POST", `/v1/listings/${id}/messages`, { body });
    setBusy(false);
    if (!r.ok) return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    router.push(`/messages/${r.data.thread_id}`);
  }

  return (
    <form onSubmit={submit} className="mt-3">
      <label htmlFor="ask" className="block text-[14px] font-semibold">Ask {seller} a question</label>
      <textarea id="ask" value={body} onChange={(e) => setBody(e.target.value)} rows={3} maxLength={2000} required
        placeholder="Condition, centering, combined orders…"
        className="mt-1 w-full rounded-md border border-line px-3 py-2 text-[14px]" />
      <p className="mt-1 text-[12px] text-muted">
        Keep payment on Slabs &amp; Singles. Messages with emails, phone numbers or payment apps aren&apos;t sent.
      </p>
      {error && <p role="alert" className="mt-1 text-[13px] text-slab">{error}</p>}
      <div className="mt-2 flex gap-2">
        <button type="submit" disabled={busy || !body.trim()} className="rounded-full bg-link px-5 py-2 text-[14px] font-semibold text-white disabled:opacity-50">
          {busy ? "Sending…" : "Send message"}
        </button>
        <button type="button" onClick={() => setOpen(false)} className="rounded-full px-4 py-2 text-[14px] text-link">Cancel</button>
      </div>
    </form>
  );
}
