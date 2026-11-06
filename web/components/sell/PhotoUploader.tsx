"use client";

import { useEffect, useRef, useState } from "react";

const MAX = 12;
const SIDE = 1600;

type Pending = { key: string; name: string; error?: string };

type Box = { x: number; y: number; w: number; h: number };

/**
 * Finds the card in a photo taken on a plain background (table, mat, sheet of paper).
 * Works on a small copy: the background colour is taken from the photo's edges, and the
 * card is everything that differs from it. Returns null when the background is busy or
 * the card already fills the frame, so the photo is left as it is.
 */
function findCard(bmp: ImageBitmap): Box | null {
  const S = 240;
  const k = S / Math.max(bmp.width, bmp.height);
  const w = Math.max(1, Math.round(bmp.width * k));
  const h = Math.max(1, Math.round(bmp.height * k));
  const c = document.createElement("canvas");
  c.width = w;
  c.height = h;
  const ctx = c.getContext("2d", { willReadFrequently: true });
  if (!ctx) return null;
  ctx.drawImage(bmp, 0, 0, w, h);
  const d = ctx.getImageData(0, 0, w, h).data;
  const ring = Math.max(2, Math.round(Math.min(w, h) * 0.03));
  const edge: number[][] = [];
  for (let y = 0; y < h; y++)
    for (let x = 0; x < w; x++)
      if (x < ring || y < ring || x >= w - ring || y >= h - ring) {
        const i = (y * w + x) * 4;
        edge.push([d[i], d[i + 1], d[i + 2]]);
      }
  const med = (n: number) => edge.map((p) => p[n]).sort((a, b) => a - b)[edge.length >> 1];
  const bg = [med(0), med(1), med(2)];
  const dist = (r: number, g: number, b: number) => Math.abs(r - bg[0]) + Math.abs(g - bg[1]) + Math.abs(b - bg[2]);
  // How much the background itself varies (lighting, texture) sets how different the card must be.
  const noise = edge.map((p) => dist(p[0], p[1], p[2])).sort((a, b) => a - b)[Math.floor(edge.length * 0.9)];
  const cut = Math.min(60, Math.max(24, noise * 1.5));
  const far = (r: number, g: number, b: number) => dist(r, g, b) > cut;
  // A busy background (carpet, wood grain, clutter) makes this guess unsafe.
  if (edge.filter((p) => far(p[0], p[1], p[2])).length > edge.length * 0.25) return null;
  const cols = new Array(w).fill(0);
  const rows = new Array(h).fill(0);
  for (let y = 0; y < h; y++)
    for (let x = 0; x < w; x++) {
      const i = (y * w + x) * 4;
      if (far(d[i], d[i + 1], d[i + 2])) {
        cols[x]++;
        rows[y]++;
      }
    }
  const span = (a: number[], need: number) => {
    let lo = a.findIndex((v) => v > need);
    let hi = a.length - 1 - [...a].reverse().findIndex((v) => v > need);
    if (lo < 0) return null;
    return [lo, hi];
  };
  const cx = span(cols, h * 0.08);
  const cy = span(rows, w * 0.08);
  if (!cx || !cy) return null;
  const bw = cx[1] - cx[0] + 1;
  const bh = cy[1] - cy[0] + 1;
  const area = (bw * bh) / (w * h);
  if (area < 0.08 || area > 0.85) return null; // nothing found, or nothing worth trimming
  // Raw cards are about 5:7 and slabs about 3:5; anything else means the guess went wrong.
  const ratio = Math.min(bw, bh) / Math.max(bw, bh);
  if (ratio < 0.5 || ratio > 0.8) return null;
  const m = Math.round(Math.max(bw, bh) * 0.03); // keep a thin border so edges and corners show
  const x0 = Math.max(0, cx[0] - m);
  const y0 = Math.max(0, cy[0] - m);
  const x1 = Math.min(w, cx[1] + 1 + m);
  const y1 = Math.min(h, cy[1] + 1 + m);
  return { x: x0 / k, y: y0 / k, w: (x1 - x0) / k, h: (y1 - y0) / k };
}

/** Shrinks a photo in the browser (honouring phone rotation) to a JPEG, cropping to the card if asked. */
async function shrink(file: File, crop = false): Promise<Blob> {
  const bmp = await createImageBitmap(file, { imageOrientation: "from-image" });
  let box: Box = { x: 0, y: 0, w: bmp.width, h: bmp.height };
  if (crop) {
    try {
      box = findCard(bmp) ?? box;
    } catch {
      /* keep the whole photo */
    }
  }
  const scale = Math.min(1, SIDE / Math.max(box.w, box.h));
  const w = Math.round(box.w * scale);
  const h = Math.round(box.h * scale);
  const canvas = document.createElement("canvas");
  canvas.width = w;
  canvas.height = h;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("no canvas");
  ctx.fillStyle = "#fff";
  ctx.fillRect(0, 0, w, h);
  ctx.drawImage(bmp, box.x, box.y, box.w, box.h, 0, 0, w, h);
  bmp.close();
  return new Promise((ok, fail) => canvas.toBlob((b) => (b ? ok(b) : fail(new Error("encode"))), "image/jpeg", 0.88));
}

export async function upload(file: File, crop = false): Promise<string> {
  let blob: Blob;
  try {
    blob = await shrink(file, crop);
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
  const [crop, setCrop] = useState(true);
  useEffect(() => {
    try {
      if (localStorage.getItem("ss-autocrop") === "0") setCrop(false);
    } catch {
      /* ignore */
    }
  }, []);
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
        const url = await upload(it.file, crop);
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
      <label className="mt-1.5 flex items-center gap-2 text-[13px]">
        <input
          type="checkbox"
          checked={crop}
          onChange={(e) => {
            setCrop(e.target.checked);
            try {
              localStorage.setItem("ss-autocrop", e.target.checked ? "1" : "0");
            } catch {
              /* ignore */
            }
          }}
          className="h-4 w-4"
        />
        Auto-crop to the card (works best on a plain background)
      </label>
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
