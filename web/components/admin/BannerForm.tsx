"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { send } from "@/lib/client";
import type { SiteBanner } from "@/lib/types";

export type BannerSize = { width: number; height: number; upload_width: number; upload_height: number; min_width: number; min_height: number };

const input = "mt-1 w-full rounded-md border border-line px-3 py-2 text-[15px]";
const label = "block text-[14px] font-semibold";
const help = "mt-0.5 text-[13px] text-muted";

/** Reads an image file's pixel size in the browser, before uploading it. */
function measure(file: File): Promise<{ w: number; h: number }> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const img = new Image();
    img.onload = () => { resolve({ w: img.naturalWidth, h: img.naturalHeight }); URL.revokeObjectURL(url); };
    img.onerror = () => { reject(new Error("This browser can't read that file. Use a JPEG or PNG image.")); URL.revokeObjectURL(url); };
    img.src = url;
  });
}

export default function BannerForm({ initial, size }: { initial: SiteBanner; size: BannerSize }) {
  const router = useRouter();
  const file = useRef<HTMLInputElement>(null);
  const [v, setV] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [msg, setMsg] = useState("");
  const [warn, setWarn] = useState("");
  const [err, setErr] = useState<Record<string, string>>({});
  const ratio = size.width / size.height;

  async function pick(f: File) {
    setErr({});
    setWarn("");
    setMsg("");
    let dims: { w: number; h: number };
    try { dims = await measure(f); } catch (x) { return setErr({ image_url: (x as Error).message }); }
    if (dims.w < size.min_width || dims.h < size.min_height) {
      return setErr({ image_url: `That image is ${dims.w} × ${dims.h}. Banners need to be at least ${size.min_width} × ${size.min_height}; ${size.upload_width} × ${size.upload_height} is best.` });
    }
    const off = Math.abs(dims.w / dims.h - ratio) / ratio;
    if (off > 0.03) {
      setWarn(`That image is ${dims.w} × ${dims.h}, a different shape from the banner (${size.upload_width} × ${size.upload_height}). The edges will be cropped to fit - check the preview below.`);
    }
    setUploading(true);
    try {
      const res = await fetch("/api/v1/admin/banners/photo", { method: "POST", headers: { "content-type": f.type || "image/jpeg" }, body: f });
      const json = await res.json().catch(() => null);
      if (!res.ok) throw new Error(json?.error?.message ?? `Upload failed (${res.status}).`);
      setV({ ...v, image_url: json.photo.url });
    } catch (x) {
      setErr({ image_url: (x as Error).message });
    }
    setUploading(false);
  }

  async function save(active: boolean) {
    setBusy(true);
    setMsg("");
    setErr({});
    const r = await send<{ banner: SiteBanner }>("PUT", "/v1/admin/banners/home", { ...v, active });
    setBusy(false);
    if (!r.ok) {
      setErr(r.fields ?? {});
      return setMsg(r.message);
    }
    setV(r.data.banner);
    setMsg(r.data.banner.active ? "Saved. The banner is live on the homepage." : "Saved. The banner is hidden from the homepage.");
    router.refresh();
  }

  return (
    <div className="mt-6 space-y-6">
      <div className="rounded-lg border border-line bg-wash p-4 text-[14px]">
        <p className="font-semibold">Banner size</p>
        <ul className="mt-1 list-disc space-y-0.5 pl-5">
          <li>Upload at <b>{size.upload_width} × {size.upload_height} pixels</b> (JPEG or PNG, under 15 MB).</li>
          <li>It shows at {size.width} × {size.height}, the standard &quot;billboard&quot; banner size; uploading at double size keeps it sharp on high-resolution screens.</li>
          <li>Smallest accepted: {size.min_width} × {size.min_height}. Other shapes are cropped to fit.</li>
          <li>On phones it shrinks to fit the screen width, so keep text large and away from the edges.</li>
        </ul>
      </div>

      <div>
        <span className={label}>Image</span>
        <div className="mt-2 aspect-[97/25] w-full max-w-[970px] overflow-hidden rounded-lg border border-line bg-wash">
          {v.image_url
            ? /* eslint-disable-next-line @next/next/no-img-element */ <img src={v.image_url} alt="Banner preview" className="h-full w-full object-cover" />
            : <span className="flex h-full items-center justify-center text-[14px] text-muted">No banner image yet - {size.upload_width} × {size.upload_height}</span>}
        </div>
        <p className={help}>Preview at the size and shape shown on the homepage.</p>
        <div className="mt-2 flex gap-4 text-[14px]">
          <button type="button" onClick={() => file.current?.click()} disabled={uploading} className="font-semibold text-link hover:underline disabled:opacity-60">
            {uploading ? "Uploading…" : v.image_url ? "Replace image" : "Upload image"}
          </button>
          {v.image_url && <button type="button" onClick={() => setV({ ...v, image_url: "" })} className="text-muted hover:underline">Remove image</button>}
        </div>
        <input ref={file} type="file" accept="image/jpeg,image/png" className="hidden" onChange={(e) => {
          const f = e.target.files?.[0];
          e.target.value = "";
          if (f) void pick(f);
        }} />
        {warn && <p role="status" className="mt-1 text-[13px] text-slab">{warn}</p>}
        {err.image_url && <p role="alert" className="mt-1 text-[13px] text-slab">{err.image_url}</p>}
      </div>

      <div className="max-w-[640px]">
        <label htmlFor="link_url" className={label}>Link (optional)</label>
        <p className={help}>Where a click goes: a page on this site starting with / (for example /search?q=prizm), or a full https:// address. Outside links open in a new tab.</p>
        <input id="link_url" className={input} value={v.link_url} maxLength={500} placeholder="/search?q=prizm"
          onChange={(e) => setV({ ...v, link_url: e.target.value })} />
        {err.link_url && <p role="alert" className="mt-1 text-[13px] text-slab">{err.link_url}</p>}
      </div>

      <div className="max-w-[640px]">
        <label htmlFor="alt_text" className={label}>Description</label>
        <p className={help}>What the banner says or shows, read aloud to people using screen readers. Required to switch it on.</p>
        <input id="alt_text" className={input} value={v.alt_text} maxLength={150} placeholder="Holiday sale: 20% off graded slabs"
          onChange={(e) => setV({ ...v, alt_text: e.target.value })} />
        {err.alt_text && <p role="alert" className="mt-1 text-[13px] text-slab">{err.alt_text}</p>}
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <button type="button" disabled={busy || uploading} onClick={() => save(true)} className="rounded-full bg-link px-6 py-2.5 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
          {busy ? "Saving…" : v.active ? "Save" : "Save and show on homepage"}
        </button>
        <button type="button" disabled={busy || uploading} onClick={() => save(false)} className="rounded-full border border-line px-6 py-2.5 font-semibold hover:bg-wash disabled:opacity-60">
          {v.active ? "Hide from homepage" : "Save without showing"}
        </button>
        <span className="text-[14px] text-muted">Status: {v.active ? <span className="text-ship">Live</span> : "Hidden"}</span>
      </div>
      {msg && <p role="status" className="text-[14px]">{msg}</p>}
    </div>
  );
}
