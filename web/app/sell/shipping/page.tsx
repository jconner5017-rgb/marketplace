import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import { CardButton, FinishCard, ShipFromForm } from "@/components/ShippingSettings";
import ReturnsPolicyForm from "@/components/ReturnsPolicyForm";
import PostageBalance, { type PostageAccount } from "@/components/PostageBalance";

export const dynamic = "force-dynamic";
export const metadata = { title: "Shipping settings", robots: { index: false } };

type Settings = {
  ship_from: { name: string; line1: string; line2?: string; city: string; state: string; zip: string; phone?: string } | null;
  card: string;
  enabled: boolean;
  returns_policy: "damaged_only" | "30_days";
};

export default async function ShippingSettingsPage({ searchParams }: { searchParams: Promise<{ card_session?: string }> }) {
  await requireUser("/sell/shipping");
  const sp = await searchParams;
  const r = await apiGet<Settings>("/v1/seller/shipping");
  if (!r.data) return <p className="py-16 text-center">Set up selling first. <Link href="/sell" className="text-link underline">Seller hub</Link></p>;
  const s = r.data;
  const pa = (await apiGet<PostageAccount>("/v1/seller/postage")).data;
  const postageOn = !!pa?.enabled;
  return (
    <div className="max-w-[680px] pt-6">
      <Link href="/sell" className="text-[14px] text-link hover:underline">← Seller hub</Link>
      <h1 className="mt-2 text-[26px] font-bold">Shipping settings</h1>
      <p className="text-muted">Buy tracked labels right from an order. The label is charged to your card at the carrier&apos;s price plus card processing; it&apos;s separate from what buyers pay you.</p>
      {!s.enabled && (
        <p className="mt-4 rounded-lg bg-wash p-3 text-[14px]">Label buying is switched on soon. You can save your address now, and enter tracking numbers by hand in the meantime.</p>
      )}

      <section className="mt-6">
        <h2 className="border-b border-line pb-2 text-[18px] font-bold">Ship-from address</h2>
        <p className="mt-2 text-[14px] text-muted">Printed as the return address on labels and tracked envelopes. We check it with the carrier when you save.</p>
        <div className="mt-3"><ShipFromForm initial={s.ship_from} /></div>
      </section>

      <section className="mt-8">
        <h2 className="border-b border-line pb-2 text-[18px] font-bold">Return policy</h2>
        <p className="mt-2 text-[14px] text-muted">
          Your default for every listing. You can set a different policy on chosen listings in Seller hub → Listings
          (tick them, then Returns). Either way the buyer pays return shipping.
        </p>
        <div className="mt-3"><ReturnsPolicyForm initial={s.returns_policy ?? "damaged_only"} /></div>
      </section>

      {postageOn && pa && (
        <section id="postage" className="mt-8 scroll-mt-24">
          <h2 className="border-b border-line pb-2 text-[18px] font-bold">Envelope postage</h2>
          <p className="mt-2 text-[14px] text-muted">
            Skip the stamp on envelope orders: buy USPS First-Class letter postage on the order page and print it on the envelope, at
            USPS&apos;s price (about 78¢ for a 1 oz letter). Pay from a prepaid balance, so you see the exact postage price with card processing
            paid once per top-up, or pay per envelope with your card (postage plus card processing, about $1.12). Stamps still work too.{" "}
            <Link href="/help/envelope-postage" className="text-link underline">How it works</Link>
          </p>
          <div className="mt-3"><PostageBalance a={pa} /></div>
        </section>
      )}

      <section className="mt-8">
        <h2 className="border-b border-line pb-2 text-[18px] font-bold">{postageOn ? "Credit card for labels and postage" : "Credit card for labels"}</h2>
        <p className="mt-2 text-[14px]">{s.card ? <>Labels are charged to <strong>{s.card}</strong>.</> : "No card saved yet."}</p>
        {sp.card_session ? <FinishCard session={sp.card_session} /> : (s.enabled || postageOn) && <div className="mt-3"><CardButton hasCard={Boolean(s.card)} /></div>}
        <p className="mt-2 text-[13px] text-muted">Your card is stored by Stripe, not by us. A label voided before the carrier scans it is refunded to the card.</p>
      </section>
    </div>
  );
}
