"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

function Secret({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="mt-3 rounded-lg border border-ink bg-wash p-3 text-[14px]">
      <p className="font-semibold">{label}</p>
      <p className="text-[13px] text-muted">Copy it now; it won&apos;t be shown again.</p>
      <div className="mt-2 flex flex-wrap items-center gap-2">
        <code className="min-w-0 flex-1 break-all rounded bg-white px-2 py-1.5 text-[13px]">{value}</code>
        <button type="button" onClick={() => { navigator.clipboard?.writeText(value); setCopied(true); }}
          className="rounded-full border border-link px-3 py-1 text-[13px] font-semibold text-link">{copied ? "Copied" : "Copy"}</button>
      </div>
    </div>
  );
}

export function NewKeyForm() {
  const router = useRouter();
  const [name, setName] = useState("FireCardManager");
  const [key, setKey] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function go(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    const r = await send<{ api_key: string }>("POST", "/v1/seller/api-keys", { name });
    setBusy(false);
    if (!r.ok) return setError(r.message);
    setKey(r.data.api_key);
    router.refresh();
  }
  return (
    <div>
      <form onSubmit={go} className="flex flex-wrap items-end gap-2 text-[14px]">
        <label className="font-semibold">Key name
          <input value={name} onChange={(e) => setName(e.target.value)} maxLength={60}
            className="mt-1 block rounded-md border border-line px-3 py-1.5 font-normal" />
        </label>
        <button type="submit" disabled={busy} className="rounded-full bg-link px-4 py-2 font-semibold text-white hover:bg-link-dark disabled:opacity-60">Create API key</button>
      </form>
      {error && <p role="alert" className="mt-2 text-[13px] text-slab">{error}</p>}
      {key && <Secret label="Your new API key" value={key} />}
    </div>
  );
}

export function RevokeKey({ id }: { id: string }) {
  const router = useRouter();
  const [confirm, setConfirm] = useState(false);
  async function go() {
    await send("DELETE", `/v1/seller/api-keys/${id}`);
    router.refresh();
  }
  return confirm ? (
    <span className="text-[13px]">
      <button type="button" onClick={go} className="font-semibold text-slab underline">Yes, revoke</button>{" "}
      <button type="button" onClick={() => setConfirm(false)} className="text-muted underline">Keep</button>
    </span>
  ) : (
    <button type="button" onClick={() => setConfirm(true)} className="text-[13px] text-muted underline hover:text-slab">Revoke</button>
  );
}

export function EndpointForm({ webhook, inventory, hasSecret }: { webhook: string; inventory: string; hasSecret: boolean }) {
  const router = useRouter();
  const [wh, setWh] = useState(webhook);
  const [inv, setInv] = useState(inventory);
  const [secret, setSecret] = useState("");
  const [msg, setMsg] = useState("");
  const [fields, setFields] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  async function save(rotate: boolean) {
    setBusy(true);
    setMsg("");
    setFields({});
    const r = await send<{ signing_secret?: string }>("PUT", "/v1/seller/connect", { webhook_url: wh, inventory_url: inv, rotate_secret: rotate });
    setBusy(false);
    if (!r.ok) {
      setFields(r.fields);
      return setMsg(r.message);
    }
    if (r.data.signing_secret) setSecret(r.data.signing_secret);
    setMsg("Saved.");
    router.refresh();
  }
  async function test() {
    const r = await send("POST", "/v1/seller/connect/test", {});
    setMsg(r.ok ? "Test event queued; it arrives within a minute." : r.message);
    router.refresh();
  }
  const input = "mt-1 block w-full rounded-md border border-line px-3 py-2 font-normal";
  return (
    <form onSubmit={(e) => { e.preventDefault(); save(false); }} className="space-y-3 text-[14px]">
      <label className="block font-semibold">Webhook URL (order.paid and ping events)
        <input value={wh} onChange={(e) => setWh(e.target.value)} placeholder="https://…" className={input} />
        {fields.webhook_url && <span className="mt-1 block text-[13px] font-normal text-slab">{fields.webhook_url}</span>}
      </label>
      <label className="block font-semibold">Inventory URL (checked nightly)
        <input value={inv} onChange={(e) => setInv(e.target.value)} placeholder="https://…" className={input} />
        {fields.inventory_url && <span className="mt-1 block text-[13px] font-normal text-slab">{fields.inventory_url}</span>}
      </label>
      <div className="flex flex-wrap gap-2">
        <button type="submit" disabled={busy} className="rounded-full bg-link px-4 py-2 font-semibold text-white hover:bg-link-dark disabled:opacity-60">Save</button>
        {hasSecret && <button type="button" onClick={() => save(true)} disabled={busy} className="rounded-full border border-link px-4 py-2 font-semibold text-link">New signing secret</button>}
        {webhook && <button type="button" onClick={test} className="rounded-full border border-link px-4 py-2 font-semibold text-link">Send test event</button>}
      </div>
      {msg && <p role="status" className="text-[13px]">{msg}</p>}
      {secret && <Secret label="Webhook signing secret" value={secret} />}
    </form>
  );
}
