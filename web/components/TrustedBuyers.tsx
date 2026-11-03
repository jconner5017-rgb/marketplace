"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

export type Trusted = { username: string; via: "list" | "invite" | "order"; added_at: string };

const VIA = { list: "Added by you", invite: "Joined by invite link", order: "Added on an order" };

/** Seller: the trusted buyer list and invite link. */
export default function TrustedBuyers({ buyers, invite, site }: { buyers: Trusted[]; invite: string; site: string }) {
  const router = useRouter();
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState(false);
  const link = invite ? `${site}/trust/${invite}` : "";

  async function call(method: string, path: string, body?: unknown) {
    setBusy(true);
    setError("");
    const r = await send(method, path, body);
    setBusy(false);
    if (!r.ok) {
      setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
      return false;
    }
    router.refresh();
    return true;
  }

  return (
    <div className="space-y-6">
      <section className="rounded-lg border border-line p-4 text-[14px]">
        <h2 className="font-bold">Invite link</h2>
        <p className="mt-1 text-muted">Text this to friends. When they open it and accept, they join your list, even if their account is brand new.</p>
        {link ? (
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <input readOnly value={link} className="min-w-0 flex-1 rounded-md border border-line px-3 py-1.5 font-mono text-[13px]" aria-label="Invite link" />
            <button type="button" onClick={() => { navigator.clipboard?.writeText(link); setCopied(true); }}
              className="rounded-full bg-link px-4 py-1.5 font-semibold text-white hover:bg-link-dark">{copied ? "Copied" : "Copy"}</button>
            <button type="button" disabled={busy} onClick={() => { setCopied(false); call("POST", "/v1/seller/trusted/invite", {}); }}
              className="rounded-full border border-link px-4 py-1.5 font-semibold text-link hover:bg-wash disabled:opacity-60">New link</button>
          </div>
        ) : (
          <p className="mt-2 text-muted">Finish setting up selling to get your invite link.</p>
        )}
        <p className="mt-2 text-[12px] text-muted">New link makes the old one stop working, if it was passed around. People already on your list stay on it.</p>
      </section>

      <section className="rounded-lg border border-line p-4 text-[14px]">
        <h2 className="font-bold">Your trusted buyers ({buyers.length})</h2>
        <form onSubmit={async (e) => { e.preventDefault(); if (await call("POST", "/v1/seller/trusted", { username: name })) setName(""); }}
          className="mt-3 flex flex-wrap gap-2">
          <label htmlFor="trust-name" className="sr-only">Username</label>
          <input id="trust-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Their username"
            className="w-[220px] rounded-md border border-line px-3 py-1.5" />
          <button type="submit" disabled={busy || !name.trim()} className="rounded-full bg-link px-4 py-1.5 font-semibold text-white hover:bg-link-dark disabled:opacity-60">Add</button>
        </form>
        {error && <p role="alert" className="mt-2 text-[13px] text-slab">{error}</p>}
        {buyers.length === 0 ? (
          <p className="mt-3 text-muted">Nobody yet.</p>
        ) : (
          <ul className="mt-3 divide-y divide-wash">
            {buyers.map((b) => (
              <li key={b.username} className="flex items-center justify-between gap-3 py-2">
                <span><span className="font-semibold">{b.username}</span> <span className="text-[12px] text-muted">· {VIA[b.via]} · {new Date(b.added_at).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" })}</span></span>
                <button type="button" disabled={busy} onClick={() => call("DELETE", `/v1/seller/trusted/${encodeURIComponent(b.username)}`)}
                  className="text-[13px] text-link underline disabled:opacity-60">Remove</button>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
