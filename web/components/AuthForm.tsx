"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { safeNext, send } from "@/lib/client";
import Field from "./Field";

export default function AuthForm({ mode }: { mode: "signin" | "register" }) {
  const router = useRouter();
  const params = useSearchParams();
  const next = safeNext(params.get("next"));
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [fields, setFields] = useState<Record<string, string>>({});
  const [disabled, setDisabled] = useState(false);
  // Sign-up protection: a server-signed token stamped when the form opened
  // (a sign-up sent within a few seconds of it is refused as a bot).
  const [formToken, setFormToken] = useState("");
  // Cloudflare Turnstile, when the server has its keys.
  const [siteKey, setSiteKey] = useState("");
  const [turnstile, setTurnstile] = useState("");
  const box = useRef<HTMLDivElement>(null);
  const widget = useRef("");
  useEffect(() => {
    if (mode !== "register") return;
    send<{ token: string; turnstile_site_key?: string }>("GET", "/v1/auth/signup-check").then((r) => {
      if (!r.ok) return;
      setFormToken(r.data.token);
      setSiteKey(r.data.turnstile_site_key ?? "");
    });
  }, [mode]);
  useEffect(() => {
    if (!siteKey || !box.current) return;
    const w = window as unknown as { turnstile?: { render: (el: HTMLElement, o: Record<string, unknown>) => string } };
    const draw = () => {
      if (!w.turnstile || !box.current || widget.current) return;
      widget.current = w.turnstile.render(box.current, {
        sitekey: siteKey, callback: (t: string) => setTurnstile(t), "expired-callback": () => setTurnstile(""), "error-callback": () => setTurnstile(""),
      });
    };
    if (w.turnstile) return draw();
    const sc = document.createElement("script");
    sc.src = "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";
    sc.async = true;
    sc.onload = draw;
    document.head.appendChild(sc);
  }, [siteKey]);

  async function submit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy(true);
    setMessage("");
    setFields({});
    const body =
      mode === "register"
        ? { email: form.get("email"), username: form.get("username"), password: form.get("password"),
            website: form.get("website") ?? "", form_token: formToken, turnstile }
        : { email: form.get("email"), password: form.get("password") };
    const res = await send("POST", mode === "register" ? "/v1/auth/signup" : "/v1/auth/login", body);
    setBusy(false);
    if (!res.ok) {
      setMessage(res.message);
      setFields(res.fields);
      setDisabled(!register && res.status === 403);
      if (register && siteKey) {
        (window as unknown as { turnstile?: { reset: (id?: string) => void } }).turnstile?.reset(widget.current);
        setTurnstile("");
      }
      return;
    }
    router.push(next);
    router.refresh();
  }

  const register = mode === "register";
  return (
    <form onSubmit={submit} noValidate className="space-y-4">
      {message && (
        <p role="alert" className="rounded-md border border-slab/40 bg-slab/5 px-3 py-2 text-[14px] text-slab">
          {message.charAt(0).toUpperCase() + message.slice(1)}.
          {disabled && <> <Link href="/appeal" className="font-semibold underline">Appeal this decision</Link></>}
        </p>
      )}
      {register
        ? <Field label="Email" name="email" type="email" autoComplete="email" required error={fields.email} />
        : <Field label="Email or username" name="email" type="text" autoComplete="username" autoCapitalize="none" required error={fields.email} />}
      {register && (
        // Honeypot: hidden from people and screen readers; bots fill it in.
        <div aria-hidden="true" style={{ position: "absolute", left: "-10000px", width: 1, height: 1, overflow: "hidden" }}>
          <label htmlFor="website">Website</label>
          <input id="website" name="website" type="text" tabIndex={-1} autoComplete="off" defaultValue="" />
        </div>
      )}
      {register && (
        <Field
          label="Username"
          name="username"
          autoComplete="username"
          required
          error={fields.username}
          hint="Shown to buyers. 3–30 letters, numbers or underscores."
        />
      )}
      <Field
        label="Password"
        name="password"
        type="password"
        autoComplete={register ? "new-password" : "current-password"}
        required
        error={fields.password}
        hint={register ? "At least 10 characters." : undefined}
      />
      {register && siteKey && <div ref={box} className="min-h-[65px]" />}
      {!register && (
        <p className="-mt-2 text-right text-[15px] sm:text-[13px]"><Link href="/forgot-password" className="inline-block py-2 text-link underline">Forgot password?</Link></p>
      )}
      <button type="submit" disabled={busy || (register && !!siteKey && !turnstile)} className="w-full rounded-full bg-link py-3.5 text-[17px] font-semibold sm:py-2.5 sm:text-[15px] text-white hover:bg-link-dark disabled:opacity-60">
        {busy ? (register ? "Creating account…" : "Signing in…") : register ? "Create account" : "Sign in"}
      </button>
      <p className="text-center text-[14px] text-muted">
        {register ? (
          <>Already have an account? <Link href={`/signin?next=${encodeURIComponent(next)}`} className="text-link underline">Sign in</Link></>
        ) : (
          <>New here? <Link href={`/register?next=${encodeURIComponent(next)}`} className="text-link underline">Create an account</Link></>
        )}
      </p>
    </form>
  );
}
