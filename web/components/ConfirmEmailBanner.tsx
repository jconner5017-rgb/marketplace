"use client";

import { useState } from "react";
import { send } from "@/lib/client";

/** Shown to new members until they confirm their email. */
export default function ConfirmEmailBanner({ email }: { email: string }) {
  const [state, setState] = useState<"idle" | "busy" | "sent" | "error">("idle");
  const [msg, setMsg] = useState("");
  async function resend() {
    setState("busy");
    const r = await send("POST", "/v1/me/confirm-email");
    if (r.ok) return setState("sent");
    setMsg(r.message);
    setState("error");
  }
  return (
    <div role="status" className="bg-[#fff6dc] text-[14px]">
      <div className="mx-auto flex max-w-[1280px] flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2">
        <span>
          <strong>Confirm your email to buy and sell.</strong> We sent a link to {email}.
        </span>
        {state === "sent" ? (
          <span className="text-ship">New link sent. Check your inbox and spam folder.</span>
        ) : (
          <button type="button" onClick={resend} disabled={state === "busy"} className="font-semibold text-link underline disabled:opacity-60">
            {state === "busy" ? "Sending…" : "Send a new link"}
          </button>
        )}
        {state === "error" && <span className="text-slab">{msg}</span>}
      </div>
    </div>
  );
}
