"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
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

  async function submit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy(true);
    setMessage("");
    setFields({});
    const body =
      mode === "register"
        ? { email: form.get("email"), username: form.get("username"), password: form.get("password") }
        : { email: form.get("email"), password: form.get("password") };
    const res = await send("POST", mode === "register" ? "/v1/auth/signup" : "/v1/auth/login", body);
    setBusy(false);
    if (!res.ok) {
      setMessage(res.message);
      setFields(res.fields);
      setDisabled(!register && res.status === 403);
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
      <Field label="Email" name="email" type="email" autoComplete="email" required error={fields.email} />
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
      <button type="submit" disabled={busy} className="w-full rounded-full bg-link py-2.5 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
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
