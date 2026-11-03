import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import AcceptInvite from "@/components/AcceptInvite";

export const dynamic = "force-dynamic";
export const metadata = { title: "Trusted buyer invite", robots: { index: false, follow: false } };

export default async function TrustInvite({ params }: { params: Promise<{ code: string }> }) {
  const { code } = await params;
  await requireUser(`/trust/${encodeURIComponent(code)}`);
  const r = await apiGet<{ seller: string }>(`/v1/trust/${encodeURIComponent(code)}`);
  if (!r.data) {
    return (
      <div className="mx-auto max-w-[560px] py-16 text-center">
        <h1 className="text-[22px] font-bold">This invite link doesn&apos;t work any more</h1>
        <p className="mt-2 text-muted">The seller may have made a new one. Ask them to send it again.</p>
        <Link href="/" className="mt-4 inline-block text-link underline">Go to Slabs &amp; Singles</Link>
      </div>
    );
  }
  const seller = r.data.seller;
  return (
    <div className="mx-auto max-w-[600px] pt-12">
      <h1 className="text-[26px] font-bold">{seller} invited you as a trusted buyer</h1>
      <p className="mt-2 text-[15px] text-muted">
        On {seller}&apos;s trusted buyer list, you can choose to have one raw card up to $100 sent in a stamped envelope instead of
        a tracked package. You still pay the same price, and a tracked package stays the default.
      </p>
      <ul className="mt-4 list-disc space-y-1 pl-5 text-[14px]">
        <li>Envelopes get USPS sorting scans but no delivery confirmation.</li>
        <li>If an envelope you chose goes missing, it isn&apos;t covered by Slabs &amp; Singles. Only accept if you know {seller}.</li>
        <li>You decide on each order, at checkout.</li>
      </ul>
      <AcceptInvite code={code} seller={seller} />
    </div>
  );
}
