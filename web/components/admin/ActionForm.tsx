"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

export type FieldDef = {
  name: string;
  label: string;
  type?: "text" | "textarea" | "select" | "datetime" | "checkbox" | "email";
  options?: [string, string][];
  placeholder?: string;
  initial?: string | boolean;
  hint?: string;
  // Plain-data options, so server pages can describe a form (functions
  // can't be passed from a server page to this client component):
  /** Convert the value before sending. */
  as?: "number" | "cents" | "list" | "bps" | "iso";
  /** Send under this key instead of name. */
  key?: string;
  /** Leave the field out when it's blank. */
  omitEmpty?: boolean;
  /** Checkbox: what to send when ticked / not ticked (undefined = leave out). */
  checked?: unknown;
  unchecked?: unknown;
  /** Show only when another field's value is (or isn't) one of these. */
  showWhen?: { field: string; in?: string[]; notIn?: string[] };
  /** Client components only. */
  showIf?: (v: Record<string, string | boolean>) => boolean;
};

function visible(f: FieldDef, v: Record<string, string | boolean>): boolean {
  if (f.showIf && !f.showIf(v)) return false;
  if (f.showWhen) {
    const cur = String(v[f.showWhen.field] ?? "");
    if (f.showWhen.in && !f.showWhen.in.includes(cur)) return false;
    if (f.showWhen.notIn && f.showWhen.notIn.includes(cur)) return false;
  }
  return true;
}

function convert(f: FieldDef, raw: string | boolean): unknown {
  if (f.type === "checkbox" && ("checked" in f || "unchecked" in f)) return raw ? f.checked : f.unchecked;
  const s = String(raw ?? "").trim();
  switch (f.as) {
    case "number": return Number(s) || 0;
    case "cents": return Math.round(Number(s.replace(/[$,]/g, "")) * 100);
    case "bps": return Math.round(Number(s) * 100);
    case "list": return s.split(",").map((a) => a.trim()).filter(Boolean);
    case "iso": return s ? new Date(s).toISOString() : undefined;
  }
  return raw;
}

/**
 * A small staff action form: posts the fields as JSON, shows the API's
 * error next to the right field, and refreshes the page when it works.
 */
export default function ActionForm({
  endpoint, method = "POST", fields, submit, danger, extra, done, transform, compact,
}: {
  endpoint: string;
  method?: string;
  fields: FieldDef[];
  submit: string;
  danger?: boolean;
  extra?: Record<string, unknown>;
  done?: string;
  transform?: (v: Record<string, string | boolean>) => Record<string, unknown>;
  compact?: boolean;
}) {
  const router = useRouter();
  const init: Record<string, string | boolean> = {};
  for (const f of fields) init[f.name] = f.initial ?? (f.type === "checkbox" ? false : f.type === "select" ? f.options?.[0]?.[0] ?? "" : "");
  const [v, setV] = useState(init);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [fieldErr, setFieldErr] = useState<Record<string, string>>({});
  const [ok, setOk] = useState("");

  async function go(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    setFieldErr({});
    setOk("");
    const shown: Record<string, string | boolean> = {};
    const out: Record<string, unknown> = {};
    for (const f of fields) {
      if (!visible(f, v)) continue;
      shown[f.name] = v[f.name];
      if (f.omitEmpty && String(v[f.name] ?? "").trim() === "") continue;
      const val = convert(f, v[f.name]);
      if (val !== undefined) out[f.key ?? f.name] = val;
    }
    const body = { ...(transform ? transform(shown) : out), ...extra };
    const r = await send(method, endpoint, body);
    setBusy(false);
    if (!r.ok) {
      setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
      setFieldErr(r.fields);
      return;
    }
    setOk(done ?? "Saved.");
    router.refresh();
  }

  const input = "mt-1 w-full rounded-md border border-line px-3 py-2 text-[14px]";
  return (
    <form onSubmit={go} className={compact ? "space-y-2" : "space-y-3"}>
      {fields.filter((f) => visible(f, v)).map((f) => (
        <div key={f.name}>
          {f.type === "checkbox" ? (
            <label className="flex items-center gap-2 text-[14px]">
              <input type="checkbox" checked={Boolean(v[f.name])} onChange={(e) => setV({ ...v, [f.name]: e.target.checked })} /> {f.label}
            </label>
          ) : (
            <label className="block text-[13px] font-semibold">
              {f.label}
              {f.type === "textarea" ? (
                <textarea value={String(v[f.name])} onChange={(e) => setV({ ...v, [f.name]: e.target.value })} rows={3}
                  placeholder={f.placeholder} className={`${input} font-normal`} />
              ) : f.type === "select" ? (
                <select value={String(v[f.name])} onChange={(e) => setV({ ...v, [f.name]: e.target.value })} className={`${input} font-normal`}>
                  {f.options!.map(([val, l]) => <option key={val} value={val}>{l}</option>)}
                </select>
              ) : (
                <input type={f.type === "datetime" ? "datetime-local" : f.type === "email" ? "email" : "text"} value={String(v[f.name])}
                  onChange={(e) => setV({ ...v, [f.name]: e.target.value })} placeholder={f.placeholder} className={`${input} font-normal`} />
              )}
            </label>
          )}
          {f.hint && !fieldErr[f.name] && <p className="mt-1 text-[12px] text-muted">{f.hint}</p>}
          {fieldErr[f.name] && <p className="mt-1 text-[12px] text-slab">{fieldErr[f.name]}</p>}
        </div>
      ))}
      {error && !Object.keys(fieldErr).length && <p role="alert" className="text-[13px] text-slab">{error}</p>}
      {ok && <p role="status" className="text-[13px] text-ship">{ok}</p>}
      <button type="submit" disabled={busy}
        className={`rounded-full px-4 py-1.5 text-[14px] font-semibold disabled:opacity-60 ${danger ? "bg-slab text-white hover:opacity-90" : "bg-link text-white hover:bg-link-dark"}`}>
        {busy ? "Saving…" : submit}
      </button>
    </form>
  );
}
