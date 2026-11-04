"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { send } from "@/lib/client";
import type { Store, StoreCategory, StoreSale } from "@/lib/types";
import { upload } from "./PhotoUploader";

const input = "w-full rounded-lg border border-line px-3 py-2 text-[15px]";
const label = "block text-[14px] font-semibold";
const help = "mt-0.5 text-[13px] text-muted";

const BUSINESS_TYPES = [
  ["llc", "LLC"], ["corporation", "Corporation"], ["partnership", "Partnership"],
  ["sole_proprietor", "Sole proprietor (registered business name)"], ["other", "Other registered business"],
] as const;

/** One image (logo or banner): upload, preview, remove. */
function ImagePick({ value, onChange, wide, name }: { value: string; onChange: (v: string) => void; wide?: boolean; name: string }) {
  const ref = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  return (
    <div>
      <div className={`overflow-hidden rounded-lg border border-line bg-wash ${wide ? "h-24 w-full max-w-[420px]" : "h-24 w-24"}`}>
        {value
          ? /* eslint-disable-next-line @next/next/no-img-element */ <img src={value} alt={`${name} preview`} className="h-full w-full object-cover" />
          : <span className="flex h-full items-center justify-center text-[12px] text-muted">No {name.toLowerCase()}</span>}
      </div>
      <div className="mt-1.5 flex gap-3 text-[14px]">
        <button type="button" onClick={() => ref.current?.click()} disabled={busy} className="font-semibold text-link hover:underline disabled:opacity-60">
          {busy ? "Uploading…" : value ? "Replace" : "Upload"}
        </button>
        {value && <button type="button" onClick={() => onChange("")} className="text-muted hover:underline">Remove</button>}
      </div>
      <input ref={ref} type="file" accept="image/*" className="hidden" onChange={async (e) => {
        const f = e.target.files?.[0];
        e.target.value = "";
        if (!f) return;
        setBusy(true);
        setErr("");
        try { onChange(await upload(f)); } catch (x) { setErr((x as Error).message); }
        setBusy(false);
      }} />
      {err && <p role="alert" className="text-[13px] text-slab">{err}</p>}
    </div>
  );
}

/** Opening a store, and its settings afterwards. */
export function StoreSettingsForm({ store, username }: { store: Store | null; username: string }) {
  const router = useRouter();
  const [v, setV] = useState({
    slug: store?.slug ?? username.toLowerCase().replace(/[^a-z0-9-]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 30),
    name: store?.name ?? "", tagline: store?.tagline ?? "", about: store?.about ?? "", policies: store?.policies ?? "",
    logo_url: store?.logo_url ?? "", banner_url: store?.banner_url ?? "",
    legal_name: store?.legal_name ?? "", business_type: store?.business_type ?? "", website: store?.website ?? "", business_note: store?.business_note ?? "",
  });
  const [fields, setFields] = useState<Record<string, string>>({});
  const [msg, setMsg] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const set = (k: keyof typeof v) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => setV({ ...v, [k]: e.target.value });
  const fe = (k: string) => fields[k] && <p role="alert" className="mt-0.5 text-[13px] text-slab">{fields[k]}</p>;

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    setMsg("");
    setFields({});
    const r = await send<{ store: Store }>("PUT", "/v1/seller/store", v);
    setBusy(false);
    if (!r.ok) {
      setFields(r.fields);
      return setErr(r.message);
    }
    setMsg(store ? "Saved." : "Thanks! We'll check your business details, usually within a day, and let you know.");
    router.refresh();
  }

  return (
    <form onSubmit={save} className="space-y-4">
      <div className="grid gap-4 md:grid-cols-2">
        <div>
          <label htmlFor="s-name" className={label}>Store name</label>
          <input id="s-name" className={input} value={v.name} onChange={set("name")} maxLength={60} required />
          {fe("name")}
        </div>
        <div>
          <label htmlFor="s-slug" className={label}>Store address</label>
          <div className="flex items-center gap-1 text-[15px]"><span className="text-muted">slabsnsingles.com/store/</span>
            <input id="s-slug" className={input} value={v.slug} onChange={set("slug")} maxLength={30} required /></div>
          <p className={help}>3 to 30 letters, numbers and dashes.</p>
          {fe("slug")}
        </div>
      </div>
      <div>
        <label htmlFor="s-tag" className={label}>Tagline</label>
        <input id="s-tag" className={input} value={v.tagline} onChange={set("tagline")} maxLength={120} placeholder="Vintage baseball and graded Pokemon since 1998" />
        {fe("tagline")}
      </div>
      <div className="flex flex-wrap gap-6">
        <div><span className={label}>Logo</span><p className={help}>Square works best.</p><ImagePick value={v.logo_url} onChange={(x) => setV({ ...v, logo_url: x })} name="Logo" /></div>
        <div className="min-w-[260px] flex-1"><span className={label}>Banner</span><p className={help}>Wide, about 1600 × 300.</p><ImagePick value={v.banner_url} onChange={(x) => setV({ ...v, banner_url: x })} wide name="Banner" /></div>
      </div>
      <div>
        <label htmlFor="s-about" className={label}>About the store</label>
        <textarea id="s-about" className={`${input} min-h-[110px]`} value={v.about} onChange={set("about")} maxLength={4000} />
        {fe("about")}
      </div>
      <div>
        <label htmlFor="s-pol" className={label}>Store policies</label>
        <textarea id="s-pol" className={`${input} min-h-[90px]`} value={v.policies} onChange={set("policies")} maxLength={4000}
          placeholder="Combined shipping, how you pack graded cards, how to reach you…" />
        <p className={help}>These add to the site&apos;s rules; they can&apos;t take away buyer protections.</p>
        {fe("policies")}
      </div>

      <fieldset className="rounded-xl border border-line p-4">
        <legend className="px-1 text-[15px] font-bold">Business details</legend>
        <p className={help}>Only our team sees these. Stores are for registered businesses.</p>
        <div className="mt-3 grid gap-4 md:grid-cols-2">
          <div>
            <label htmlFor="s-legal" className={label}>Legal business name</label>
            <input id="s-legal" className={input} value={v.legal_name} onChange={set("legal_name")} maxLength={120} required />
            {fe("legal_name")}
          </div>
          <div>
            <label htmlFor="s-type" className={label}>Business type</label>
            <select id="s-type" className={input} value={v.business_type} onChange={set("business_type")} required>
              <option value="">Choose…</option>
              {BUSINESS_TYPES.map(([k, l]) => <option key={k} value={k}>{l}</option>)}
            </select>
            {fe("business_type")}
          </div>
          <div>
            <label htmlFor="s-web" className={label}>Website (optional)</label>
            <input id="s-web" className={input} value={v.website} onChange={set("website")} placeholder="yourshop.com" />
            {fe("website")}
          </div>
          <div>
            <label htmlFor="s-note" className={label}>Anything that helps us check (optional)</label>
            <input id="s-note" className={input} value={v.business_note} onChange={set("business_note")} maxLength={1000} placeholder="State registration, shop address, show booths…" />
            {fe("business_note")}
          </div>
        </div>
      </fieldset>

      {err && <p role="alert" className="text-[14px] text-slab">{err}</p>}
      {msg && <p role="status" className="text-[14px] text-ship">{msg}</p>}
      <button disabled={busy} className="rounded-full bg-link px-6 py-2.5 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
        {busy ? "Saving…" : store ? "Save store" : "Apply for a store"}
      </button>
    </form>
  );
}

/** The store's categories: rename, add, remove, reorder. */
export function StoreCategoriesForm({ categories }: { categories: StoreCategory[] }) {
  const router = useRouter();
  const [list, setList] = useState(categories.map((c) => ({ id: c.id, name: c.name, listings: c.listings })));
  const [next, setNext] = useState("");
  const [err, setErr] = useState("");
  const [msg, setMsg] = useState("");
  const [busy, setBusy] = useState(false);
  const move = (i: number, d: number) => {
    const j = i + d;
    if (j < 0 || j >= list.length) return;
    const c = [...list];
    [c[i], c[j]] = [c[j], c[i]];
    setList(c);
  };
  async function save() {
    setBusy(true);
    setErr("");
    setMsg("");
    const r = await send("PUT", "/v1/seller/store/categories", { categories: list.map(({ id, name }) => ({ id, name })) });
    setBusy(false);
    if (!r.ok) return setErr(r.message);
    setMsg("Saved.");
    router.refresh();
  }
  return (
    <div>
      <ul className="space-y-2">
        {list.map((c, i) => (
          <li key={c.id || `new-${i}`} className="flex items-center gap-2">
            <label className="sr-only" htmlFor={`cat-${i}`}>Category {i + 1}</label>
            <input id={`cat-${i}`} className={input} value={c.name} maxLength={40}
              onChange={(e) => setList(list.map((x, k) => (k === i ? { ...x, name: e.target.value } : x)))} />
            <span className="num w-16 shrink-0 text-right text-[13px] text-muted">{c.listings} cards</span>
            <button type="button" onClick={() => move(i, -1)} aria-label="Move up" className="px-1 text-muted hover:text-ink">↑</button>
            <button type="button" onClick={() => move(i, 1)} aria-label="Move down" className="px-1 text-muted hover:text-ink">↓</button>
            <button type="button" onClick={() => setList(list.filter((_, k) => k !== i))} className="text-[13px] text-slab hover:underline">Remove</button>
          </li>
        ))}
      </ul>
      <div className="mt-3 flex gap-2">
        <label htmlFor="cat-new" className="sr-only">New category</label>
        <input id="cat-new" className={input} value={next} maxLength={40} placeholder="New category, like Graded Pokemon"
          onChange={(e) => setNext(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); if (next.trim()) { setList([...list, { id: 0, name: next.trim(), listings: 0 }]); setNext(""); } } }} />
        <button type="button" onClick={() => { if (next.trim()) { setList([...list, { id: 0, name: next.trim(), listings: 0 }]); setNext(""); } }}
          className="rounded-lg border border-ink px-4 py-2 text-[14px] font-semibold hover:bg-wash">Add</button>
      </div>
      <p className={help}>Removing a category keeps its cards; they just aren&apos;t in a category. Put cards in categories from Listings.</p>
      {err && <p role="alert" className="mt-2 text-[14px] text-slab">{err}</p>}
      {msg && <p role="status" className="mt-2 text-[14px] text-ship">{msg}</p>}
      <button type="button" onClick={save} disabled={busy} className="mt-3 rounded-full bg-ink px-5 py-2 text-[14px] font-semibold text-white hover:opacity-90 disabled:opacity-60">
        {busy ? "Saving…" : "Save categories"}
      </button>
    </div>
  );
}

const when = (d: string) => new Date(d).toLocaleString("en-US", { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" });

/** Schedule a store sale, and the list of sales. */
export function StoreSalesForm({ sales, categories }: { sales: StoreSale[]; categories: StoreCategory[] }) {
  const router = useRouter();
  const local = (d: Date) => new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
  const [v, setV] = useState({ name: "", percent: "15", category: "", starts: "", ends: local(new Date(Date.now() + 3 * 864e5)) });
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const now = Date.now();
  async function create(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    const r = await send("POST", "/v1/seller/store/sales", {
      name: v.name, percent: Number(v.percent), category_id: v.category ? Number(v.category) : null,
      starts_at: v.starts ? new Date(v.starts).toISOString() : null, ends_at: new Date(v.ends).toISOString(),
    });
    setBusy(false);
    if (!r.ok) return setErr(r.message);
    setV({ ...v, name: "" });
    router.refresh();
  }
  async function end(id: number) {
    const r = await send("DELETE", `/v1/seller/store/sales/${id}`);
    if (!r.ok) return setErr(r.message);
    router.refresh();
  }
  const state = (s: StoreSale) => s.cancelled_at ? "Ended early" : new Date(s.ends_at).getTime() <= now ? "Ended"
    : new Date(s.starts_at).getTime() > now ? "Scheduled" : "Running";
  return (
    <div>
      <form onSubmit={create} className="grid gap-3 md:grid-cols-2">
        <div>
          <label htmlFor="sale-name" className={label}>Sale name</label>
          <input id="sale-name" className={input} value={v.name} maxLength={60} required placeholder="Weekend sale" onChange={(e) => setV({ ...v, name: e.target.value })} />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label htmlFor="sale-pct" className={label}>Percent off</label>
            <select id="sale-pct" className={input} value={v.percent} onChange={(e) => setV({ ...v, percent: e.target.value })}>
              {[5, 10, 15, 20, 25, 30, 35, 40, 45, 50].map((p) => <option key={p} value={p}>{p}%</option>)}
            </select>
          </div>
          <div>
            <label htmlFor="sale-cat" className={label}>Cards</label>
            <select id="sale-cat" className={input} value={v.category} onChange={(e) => setV({ ...v, category: e.target.value })}>
              <option value="">Whole store</option>
              {categories.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
          </div>
        </div>
        <div>
          <label htmlFor="sale-start" className={label}>Starts (blank = now)</label>
          <input id="sale-start" type="datetime-local" className={input} value={v.starts} onChange={(e) => setV({ ...v, starts: e.target.value })} />
        </div>
        <div>
          <label htmlFor="sale-end" className={label}>Ends</label>
          <input id="sale-end" type="datetime-local" className={input} value={v.ends} required onChange={(e) => setV({ ...v, ends: e.target.value })} />
        </div>
        <p className={`${help} md:col-span-2`}>Up to 31 days, one sale at a time. Fixed-price listings only (not auctions or accepted offers). Buyers pay the sale price, and our fee is based on it.</p>
        {err && <p role="alert" className="text-[14px] text-slab md:col-span-2">{err}</p>}
        <div className="md:col-span-2">
          <button disabled={busy} className="rounded-full bg-ink px-5 py-2 text-[14px] font-semibold text-white hover:opacity-90 disabled:opacity-60">{busy ? "Saving…" : "Schedule sale"}</button>
        </div>
      </form>
      {sales.length > 0 && (
        <table className="mt-5 w-full text-[14px]">
          <thead><tr className="border-b border-line text-left text-[13px] text-muted"><th className="py-1.5 font-semibold">Sale</th><th className="py-1.5 font-semibold">When</th><th className="py-1.5 font-semibold">Status</th><th /></tr></thead>
          <tbody>
            {sales.map((s) => (
              <tr key={s.id} className="border-b border-wash">
                <td className="py-1.5">{s.name} · {s.percent}% off {s.category ? s.category : "storewide"}</td>
                <td className="py-1.5 text-muted">{when(s.starts_at)} – {when(s.ends_at)}</td>
                <td className="py-1.5">{state(s)}</td>
                <td className="py-1.5 text-right">{(state(s) === "Running" || state(s) === "Scheduled") && (
                  <button onClick={() => end(s.id)} className="text-[13px] text-slab hover:underline">{state(s) === "Running" ? "End now" : "Cancel"}</button>
                )}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

/** Send followers an update (once a week). */
export function StoreUpdateForm({ followers, last }: { followers: number; last?: string }) {
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [err, setErr] = useState("");
  const [msg, setMsg] = useState("");
  const [busy, setBusy] = useState(false);
  const nextAt = last ? new Date(new Date(last).getTime() + 7 * 864e5) : null;
  const waiting = nextAt && nextAt.getTime() > Date.now();
  async function go(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    setMsg("");
    const r = await send<{ recipients: number; emailed: number }>("POST", "/v1/seller/store/updates", { subject, body });
    setBusy(false);
    if (!r.ok) return setErr(r.message);
    setMsg(`Sent to ${r.data.recipients} ${r.data.recipients === 1 ? "follower" : "followers"}${r.data.emailed ? ` (${r.data.emailed} by email)` : ""}.`);
    setSubject("");
    setBody("");
  }
  if (waiting && !msg) {
    return <p className="text-[14px] text-muted">You can send your next update on {nextAt!.toLocaleDateString("en-US", { month: "long", day: "numeric" })}.</p>;
  }
  return (
    <form onSubmit={go} className="space-y-3">
      <p className={help}>Goes to your {followers.toLocaleString()} {followers === 1 ? "follower" : "followers"} in their notifications, and by email to those who get announcement emails. One update every 7 days.</p>
      <div>
        <label htmlFor="up-sub" className={label}>Subject</label>
        <input id="up-sub" className={input} value={subject} maxLength={100} required onChange={(e) => setSubject(e.target.value)} placeholder="New vintage stock this week" />
      </div>
      <div>
        <label htmlFor="up-body" className={label}>Message</label>
        <textarea id="up-body" className={`${input} min-h-[100px]`} value={body} maxLength={2000} required onChange={(e) => setBody(e.target.value)} />
      </div>
      {err && <p role="alert" className="text-[14px] text-slab">{err}</p>}
      {msg && <p role="status" className="text-[14px] text-ship">{msg}</p>}
      <button disabled={busy || followers === 0} className="rounded-full bg-ink px-5 py-2 text-[14px] font-semibold text-white hover:opacity-90 disabled:opacity-60">
        {busy ? "Sending…" : "Send update"}
      </button>
    </form>
  );
}

/** Per-listing store category picker on the Listings page. */
export function StoreCategoryPicker({ listingId, value, categories }: { listingId: string; value?: number; categories: StoreCategory[] }) {
  const [v, setV] = useState(value ? String(value) : "");
  const [state, setState] = useState<"" | "saving" | "saved" | "error">("");
  return (
    <span className="inline-flex items-center gap-1">
      <label className="sr-only" htmlFor={`sc-${listingId}`}>Store category</label>
      <select id={`sc-${listingId}`} value={v} className="max-w-[150px] rounded border border-line px-1.5 py-1 text-[13px]"
        onChange={async (e) => {
          const nv = e.target.value;
          setV(nv);
          setState("saving");
          const r = await send("POST", "/v1/seller/store/categorize", { category_id: nv ? Number(nv) : 0, listing_ids: [listingId] });
          setState(r.ok ? "saved" : "error");
        }}>
        <option value="">No category</option>
        {categories.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
      </select>
      {state === "saved" && <span className="text-[12px] text-ship" aria-live="polite">✓</span>}
      {state === "error" && <span className="text-[12px] text-slab" role="alert">!</span>}
    </span>
  );
}
