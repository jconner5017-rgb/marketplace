"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

export default function WatchRowActions({ id, available, inCart }: { id: string; available: boolean; inCart: boolean }) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function run(method: string, path: string, body?: object) {
    setBusy(true);
    setError("");
    const r = await send(method, path, body);
    setBusy(false);
    if (!r.ok) return setError(r.message);
    router.refresh();
  }
  return (
    <div className="flex w-36 shrink-0 flex-col gap-2 text-[14px]">
      {available && (inCart ? (
        <Link href="/cart" className="rounded-full border border-link py-1.5 text-center font-semibold text-link hover:bg-wash">In cart</Link>
      ) : (
        <button type="button" disabled={busy} onClick={() => run("PUT", `/v1/cart/${id}`, { quantity: 1 })}
          className="rounded-full bg-link py-1.5 font-semibold text-white hover:bg-link-dark disabled:opacity-60">Add to cart</button>
      ))}
      <button type="button" disabled={busy} onClick={() => run("DELETE", `/v1/watchlist/${id}`)}
        className="rounded-full border border-line py-1.5 hover:border-ink disabled:opacity-60">Remove</button>
      {error && <p role="alert" className="text-[12px] text-slab">{error}</p>}
    </div>
  );
}
