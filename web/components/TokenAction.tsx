"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState, type FormEvent } from "react";
import { send } from "@/lib/client";

/** Confirm-email and unsubscribe pages: one button that spends the link. */
export function ConfirmEmail() {
  const token = useSearchParams().get("token") ?? "";
  const router = useRouter();
  const [state, setState] = useState<"idle" | "busy" | "done" | "error">("idle");
  const [msg, setMsg] = useState("");
  async function go() {
    setState("busy");
    const r = await send("POST", "/v1/auth/confirm-email", { token });
    if (!r.ok) {
      setMsg(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
      return setState("error");
    }
    setState("done");
    router.refresh();
  }
  if (!token) return <p className="mt-3 text-muted">This link is missing its code. Open the link from the email again.</p>;
  if (state === "done") {
    return (
      <div className="mt-3">
        <p className="text-ship">Your email is confirmed. You&apos;re all set to buy and sell.</p>
        <Link href="/" className="mt-4 inline-block rounded-full bg-link px-5 py-2 font-semibold text-white">Start browsing</Link>
      </div>
    );
  }
  return (
    <div className="mt-3">
      <button type="button" onClick={go} disabled={state === "busy"} className="rounded-full bg-link px-5 py-2 font-semibold text-white disabled:opacity-60">
        {state === "busy" ? "Confirming…" : "Confirm my email"}
      </button>
      {state === "error" && <p role="alert" className="mt-3 text-slab">{msg}</p>}
    </div>
  );
}

export function Unsubscribe() {
  const q = useSearchParams();
  const [state, setState] = useState<"idle" | "busy" | "done" | "error">("idle");
  async function go() {
    setState("busy");
    const r = await send("POST", `/v1/email/unsubscribe?u=${encodeURIComponent(q.get("u") ?? "")}&t=${encodeURIComponent(q.get("t") ?? "")}`);
    setState(r.ok ? "done" : "error");
  }
  if (state === "done") return <p className="mt-3 text-ship">You&apos;re unsubscribed from announcement emails. Account emails, like password resets, still come through.</p>;
  return (
    <div className="mt-3">
      <button type="button" onClick={go} disabled={state === "busy"} className="rounded-full bg-link px-5 py-2 font-semibold text-white disabled:opacity-60">
        {state === "busy" ? "Unsubscribing…" : "Unsubscribe"}
      </button>
      {state === "error" && <p role="alert" className="mt-3 text-slab">That link isn&apos;t valid. Sign in and turn off announcements under Email settings instead.</p>}
    </div>
  );
}

export function ForgotPassword() {
  const [state, setState] = useState<"idle" | "busy" | "done" | "error">("idle");
  const [msg, setMsg] = useState("");
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setState("busy");
    const r = await send("POST", "/v1/auth/forgot", { email: new FormData(e.currentTarget).get("email") });
    if (!r.ok) {
      setMsg(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
      return setState("error");
    }
    setState("done");
  }
  if (state === "done") {
    return <p className="mt-4 text-ship">If that email has an account, a reset link is on its way. It works for one hour; check your spam folder too.</p>;
  }
  return (
    <form onSubmit={submit} className="mt-4 space-y-3">
      <label htmlFor="email" className="block text-[14px] font-semibold">Email</label>
      <input id="email" name="email" type="email" required autoComplete="email" className="w-full rounded-md border border-line px-3 py-2 focus:border-ink" />
      <button type="submit" disabled={state === "busy"} className="w-full rounded-full bg-link py-2.5 font-semibold text-white disabled:opacity-60">
        {state === "busy" ? "Sending…" : "Email me a reset link"}
      </button>
      {state === "error" && <p role="alert" className="text-slab">{msg}</p>}
    </form>
  );
}

export function ResetPassword() {
  const token = useSearchParams().get("token") ?? "";
  const [state, setState] = useState<"idle" | "busy" | "done" | "error">("idle");
  const [msg, setMsg] = useState("");
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    if (f.get("password") !== f.get("again")) {
      setMsg("The two passwords don't match.");
      return setState("error");
    }
    setState("busy");
    const r = await send<{ reset: boolean }>("POST", "/v1/auth/reset", { token, password: f.get("password") });
    if (!r.ok) {
      setMsg(r.fields.password ? `Password ${r.fields.password}.` : r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
      return setState("error");
    }
    setState("done");
  }
  if (!token) return <p className="mt-3 text-muted">This link is missing its code. Open the link from the email again.</p>;
  if (state === "done") {
    return (
      <div className="mt-4">
        <p className="text-ship">Your password is changed, and you&apos;ve been signed out everywhere.</p>
        <Link href="/signin" className="mt-4 inline-block rounded-full bg-link px-5 py-2 font-semibold text-white">Sign in</Link>
      </div>
    );
  }
  const input = "w-full rounded-md border border-line px-3 py-2 focus:border-ink";
  return (
    <form onSubmit={submit} className="mt-4 space-y-3">
      <label htmlFor="password" className="block text-[14px] font-semibold">New password</label>
      <input id="password" name="password" type="password" required minLength={10} autoComplete="new-password" className={input} />
      <label htmlFor="again" className="block text-[14px] font-semibold">Type it again</label>
      <input id="again" name="again" type="password" required minLength={10} autoComplete="new-password" className={input} />
      <p className="text-[13px] text-muted">At least 10 characters.</p>
      <button type="submit" disabled={state === "busy"} className="w-full rounded-full bg-link py-2.5 font-semibold text-white disabled:opacity-60">
        {state === "busy" ? "Saving…" : "Set new password"}
      </button>
      {state === "error" && <p role="alert" className="text-slab">{msg}</p>}
    </form>
  );
}
