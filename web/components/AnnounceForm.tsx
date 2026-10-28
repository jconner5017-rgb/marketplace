"use client";

import { useState } from "react";
import { send } from "@/lib/client";
import Field from "./Field";

export default function AnnounceForm() {
  const [msg, setMsg] = useState("");
  const [busy, setBusy] = useState(false);
  async function submit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const f = new FormData(form);
    setBusy(true);
    const r = await send("POST", "/v1/admin/announcements", { title: f.get("title"), body: f.get("body"), link: f.get("link") });
    setBusy(false);
    if (!r.ok) return setMsg(r.message);
    form.reset();
    setMsg("Posted. Members will see it in their bell.");
  }
  return (
    <form onSubmit={submit} className="mt-6 space-y-4">
      <Field label="Title" name="title" maxLength={120} required placeholder="Holiday shipping cutoff is Dec 18" />
      <Field label="Details (optional)" name="body">
        <textarea id="body" name="body" rows={4} maxLength={1000} className="mt-1 w-full rounded-md border border-line px-3 py-2" />
      </Field>
      <Field label="Link (optional)" name="link" placeholder="/help/shipping" hint="A page on this site (starting with /) or an https:// link." />
      <button type="submit" disabled={busy} className="rounded-full bg-link px-6 py-2.5 font-semibold text-white disabled:opacity-60">
        {busy ? "Posting…" : "Post announcement"}
      </button>
      {msg && <p role="status" className="text-[14px]">{msg}</p>}
    </form>
  );
}
