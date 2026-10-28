"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import QRCode from "qrcode";
import { send } from "@/lib/client";

type Mode = "setup" | "verify" | "locked";

/** Authenticator enrollment (QR, first code, backup codes) and the per-sign-in code check. */
export default function MfaForm({ mode, next }: { mode: Mode; next: string }) {
  const router = useRouter();
  const [secret, setSecret] = useState("");
  const [qr, setQr] = useState("");
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(mode === "locked" ? "Too many wrong codes. Staff access is locked for 15 minutes." : "");
  const [backups, setBackups] = useState<string[] | null>(null);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    if (mode !== "setup") return;
    send<{ secret: string; otpauth_url: string }>("POST", "/v1/admin/mfa/setup").then(async (r) => {
      if (!r.ok) return setError(r.message);
      setSecret(r.data.secret);
      setQr(await QRCode.toDataURL(r.data.otpauth_url, { margin: 1, width: 220 }));
    });
  }, [mode]);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    if (mode === "setup") {
      const r = await send<{ backup_codes: string[] }>("POST", "/v1/admin/mfa/confirm", { code });
      setBusy(false);
      if (!r.ok) return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
      setBackups(r.data.backup_codes);
      return;
    }
    const r = await send<{ used_backup: boolean; backup_codes_left: number }>("POST", "/v1/admin/mfa/verify", { code });
    setBusy(false);
    if (!r.ok) return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    router.push(next);
    router.refresh();
  }

  function download() {
    const text = `Slabs & Singles staff backup codes\nEach code works once.\n\n${backups!.join("\n")}\n`;
    const url = URL.createObjectURL(new Blob([text], { type: "text/plain" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = "slabs-and-singles-backup-codes.txt";
    a.click();
    URL.revokeObjectURL(url);
  }

  if (backups) {
    return (
      <div className="mt-6 rounded-lg border border-ink p-5 text-[14px]">
        <p className="font-semibold">Save your backup codes</p>
        <p className="mt-1 text-muted">If you lose your phone, each code gets you in once. They won&apos;t be shown again.</p>
        <ul className="num mt-3 grid grid-cols-2 gap-2 font-mono text-[15px]">
          {backups.map((b) => <li key={b} className="rounded bg-wash px-2 py-1 text-center">{b}</li>)}
        </ul>
        <div className="mt-4 flex flex-wrap gap-2">
          <button type="button" onClick={download} className="rounded-full border border-link px-4 py-2 font-semibold text-link hover:bg-wash">Download</button>
          <button type="button" onClick={() => navigator.clipboard?.writeText(backups.join("\n"))} className="rounded-full border border-line px-4 py-2 font-semibold hover:border-ink">Copy</button>
        </div>
        <label className="mt-4 flex items-center gap-2">
          <input type="checkbox" checked={saved} onChange={(e) => setSaved(e.target.checked)} /> I&apos;ve saved these somewhere safe
        </label>
        <button type="button" disabled={!saved} onClick={() => { router.push(next); router.refresh(); }}
          className="mt-4 w-full rounded-full bg-link py-2.5 font-semibold text-white hover:bg-link-dark disabled:opacity-50">
          Continue to the back office
        </button>
      </div>
    );
  }

  return (
    <form onSubmit={submit} className="mt-6 space-y-4 text-[14px]">
      {mode === "setup" && (
        <ol className="list-decimal space-y-3 pl-5">
          <li>Open Google Authenticator, Authy, 1Password or another authenticator app on your phone.</li>
          <li>
            Scan this code, or enter the key by hand.
            <div className="mt-2 flex flex-col items-start gap-2">
              {qr ? (
                // eslint-disable-next-line @next/next/no-img-element
                <img src={qr} alt="QR code for your authenticator app" width={220} height={220} className="rounded border border-line" />
              ) : (
                <div className="h-[220px] w-[220px] animate-pulse rounded bg-wash" aria-label="Loading QR code" />
              )}
              {secret && <code className="break-all rounded bg-wash px-2 py-1 text-[13px]">{secret.match(/.{1,4}/g)?.join(" ")}</code>}
            </div>
          </li>
          <li>Enter the 6-digit code the app shows.</li>
        </ol>
      )}
      {mode !== "setup" && <p className="text-muted">Enter the 6-digit code from your authenticator app, or one of your backup codes.</p>}
      <div>
        <label htmlFor="mfa-code" className="block font-semibold">Code</label>
        <input id="mfa-code" value={code} onChange={(e) => setCode(e.target.value)} autoComplete="one-time-code" inputMode={mode === "setup" ? "numeric" : "text"}
          autoFocus maxLength={12} disabled={mode === "locked"}
          className="num mt-1 w-full rounded-md border border-line px-3 py-2 text-[20px] tracking-[0.3em]" />
      </div>
      {error && <p role="alert" className="text-slab">{error}</p>}
      <button type="submit" disabled={busy || mode === "locked" || code.trim().length < 6}
        className="w-full rounded-full bg-link py-2.5 font-semibold text-white hover:bg-link-dark disabled:opacity-50">
        {busy ? "Checking…" : mode === "setup" ? "Turn on" : "Continue"}
      </button>
      <p className="text-[12px] text-muted">Staff access lasts 12 hours, then you&apos;ll be asked for a new code.</p>
    </form>
  );
}
