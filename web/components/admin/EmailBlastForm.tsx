"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

export default function EmailBlastForm({ audiences }: { audiences: Record<string, number> }) {
  const router = useRouter();
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [audience, setAudience] = useState("all");
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const count = audiences[audience] ?? 0;

  async function go(test: boolean) {
    setBusy(true);
    setMsg(null);
    const r = await send<{ to?: string; recipients?: number }>("POST", "/v1/admin/email", { subject, body, audience, test });
    setBusy(false);
    setConfirm(false);
    if (!r.ok) return setMsg({ ok: false, text: r.message.charAt(0).toUpperCase() + r.message.slice(1) + "." });
    if (test) return setMsg({ ok: true, text: `Test sent to ${r.data.to}. Check how it looks before sending to everyone.` });
    setMsg({ ok: true, text: `Sending to ${r.data.recipients} members. It goes out over the next few minutes.` });
    setSubject("");
    setBody("");
    router.refresh();
  }

  const input = "mt-1 w-full rounded-md border border-line px-3 py-2 text-[15px] focus:border-ink";
  return (
    <div className="mt-6 space-y-4">
      <div>
        <label htmlFor="audience" className="text-[14px] font-semibold">Send to</label>
        <select id="audience" value={audience} onChange={(e) => setAudience(e.target.value)} className={input}>
          <option value="all">Everyone ({audiences.all ?? 0})</option>
          <option value="sellers">Sellers who can take payments ({audiences.sellers ?? 0})</option>
        </select>
      </div>
      <div>
        <label htmlFor="subject" className="text-[14px] font-semibold">Subject</label>
        <input id="subject" value={subject} maxLength={150} onChange={(e) => setSubject(e.target.value)} className={input} />
      </div>
      <div>
        <label htmlFor="body" className="text-[14px] font-semibold">Message</label>
        <textarea id="body" value={body} rows={12} onChange={(e) => setBody(e.target.value)} className={input}
          placeholder={"Plain text. Links like https://slabsnsingles.com/help work as-is.\n\nThe unsubscribe link and mailing address are added at the bottom."} />
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <button type="button" disabled={busy || !subject || !body} onClick={() => go(true)}
          className="rounded-full border border-link px-4 py-2 font-semibold text-link disabled:opacity-50">
          Send me a test
        </button>
        {confirm ? (
          <>
            <span className="text-[14px]">Send to {count} members? This can&apos;t be undone.</span>
            <button type="button" disabled={busy} onClick={() => go(false)} className="rounded-full bg-slab px-4 py-2 font-semibold text-white disabled:opacity-50">
              Yes, send it
            </button>
            <button type="button" onClick={() => setConfirm(false)} className="text-[14px] text-link underline">Cancel</button>
          </>
        ) : (
          <button type="button" disabled={busy || !subject || !body || count === 0} onClick={() => setConfirm(true)}
            className="rounded-full bg-link px-4 py-2 font-semibold text-white disabled:opacity-50">
            Send to {count} members…
          </button>
        )}
      </div>
      {msg && <p role="status" className={msg.ok ? "text-ship" : "text-slab"}>{msg.text}</p>}
    </div>
  );
}
