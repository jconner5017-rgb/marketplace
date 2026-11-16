import type { CSSProperties } from "react";
import Link from "next/link";
import { notFound } from "next/navigation";
import { apiPost } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import PrintButton from "@/components/orders/PrintButton";
import PostageImage from "@/components/orders/PostageImage";

export const dynamic = "force-dynamic";
export const metadata = { title: "Print envelope", robots: { index: false } };

type Addr = { name: string; line1: string; line2?: string; city: string; state: string; zip: string };
type Envelope = {
  order_id: string; order_number: number; to: Addr; from: Addr | null;
  barcode: string; human_readable: string; bars: string; svg: string; full_zip: boolean;
  postage_url?: string; tracking_number?: string;
};

function Lines({ a }: { a: Addr }) {
  return (
    <>
      <div>{a.name}</div>
      <div>{a.line1}</div>
      {a.line2 ? <div>{a.line2}</div> : null}
      <div>{a.city}, {a.state} {a.zip}</div>
    </>
  );
}

// The barcode SVG is built by our API from digits only; it carries its own
// true print size (about 2.9 by 0.145 inches).
function Barcode({ e }: { e: Envelope }) {
  return (
    <div>
      <div dangerouslySetInnerHTML={{ __html: e.svg }} />
    </div>
  );
}

type Face = { w: number; h: number; addrLeft: number; addrTop: number; addrPt: number; retPt: number; stampW: number; stampH: number };

// USPS letter sizes: 5 × 3½ in up to 11½ × 6⅛ in.
const MIN_W = 5, MAX_W = 11.5, MIN_H = 3.5, MAX_H = 6.125;
// The postage label EasyPost prints for letters, plus a margin.
const POSTAGE_W = 7.25, POSTAGE_H = 3.25;

/** Where the address and stamp go on an envelope w × h inches. */
function faceFor(w: number, h: number): Face {
  const small = w < 7;
  return {
    w, h,
    addrLeft: Math.round(w * (small ? 0.33 : 0.38) * 100) / 100,
    addrTop: Math.round(h * 0.42 * 100) / 100,
    addrPt: small ? 11 : 12, retPt: small ? 8 : 9,
    stampW: small ? 0.75 : 0.85, stampH: small ? 0.9 : 1,
  };
}

type Size = { key: string; label: string; w: number; h: number; sideways?: boolean; note?: string };
const SIZES: Size[] = [
  { key: "6", label: "#6¾ (6½ × 3⅝)", w: 6.5, h: 3.625, sideways: true, note: "1 to 2 cards in a toploader or card saver" },
  { key: "10", label: "#10 (9½ × 4⅛)", w: 9.5, h: 4.125, note: "3 to 4+ cards" },
  { key: "a9", label: "A9 (8¾ × 5¾)", w: 8.75, h: 5.75, note: "extra room for rigid inserts" },
];

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));
const inches = (v: number) => `${Math.round(v * 1000) / 1000}`;

/** The front of an envelope at true size, in inches. */
function EnvelopeFace({ e, f, style }: { e: Envelope; f: Face; style?: CSSProperties }) {
  if (e.postage_url) {
    // The postage label carries both addresses, the barcode and the postage:
    // it goes in the top right corner, where a stamp would.
    return (
      <div style={{ position: "relative", width: `${f.w}in`, height: `${f.h}in`, background: "#fff", ...style }}>
        <div style={{ position: "absolute", right: "0.125in", top: "0.125in" }}><PostageImage src={e.postage_url} /></div>
      </div>
    );
  }
  return (
    <div style={{ position: "relative", width: `${f.w}in`, height: `${f.h}in`, background: "#fff", color: "#000",
      fontFamily: "Arial, Helvetica, sans-serif", ...style }}>
      <div style={{ position: "absolute", left: "0.3in", top: "0.25in", fontSize: `${f.retPt}pt`, lineHeight: 1.2 }}>
        {e.from ? <Lines a={e.from} /> : null}
      </div>
      <div style={{ position: "absolute", right: "0.25in", top: "0.2in", width: `${f.stampW}in`, height: `${f.stampH}in`, border: "1px dashed #999",
        fontSize: "7pt", color: "#777", display: "flex", alignItems: "center", justifyContent: "center" }}
        className="print:!border-transparent print:!text-transparent">
        Stamp
      </div>
      <div style={{ position: "absolute", left: `${f.addrLeft}in`, top: `${f.addrTop}in` }}>
        <Barcode e={e} />
        <div style={{ fontSize: `${f.addrPt}pt`, lineHeight: 1.25, marginTop: "0.05in", textTransform: "uppercase" }}><Lines a={e.to} /></div>
      </div>
    </div>
  );
}

export default async function EnvelopePage({ params, searchParams }: {
  params: Promise<{ id: string }>; searchParams: Promise<{ format?: string; turn?: string; w?: string; h?: string }>;
}) {
  const { id } = await params;
  const sp = await searchParams;
  await requireUser(`/sell/orders/${id}/envelope`);
  const left = sp.turn === "left";
  const r = await apiPost<{ envelope: Envelope }>(`/v1/orders/${encodeURIComponent(id)}/envelope`);
  if (r.status === 404) notFound();
  const e = r.data?.envelope;

  if (!e) {
    return (
      <div className="mx-auto max-w-[620px] pt-8">
        <Link href={`/sell/orders/${id}`} className="text-[14px] text-link hover:underline">← Order</Link>
        <h1 className="mt-2 text-[24px] font-bold">Tracked envelope</h1>
        <p className="mt-3 rounded-lg bg-wash p-4 text-[14px]">
          {r.status === 503 ? "Envelope tracking isn't switched on yet. Ship in a stamped envelope and mark it shipped as usual."
            : r.status === 409 ? "This order can't use a tracked envelope: it ships as a package, or it's already closed."
            : "We couldn't make the barcode just now. Try again in a moment."}
        </p>
      </div>
    );
  }

  const postage = !!e.postage_url;
  // The size to print: a standard envelope, a custom size, or a label.
  const cw = clamp(parseFloat(sp.w ?? "") || 9.5, MIN_W, MAX_W);
  const ch = clamp(parseFloat(sp.h ?? "") || 4.125, MIN_H, MAX_H);
  const labelKey = postage ? "sticker" : "label";
  let format = sp.format ?? "10";
  if (![...SIZES.map((s) => s.key), "custom", labelKey].includes(format)) format = "10";
  const std = SIZES.find((s) => s.key === format);
  const size: Size = std ?? (format === "custom" ? { key: "custom", label: "Custom", w: cw, h: ch }
    : postage ? { key: "sticker", label: "7 × 3 in label", w: 7, h: 3 } : { key: "label", label: "4 × 2 in label", w: 4, h: 2 });
  // Postage needs room for its 7 × 3 in label.
  const tooSmall = postage && format !== "sticker" && (size.w < POSTAGE_W || size.h < POSTAGE_H);
  const sideways = !!size.sideways && !postage;
  const page = sideways ? `${inches(size.h)}in ${inches(size.w)}in` : `${inches(size.w)}in ${inches(size.h)}in`;
  const base = `/sell/orders/${e.order_id}/envelope`;
  const opt = (on: boolean) => (on ? "font-semibold" : "text-link underline");
  const f = faceFor(size.w, size.h);

  return (
    <div className="mx-auto max-w-[1180px] pt-6">
      <style>{`
        @media print {
          @page { size: ${page}; margin: 0; }
          body * { visibility: hidden !important; }
          #print-area, #print-area * { visibility: visible !important; }
          #print-area { position: fixed; left: 0; top: 0; border: 0 !important; box-shadow: none !important; margin: 0 !important; }
        }
      `}</style>
      <div className="print:hidden">
        <Link href={`/sell/orders/${e.order_id}`} className="text-[14px] text-link hover:underline">← Order #{e.order_number}</Link>
        <h1 className="mt-2 text-[24px] font-bold">{postage ? "Print the envelope with postage" : "Print a tracked envelope"}</h1>
        {postage ? (
          <p className="mt-1 max-w-[680px] text-[14px] text-muted">
            The postage is paid: no stamp needed. The label carries both addresses and the USPS barcode
            {e.tracking_number ? <> (tracking <span className="num">{e.tracking_number}</span>)</> : null}. Print it on the envelope, or on a
            7 × 3 in label sticker, and mail it.{" "}
            <Link href="/help/envelope-postage" className="text-link underline">Envelope postage help</Link>
          </p>
        ) : (
          <p className="mt-1 max-w-[680px] text-[14px] text-muted">
            Print this, put on an 82¢ Forever stamp (a $1.31 nonmachinable stamp for thick cards or very rigid toploaders), and mail it. Then mark the order shipped: the barcode becomes its tracking, and
            the buyer sees each USPS scan. Pack the card in a penny sleeve and a toploader (recommended for thin, non-relic cards);
            thick or relic/memorabilia cards go by envelope at your own risk, and lumpy envelopes may be hand-sorted and get few scans.{" "}
            <Link href="/help/tracked-envelopes" className="text-link underline">Printing tips</Link>
          </p>
        )}
        <div className="mt-4 flex flex-wrap items-center gap-x-3 gap-y-2 text-[14px]">
          {!tooSmall && <PrintButton label={format === "label" || format === "sticker" ? "Print label" : "Print envelope"} />}
          <span className="text-muted">Size:</span>
          {SIZES.map((s) => (
            <Link key={s.key} href={`${base}?format=${s.key}`} className={opt(format === s.key)} title={s.note}>{s.label}</Link>
          ))}
          <Link href={`${base}?format=custom&w=${inches(cw)}&h=${inches(ch)}`} className={opt(format === "custom")}>Custom</Link>
          <Link href={`${base}?format=${labelKey}`} className={opt(format === labelKey)}>{postage ? "7×3 in label sticker" : "4×2 in label"}</Link>
        </div>
        {format === "custom" && (
          <form method="get" action={base} className="mt-3 flex flex-wrap items-end gap-3 text-[13px]">
            <input type="hidden" name="format" value="custom" />
            <label className="font-semibold">Width (in)
              <input name="w" type="number" step="0.125" min={MIN_W} max={MAX_W} defaultValue={inches(cw)}
                className="mt-1 block w-24 rounded-md border border-line px-2 py-1.5 font-normal" />
            </label>
            <label className="font-semibold">Height (in)
              <input name="h" type="number" step="0.125" min={MIN_H} max={MAX_H} defaultValue={inches(ch)}
                className="mt-1 block w-24 rounded-md border border-line px-2 py-1.5 font-normal" />
            </label>
            <button type="submit" className="rounded-full border border-ink px-4 py-1.5 font-semibold">Use this size</button>
            <span className="text-muted">USPS letters: 5 × 3½ in up to 11½ × 6⅛ in.</span>
          </form>
        )}
        {tooSmall && (
          <p className="mt-3 max-w-[680px] rounded-lg bg-wash p-3 text-[13px]">
            The postage label is 7 × 3 in, so it needs an envelope at least {POSTAGE_W} × {POSTAGE_H} in. Use a #10, an A9 or a
            larger custom size, or print it on a 7 × 3 in label sticker and put that on your envelope.
          </p>
        )}
        {sideways && (
          <div className="mt-3 max-w-[680px] rounded-lg bg-wash p-3 text-[13px]">
            <p>
              For a 6½ × 3⅝ in envelope fed into the printer short edge first. In the print dialog choose paper size
              &ldquo;Envelope #6¾&rdquo; (or a custom 3.625 × 6.5 in), portrait, 100% scale.
            </p>
            <p className="mt-1">
              Address came out upside down? Turn it the other way:{" "}
              <Link href={`${base}?format=6`} className={opt(!left)}>turned right</Link>{" · "}
              <Link href={`${base}?format=6&turn=left`} className={opt(left)}>turned left</Link>
            </p>
          </div>
        )}
        <p className="mt-2 text-[13px] text-muted">
          In the print dialog: paper size to match the envelope ({inches(size.w)} × {inches(size.h)} in), margins None, scale 100%
          (not &ldquo;fit to page&rdquo;), headers and footers off.
          {!postage && !e.full_zip && " This address printed with a 5-digit ZIP; it still tracks, though ZIP+4 sorts a little better."}
        </p>
      </div>

      {tooSmall ? null : format === "sticker" && e.postage_url ? (
        <div id="print-area" className="keep-light mt-6 overflow-hidden border border-line bg-white shadow-sm" style={{ width: "7in", height: "3in" }}>
          <PostageImage src={e.postage_url} />
        </div>
      ) : format === "label" ? (
        <div id="print-area" className="keep-light mt-6 overflow-hidden border border-line bg-white text-black shadow-sm"
          style={{ width: "4in", height: "2in", padding: "0.15in 0.2in", fontFamily: "Arial, Helvetica, sans-serif" }}>
          <div style={{ fontSize: "7pt", lineHeight: 1.15 }}>
            {e.from ? <>{e.from.name} · {e.from.line1}{e.from.line2 ? `, ${e.from.line2}` : ""} · {e.from.city}, {e.from.state} {e.from.zip}</> : null}
          </div>
          <div style={{ marginTop: "0.18in", marginLeft: "0.35in" }}>
            <Barcode e={e} />
            <div style={{ fontSize: "10pt", lineHeight: 1.2, marginTop: "0.04in", textTransform: "uppercase" }}><Lines a={e.to} /></div>
          </div>
        </div>
      ) : sideways ? (
        // A portrait page the width of the envelope's short side; the face is
        // turned a quarter so it prints sideways.
        <div id="print-area" className="keep-light relative mt-6 overflow-hidden border border-line bg-white shadow-sm"
          style={{ width: `${size.h}in`, height: `${size.w}in` }}>
          <EnvelopeFace e={e} f={f} style={{
            position: "absolute", left: 0, top: 0, transformOrigin: "top left",
            transform: left ? `translateY(${size.w}in) rotate(-90deg)` : `translateX(${size.h}in) rotate(90deg)`,
          }} />
        </div>
      ) : (
        <div id="print-area" className="keep-light mt-6 overflow-hidden border border-line shadow-sm" style={{ width: `${size.w}in`, height: `${size.h}in` }}>
          <EnvelopeFace e={e} f={f} />
        </div>
      )}
      {!postage && <p className="num mt-2 text-[12px] text-muted print:hidden">Barcode {e.human_readable}</p>}
    </div>
  );
}
