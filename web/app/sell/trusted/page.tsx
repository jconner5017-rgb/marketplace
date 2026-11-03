import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import { SITE_URL } from "@/lib/seo";
import TrustedBuyers, { type Trusted } from "@/components/TrustedBuyers";

export const dynamic = "force-dynamic";
export const metadata = { title: "Trusted buyers", robots: { index: false } };

export default async function TrustedPage() {
  await requireUser("/sell/trusted");
  const r = await apiGet<{ buyers: Trusted[]; invite: string }>("/v1/seller/trusted");
  return (
    <div className="mx-auto max-w-[720px] pt-6">
      <Link href="/sell" className="text-[14px] text-link hover:underline">← Seller hub</Link>
      <h1 className="mt-2 text-[26px] font-bold">Trusted buyers</h1>
      <p className="mt-1 text-[14px] text-muted">
        Friends and regulars you trust can choose to have one raw card up to $100 sent in a stamped envelope instead of a
        tracked package, so you skip a $5 label. Anyone can get an envelope on a card up to $20; this raises it to $100 for
        people on your list. Envelopes have no delivery confirmation, so a missing envelope on these orders isn&apos;t covered by
        Slabs &amp; Singles. Only add people you&apos;d trust with that.{" "}
        <Link href="/help/trusted-buyers" className="text-link underline">How it works</Link>
      </p>
      <div className="mt-5">
        {r.data ? <TrustedBuyers buyers={r.data.buyers} invite={r.data.invite} site={SITE_URL} /> : <p className="text-muted">Couldn&apos;t load your list.</p>}
      </div>
    </div>
  );
}
