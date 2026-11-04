"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { send } from "@/lib/client";

/** Follow / Following toggle for a store. Signed-out visitors go to sign in. */
export default function FollowStoreButton({ slug, following, followers, signedIn }: { slug: string; following: boolean; followers: number; signedIn: boolean }) {
  const router = useRouter();
  const [on, setOn] = useState(following);
  const [count, setCount] = useState(followers);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  async function toggle() {
    if (!signedIn) {
      router.push(`/signin?next=${encodeURIComponent(`/store/${slug}`)}`);
      return;
    }
    setBusy(true);
    setErr("");
    const r = await send<{ following: boolean; followers: number }>(on ? "DELETE" : "POST", `/v1/stores/${encodeURIComponent(slug)}/follow`);
    setBusy(false);
    if (!r.ok) return setErr(r.message);
    setOn(r.data.following);
    setCount(r.data.followers);
  }
  return (
    <div className="flex flex-col items-start gap-1">
      <button onClick={toggle} disabled={busy} aria-pressed={on}
        className={`rounded-full px-5 py-2 text-[14px] font-semibold ${on ? "border border-ink bg-paper hover:bg-wash" : "bg-ink text-white hover:opacity-90"} disabled:opacity-60`}>
        {on ? "✓ Following" : "Follow store"}
      </button>
      <span className="num text-[13px] text-muted">{count.toLocaleString()} {count === 1 ? "follower" : "followers"}</span>
      {err && <span role="alert" className="text-[13px] text-slab">{err}</span>}
    </div>
  );
}
