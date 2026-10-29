"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

export function AutoFeedbackForm({ mode, comment }: { mode: string; comment: string }) {
  const router = useRouter();
  const [m, setM] = useState(mode);
  const [c, setC] = useState(comment);
  const [msg, setMsg] = useState("");
  async function go(e: React.FormEvent) {
    e.preventDefault();
    const r = await send("PUT", "/v1/seller/feedback", { auto_feedback: m, auto_feedback_comment: c });
    setMsg(r.ok ? "Saved." : r.message);
    if (r.ok) router.refresh();
  }
  return (
    <form onSubmit={go} className="space-y-3 text-[14px]">
      <fieldset>
        <legend className="font-semibold">Leave buyers a positive automatically</legend>
        <div className="mt-2 space-y-1">
          {[["off", "Off: I'll rate buyers myself"], ["paid", "When payment clears"], ["delivered", "When the carrier or buyer confirms delivery"]].map(([v, l]) => (
            <label key={v} className="flex items-center gap-2"><input type="radio" name="auto" checked={m === v} onChange={() => setM(v)} /> {l}</label>
          ))}
        </div>
      </fieldset>
      <label className="block font-semibold">Comment
        <input value={c} onChange={(e) => setC(e.target.value)} maxLength={200} className="mt-1 block w-full rounded-md border border-line px-3 py-2 font-normal" />
      </label>
      <button type="submit" className="rounded-full bg-link px-4 py-2 font-semibold text-white hover:bg-link-dark">Save</button>
      {msg && <span role="status" className="ml-3">{msg}</span>}
    </form>
  );
}

export function OutsideClaimForm({ platforms }: { platforms: Record<string, string> }) {
  const router = useRouter();
  const [v, setV] = useState({ platform: "ebay", profile_url: "", feedback_count: "", positive_percent: "" });
  const [fields, setFields] = useState<Record<string, string>>({});
  const [code, setCode] = useState("");
  const [msg, setMsg] = useState("");
  async function go(e: React.FormEvent) {
    e.preventDefault();
    setFields({});
    setMsg("");
    const r = await send<{ claim: { verify_code: string } }>("POST", "/v1/seller/outside-feedback", {
      platform: v.platform, profile_url: v.profile_url, feedback_count: Number(v.feedback_count) || 0, positive_percent: Number(v.positive_percent) || 0,
    });
    if (!r.ok) {
      setFields(r.fields);
      return setMsg(r.message);
    }
    setCode(r.data.claim.verify_code);
    router.refresh();
  }
  const input = "mt-1 block w-full rounded-md border border-line px-3 py-2 font-normal";
  return (
    <form onSubmit={go} className="grid gap-3 text-[14px] sm:grid-cols-2">
      <label className="font-semibold">Site
        <select value={v.platform} onChange={(e) => setV({ ...v, platform: e.target.value })} className={input}>
          {Object.entries(platforms).map(([k, l]) => <option key={k} value={k}>{l}</option>)}
        </select>
      </label>
      <label className="font-semibold">Link to your profile there
        <input value={v.profile_url} onChange={(e) => setV({ ...v, profile_url: e.target.value })} placeholder="https://www.ebay.com/usr/…" className={input} />
        {fields.profile_url && <span className="mt-1 block text-[13px] font-normal text-slab">{fields.profile_url}</span>}
      </label>
      <label className="font-semibold">Number of ratings
        <input inputMode="numeric" value={v.feedback_count} onChange={(e) => setV({ ...v, feedback_count: e.target.value })} className={input} />
      </label>
      <label className="font-semibold">Positive %
        <input inputMode="decimal" value={v.positive_percent} onChange={(e) => setV({ ...v, positive_percent: e.target.value })} placeholder="99.8" className={input} />
      </label>
      <div className="sm:col-span-2">
        <button type="submit" className="rounded-full bg-link px-4 py-2 font-semibold text-white hover:bg-link-dark">Submit for checking</button>
        {msg && <span role="alert" className="ml-3 text-slab">{msg}</span>}
      </div>
      {code && (
        <div className="rounded-lg border border-ink bg-wash p-3 sm:col-span-2">
          <p className="font-semibold">Now add this code to your profile bio there: <code className="rounded bg-white px-1.5 py-0.5">{code}</code></p>
          <p className="mt-1 text-[13px] text-muted">Our team checks for it, usually within two business days. You can remove it once you&apos;re verified.</p>
        </div>
      )}
    </form>
  );
}
