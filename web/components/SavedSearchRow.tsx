"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

export type Saved = { id: string; name: string; query: string; notify: boolean; new_count: number; created_at: string };

export default function SavedSearchRow({ s }: { s: Saved }) {
  const router = useRouter();
  const [notify, setNotify] = useState(s.notify);
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const [error, setError] = useState("");
  const href = `/search?${s.query}&sort=newest&saved=${s.id}`;

  async function toggle() {
    setBusy(true);
    const r = await send("PATCH", `/v1/saved-searches/${s.id}`, { notify: !notify });
    setBusy(false);
    if (r.ok) setNotify(!notify);
    else setError(r.message);
  }
  async function remove() {
    setBusy(true);
    const r = await send("DELETE", `/v1/saved-searches/${s.id}`);
    setBusy(false);
    if (r.ok) router.refresh();
    else setError(r.message);
  }

  return (
    <li className="flex flex-wrap items-center gap-x-4 gap-y-2 py-4 text-[14px]">
      <div className="min-w-0 flex-1">
        <Link href={href} className="text-[16px] font-semibold text-link hover:underline">{s.name}</Link>
        {s.new_count > 0 && (
          <span className="ml-2 rounded-full bg-ship/10 px-2 py-0.5 text-[12px] font-semibold text-ship">{s.new_count} new</span>
        )}
        <p className="mt-0.5 truncate text-[12px] text-muted">{Array.from(new URLSearchParams(s.query)).map(([k, v]) => `${k}: ${v}`).join(" · ")}</p>
      </div>
      <label className="flex items-center gap-2">
        <input type="checkbox" checked={notify} onChange={toggle} disabled={busy} /> Notify me
      </label>
      {confirm ? (
        <span className="flex gap-2">
          <button type="button" onClick={remove} disabled={busy} className="font-semibold text-slab underline">Delete</button>
          <button type="button" onClick={() => setConfirm(false)} className="text-muted underline">Keep</button>
        </span>
      ) : (
        <button type="button" onClick={() => setConfirm(true)} className="text-muted underline">Delete</button>
      )}
      {error && <p role="alert" className="w-full text-slab">{error}</p>}
    </li>
  );
}
