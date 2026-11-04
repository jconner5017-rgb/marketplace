import { getSeller } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import { getFees } from "@/lib/catalog";
import TaxToggle from "@/components/TaxToggle";
import { SellingCosts } from "@/components/sell/HubBits";

export const dynamic = "force-dynamic";
export const metadata = { title: "Payments", robots: { index: false } };

export default async function SellerPayments() {
  await requireUser("/sell/payments");
  const [{ seller }, fees] = await Promise.all([getSeller(), getFees()]);
  return (
    <div className="pb-10 pt-6">
      <h1 className="text-[26px] font-bold">Payments</h1>
      <div className="mt-5 grid gap-4 md:grid-cols-2">
        <section className="rounded-xl border border-line p-4">
          <h2 className="font-bold">Payouts</h2>
          <p className="mt-1 text-[14px] text-muted">
            Buyers pay you directly through your own Stripe account. Your balance, payouts to your bank, refunds and
            disputes are all in your Stripe dashboard.
          </p>
          <a href="https://dashboard.stripe.com/" target="_blank" rel="noopener noreferrer"
            className="mt-3 inline-block rounded-lg bg-ink px-4 py-2 text-[14px] font-semibold text-white hover:opacity-90">Open Stripe dashboard</a>
        </section>
        <section className="rounded-xl border border-line p-4">
          <h2 className="font-bold">Sales tax</h2>
          <p className="mt-1 text-[14px] text-muted">
            You&apos;re responsible for collecting and paying sales tax where your state or the buyer&apos;s state requires it.
            Stripe Tax can work it out on each sale and add it at checkout; it runs on your own Stripe account and
            Stripe charges for it. <a href="https://dashboard.stripe.com/tax" target="_blank" rel="noopener noreferrer" className="text-link underline">Set up Stripe Tax</a>
          </p>
          {seller && <TaxToggle on={seller.collect_tax} />}
        </section>
      </div>
      <SellingCosts fees={fees} />
    </div>
  );
}
