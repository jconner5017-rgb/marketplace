import type { CSSProperties } from "react";
import Link from "next/link";
import { notFound } from "next/navigation";
import { apiPost } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import PrintButton from "@/components/orders/PrintButton";

export const dynamic = "force-dynamic";
export const metadata = { title: "Print envelope", robots: { index: false } };

type Addr = { name: string; line1: string; line2?: string; city: string; state: string; zip: string };
type Envelope = {
  order_id: string; order_number: number; to: Addr; from: Addr | null;
  barcode: string; human_readable: string; bars: string; svg: string; full_zip: boolean;
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
const FACES: Record<"10" | "6", Face> = {
  "10": { w: 9.5, h: 4.125, addrLeft: 3.6, addrTop: 1.75, addrPt: 12, retPt: 9, stampW: 0.85, stampH: 1 },
  // #6¾ (6½ × 3⅝ in), the usual small envelope for a single card.
  "6": { w: 6.5, h: 3.625, addrLeft: 2.15, addrTop: 1.45, addrPt: 11, retPt: 8, stampW: 0.75, stampH: 0.9 },
};

/** The front of an envelope at true size, in inches. */
function EnvelopeFace({ e, f, style }: { e: Envelope; f: Face; style?: CSSProperties }) {
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
  params: Promise<{ id: string }>; searchParams: Promise<{ format?: string; turn?: string }>;
}) {
  const { id } = await params;
  const { format: fmt, turn } = await searchParams;
  await requireUser(`/sell/orders/${id}/envelope`);
  const format = fmt === "label" || fmt === "6" ? fmt : "10";
  const left = turn === "left";
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

  const base = `/sell/orders/${e.order_id}/envelope`;
  const six = FACES["6"];
  const page = format === "label" ? "4in 2in" : format === "6" ? `${six.h}in ${six.w}in` : "9.5in 4.125in";
  const opt = (on: boolean) => (on ? "font-semibold" : "text-link underline");

  return (
    <div className="mx-auto max-w-[960px] pt-6">
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
        <h1 className="mt-2 text-[24px] font-bold">Print a tracked envelope</h1>
        <p className="mt-1 max-w-[680px] text-[14px] text-muted">
          Print this, put on a $1.31 nonmachinable stamp (needed with a toploader inside; an 82¢ Forever stamp otherwise), and mail it. Then mark the order shipped: the barcode becomes its tracking, and
          the buyer sees each USPS scan. Pack the card in a penny sleeve and a toploader (recommended for thin, non-relic cards);
          thick or relic/memorabilia cards go by envelope at your own risk, and lumpy envelopes may be hand-sorted and get few scans.{" "}
          <Link href="/help/tracked-envelopes" className="text-link underline">Printing tips</Link>
        </p>
        <div className="mt-4 flex flex-wrap items-center gap-3 text-[14px]">
          <PrintButton label={format === "label" ? "Print label" : "Print envelope"} />
          <span className="text-muted">Size:</span>
          <Link href={base} className={opt(format === "10")}>#10 envelope</Link>
          <Link href={`${base}?format=6`} className={opt(format === "6")}>#6¾ envelope (sideways)</Link>
          <Link href={`${base}?format=label`} className={opt(format === "label")}>4×2 in label</Link>
        </div>
        {format === "6" && (
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
          In the print dialog: paper size to match the envelope, margins None, scale 100% (not &ldquo;fit to page&rdquo;),
          headers and footers off.
          {!e.full_zip && " This address printed with a 5-digit ZIP; it still tracks, though ZIP+4 sorts a little better."}
        </p>
      </div>

      {format === "label" ? (
        <div id="print-area" className="mt-6 overflow-hidden border border-line bg-white text-black shadow-sm"
          style={{ width: "4in", height: "2in", padding: "0.15in 0.2in", fontFamily: "Arial, Helvetica, sans-serif" }}>
          <div style={{ fontSize: "7pt", lineHeight: 1.15 }}>
            {e.from ? <>{e.from.name} · {e.from.line1}{e.from.line2 ? `, ${e.from.line2}` : ""} · {e.from.city}, {e.from.state} {e.from.zip}</> : null}
          </div>
          <div style={{ marginTop: "0.18in", marginLeft: "0.35in" }}>
            <Barcode e={e} />
            <div style={{ fontSize: "10pt", lineHeight: 1.2, marginTop: "0.04in", textTransform: "uppercase" }}><Lines a={e.to} /></div>
          </div>
        </div>
      ) : format === "6" ? (
        // A portrait page the width of the envelope's short side; the face is
        // turned a quarter so it prints sideways.
        <div id="print-area" className="relative mt-6 overflow-hidden border border-line bg-white shadow-sm"
          style={{ width: `${six.h}in`, height: `${six.w}in` }}>
          <EnvelopeFace e={e} f={six} style={{
            position: "absolute", left: 0, top: 0, transformOrigin: "top left",
            transform: left ? `translateY(${six.w}in) rotate(-90deg)` : `translateX(${six.h}in) rotate(90deg)`,
          }} />
        </div>
      ) : (
        <div id="print-area" className="mt-6 overflow-hidden border border-line shadow-sm" style={{ width: "9.5in", height: "4.125in" }}>
          <EnvelopeFace e={e} f={FACES["10"]} />
        </div>
      )}
      <p className="num mt-2 text-[12px] text-muted print:hidden">Barcode {e.human_readable}</p>
    </div>
  );
}
