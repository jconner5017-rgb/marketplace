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

export default async function EnvelopePage({ params, searchParams }: {
  params: Promise<{ id: string }>; searchParams: Promise<{ format?: string }>;
}) {
  const { id } = await params;
  const { format } = await searchParams;
  await requireUser(`/sell/orders/${id}/envelope`);
  const label = format === "label";
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

  return (
    <div className="mx-auto max-w-[960px] pt-6">
      <style>{`
        @media print {
          @page { size: ${label ? "4in 2in" : "9.5in 4.125in"}; margin: 0; }
          body * { visibility: hidden !important; }
          #print-area, #print-area * { visibility: visible !important; }
          #print-area { position: fixed; left: 0; top: 0; border: 0 !important; box-shadow: none !important; margin: 0 !important; }
        }
      `}</style>
      <div className="print:hidden">
        <Link href={`/sell/orders/${e.order_id}`} className="text-[14px] text-link hover:underline">← Order #{e.order_number}</Link>
        <h1 className="mt-2 text-[24px] font-bold">Print a tracked envelope</h1>
        <p className="mt-1 max-w-[680px] text-[14px] text-muted">
          Print this, put on a First-Class stamp, and mail it. Then mark the order shipped: the barcode becomes its tracking, and
          the buyer sees each USPS scan. Use a flat #10 envelope with a penny sleeve and a thin top loader so it runs through the
          sorting machines; stiff or lumpy envelopes are hand-sorted and don&apos;t get scans.
        </p>
        <div className="mt-4 flex flex-wrap items-center gap-3 text-[14px]">
          <PrintButton label={label ? "Print label" : "Print envelope"} />
          <span className="text-muted">Size:</span>
          <Link href={`/sell/orders/${e.order_id}/envelope`} className={label ? "text-link underline" : "font-semibold"}>#10 envelope</Link>
          <Link href={`/sell/orders/${e.order_id}/envelope?format=label`} className={label ? "font-semibold" : "text-link underline"}>4×2 in label</Link>
        </div>
        <p className="mt-2 text-[13px] text-muted">
          Print at 100% (actual size), not &ldquo;fit to page&rdquo;, so the barcode stays the right size.
          {!e.full_zip && " This address printed with a 5-digit ZIP; it still tracks, though ZIP+4 sorts a little better."}
        </p>
      </div>

      {label ? (
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
      ) : (
        <div id="print-area" className="relative mt-6 overflow-hidden border border-line bg-white text-black shadow-sm"
          style={{ width: "9.5in", height: "4.125in", fontFamily: "Arial, Helvetica, sans-serif" }}>
          <div style={{ position: "absolute", left: "0.35in", top: "0.3in", fontSize: "9pt", lineHeight: 1.2 }}>
            {e.from ? <Lines a={e.from} /> : <div>&nbsp;</div>}
          </div>
          <div style={{ position: "absolute", right: "0.3in", top: "0.25in", width: "0.85in", height: "1in", border: "1px dashed #999",
            fontSize: "7pt", color: "#777", display: "flex", alignItems: "center", justifyContent: "center", textAlign: "center" }}
            className="print:!border-transparent print:!text-transparent">
            Stamp
          </div>
          <div style={{ position: "absolute", left: "3.6in", top: "1.75in" }}>
            <Barcode e={e} />
            <div style={{ fontSize: "12pt", lineHeight: 1.25, marginTop: "0.05in", textTransform: "uppercase" }}><Lines a={e.to} /></div>
          </div>
        </div>
      )}
      <p className="num mt-2 text-[12px] text-muted print:hidden">Barcode {e.human_readable}</p>
    </div>
  );
}
