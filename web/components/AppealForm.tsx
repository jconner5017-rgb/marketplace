"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

/**
 * One appeal against a decision. Signed-in members pass about/ref; the
 * public form (suspended or banned members) also asks for email and password.
 */
export default function AppealForm({ about, refId, account }: { about?: string; refId?: string; account?: boolean }) {
  const router = useRouter();
  const [open, setOpen] = useState(Boolean(account));
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);

  async function go(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    const r = account
      ? await send("POST", "/v1/appeals/account", { email, password, message })
      : await send("POST", "/v1/me/appeals", { about, ref: refId ?? "", message });
    setBusy(false);
    if (!r.ok) return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    setDone(true);
    if (!account) router.refresh();
  }

  if (done) return <p role="status" className="mt-2 text-[14px] text-ship">Appeal sent. Our team reviews appeals within a few days and you&apos;ll be notified of the decision.</p>;
  if (!open) {
    return <button type="button" onClick={() => setOpen(true)} className="mt-2 text-[14px] font-semibold text-link hover:underline">Appeal</button>;
  }
  const input = "mt-1 w-full rounded-md border border-line px-3 py-2 text-[15px] outline-none focus:border-ink";
  return (
    <form onSubmit={go} className="mt-3 space-y-3">
      {account && (
        <>
          <label className="block text-[14px] font-semibold">Email
            <input type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} className={`${input} font-normal`} />
          </label>
          <label className="block text-[14px] font-semibold">Password
            <input type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} className={`${input} font-normal`} />
          </label>
        </>
      )}
      <label className="block text-[14px] font-semibold">Why should we change this decision?
        <textarea required minLength={20} maxLength={3000} rows={5} value={message} onChange={(e) => setMessage(e.target.value)} className={`${input} font-normal`} />
      </label>
      <p className="text-[13px] text-muted">You can appeal each decision once, so include everything that helps: order numbers, tracking, what happened.</p>
      {error && <p role="alert" className="text-[13px] text-slab">{error}</p>}
      <button type="submit" disabled={busy} className="rounded-full bg-link px-5 py-2 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
        {busy ? "Sending…" : "Send appeal"}
      </button>
    </form>
  );
}
