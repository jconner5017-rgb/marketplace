"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

/** Two-step delete (or cancel) button for small admin records. */
export default function DeleteButton({ endpoint, label = "Delete" }: { endpoint: string; label?: string }) {
  const router = useRouter();
  const [confirm, setConfirm] = useState(false);
  const [error, setError] = useState("");
  async function go() {
    const r = await send("DELETE", endpoint);
    if (!r.ok) {
      setConfirm(false);
      return setError(r.message);
    }
    router.refresh();
  }
  return (
    <span className="text-[13px]">
      {confirm ? (
        <>
          <button type="button" onClick={go} className="font-semibold text-slab underline">Yes, {label.toLowerCase()}</button>{" "}
          <button type="button" onClick={() => setConfirm(false)} className="text-muted underline">Keep</button>
        </>
      ) : (
        <button type="button" onClick={() => setConfirm(true)} className="text-muted underline hover:text-slab">{label}</button>
      )}
      {error && <span role="alert" className="block text-slab">{error}</span>}
    </span>
  );
}
