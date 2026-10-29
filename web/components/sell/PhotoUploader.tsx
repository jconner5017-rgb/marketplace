"use client";

import { useRef, useState } from "react";

const MAX = 12;
const SIDE = 1600;

type Pending = { key: string; name: string; error?: string };

/** Shrinks a photo in the browser (honouring phone rotation) to a JPEG. */
async function shrink(file: File): Promise<Blob> {
  const bmp = await createImageBitmap(file, { imageOrientation: "from-image" });
  const scale = Math.min(1, SIDE / Math.max(bmp.width, bmp.height));
  const w = Math.round(bmp.width * scale);
  const h = Math.round(bmp.height * scale);
  const canvas = document.createElement("canvas");
  canvas.width = w;
  canvas.height = h;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("no canvas");
  ctx.fillStyle = "#fff";
  ctx.fillRect(0, 0, w, h);
  ctx.drawImage(bmp, 0, 0, w, h);
  bmp.close();
  return new Promise((ok, fail) => canvas.toBlob((b) => (b ? ok(b) : fail(new Error("encode"))), "image/jpeg", 0.88));
}

async function upload(file: File): Promise<string> {
  let blob: Blob;
  try {
    blob = await shrink(file);
  } catch {
    throw new Error("This browser can't read that file. Use a JPEG or PNG photo.");
  }
  let res: Response;
  try {
    res = await fetch("/api/v1/photos", { method: "POST", headers: { "content-type": "image/jpeg" }, body: blob });
  } catch {
    throw new Error("Upload failed. Check your connection and try again.");
  }
  const json = await res.json().catch(() => null);
  if (!res.ok) throw new Error(json?.error?.message ? json.error.message.charAt(0).toUpperCase() + json.error.message.slice(1) + "." : `Upload failed (${res.status}).`);
  return json.photo.url as string;
}

/** Photo picker for listings: upload, reorder (first is the front), remove, or paste links. */
export default function PhotoUploader({ value, onChange, error }: { value: string[]; onChange: (v: string[]) => void; error?: string }) {
  const input = useRef<HTMLInputElement>(null);
  const [pending, setPending] = useState<Pending[]>([]);
  const [drag, setDrag] = useState(false);
  const [links, setLinks] = useState(false);
  const [linkText, setLinkText] = useState("");
  const urls = useRef(value);
  urls.current = value;

  async function add(files: FileList | File[]) {
    const list = Array.from(files).filter((f) => f.type.startsWith("image/") || /\.(heic|heif)$/i.test(f.name));
    const room = MAX - urls.current.length - pending.filter((p) => !p.error).length;
    const take = list.slice(0, Math.max(0, room));
    const items = take.map((f, i) => ({ key: `${Date.now()}-${i}-${f.name}`, name: f.name, file: f }));
    setPending((p) => [...p.filter((x) => !x.error), ...items.map(({ key, name }) => ({ key, name }))]);
    // Upload in order so the photos keep the order they were picked in.
    for (const it of items) {
      try {
        const url = await upload(it.file);
        onChange([...urls.current, url]);
        setPending((p) => p.filter((x) => x.key !== it.key));
      } catch (e) {
        setPending((p) => p.map((x) => (x.key === it.key ? { ...x, error: (e as Error).message } : x)));
      }
    }
  }

  function move(i: number, d: -1 | 1) {
    const v = [...value];
    const j = i + d;
    if (j < 0 || j >= v.length) return;
    [v[i], v[j]] = [v[j], v[i]];
    onChange(v);
  }

  function addLinks() {
    const add = linkText.split(/\s+/).map((s) => s.trim()).filter((s) => /^https:\/\//.test(s));
    onChange([...value, ...add].slice(0, MAX));
    setLinkText("");
    setLinks(false);
  }

  const busy = pending.some((p) => !p.error);
  const full = value.length >= MAX;

  return (
    <div>
      <p className="text-[14px] font-semibold">Photos</p>
      <p id="photos-hint" className="mt-0.5 text-[12px] text-muted">Up to {MAX}. The first photo is the front and shows in search. Show the back, corners and any flaws.</p>
      <ul className="mt-2 grid grid-cols-3 gap-3 sm:grid-cols-4">
        {value.map((u, i) => (
          <li key={u} className="relative rounded-md border border-line bg-wash p-1">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={u} alt={`Photo ${i + 1}`} className="aspect-[5/7] w-full rounded object-contain" />
            {i === 0 && <span className="absolute left-2 top-2 rounded bg-ink px-1.5 py-0.5 text-[11px] font-semibold text-white">Front</span>}
            <div className="mt-1 flex justify-between text-[12px]">
              <span className="flex gap-1">
                <button type="button" onClick={() => move(i, -1)} disabled={i === 0} aria-label={`Move photo ${i + 1} earlier`} className="rounded px-1.5 hover:bg-white disabled:opacity-30">←</button>
                <button type="button" onClick={() => move(i, 1)} disabled={i === value.length - 1} aria-label={`Move photo ${i + 1} later`} className="rounded px-1.5 hover:bg-white disabled:opacity-30">→</button>
              </span>
              <button type="button" onClick={() => onChange(value.filter((_, k) => k !== i))} className="text-slab underline">Remove</button>
            </div>
          </li>
        ))}
        {pending.map((p) => (
          <li key={p.key} className={`flex aspect-[5/7] flex-col items-center justify-center rounded-md border p-2 text-center text-[12px] ${p.error ? "border-slab text-slab" : "border-line text-muted"}`}>
            {p.error ? <><span className="font-semibold">{p.name}</span><span className="mt-1">{p.error}</span></> : <span aria-live="polite">Uploading…</span>}
          </li>
        ))}
        {!full && (
          <li>
            <button
              type="button"
              onClick={() => input.current?.click()}
              onDragOver={(e) => { e.preventDefault(); setDrag(true); }}
              onDragLeave={() => setDrag(false)}
              onDrop={(e) => { e.preventDefault(); setDrag(false); add(e.dataTransfer.files); }}
              aria-describedby="photos-hint"
              className={`flex aspect-[5/7] w-full flex-col items-center justify-center rounded-md border-2 border-dashed text-[13px] font-semibold ${drag ? "border-link bg-wash text-link" : error ? "border-slab text-slab" : "border-line text-link hover:border-link"}`}
            >
              <span className="text-[24px] leading-none">+</span>
              Add photos
              <span className="mt-1 text-[11px] font-normal text-muted">or drop them here</span>
            </button>
          </li>
        )}
      </ul>
      <input ref={input} type="file" accept="image/*" multiple className="sr-only" tabIndex={-1} aria-hidden
        onChange={(e) => { if (e.target.files) add(e.target.files); e.target.value = ""; }} />
      {busy && <p className="mt-2 text-[12px] text-muted">Keep this page open until the uploads finish.</p>}
      {error && <p className="mt-1 text-[13px] text-slab">{error}</p>}
      <div className="mt-2 text-[13px]">
        {links ? (
          <div className="flex flex-col gap-2 sm:flex-row">
            <textarea value={linkText} onChange={(e) => setLinkText(e.target.value)} rows={2} placeholder="https://…/front.jpg"
              aria-label="Photo links, one per line" className="min-w-0 flex-1 rounded-md border border-line px-3 py-2 font-mono text-[12px]" />
            <span className="flex gap-2 self-start">
              <button type="button" onClick={addLinks} className="rounded-full border border-link px-3 py-1 font-semibold text-link">Add links</button>
              <button type="button" onClick={() => setLinks(false)} className="text-muted underline">Cancel</button>
            </span>
          </div>
        ) : (
          !full && <button type="button" onClick={() => setLinks(true)} className="text-link underline">Add photos by link instead</button>
        )}
      </div>
    </div>
  );
}
