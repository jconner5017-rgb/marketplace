import Link from "next/link";
import { apiGet, getSeller } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import { money, timeLeft } from "@/lib/format";
import type { Hold, Listing, Offer, Order } from "@/lib/types";
import OnboardButton from "@/components/OnboardButton";
import { GradeBadge, Photo } from "@/components/CardBits";
import ListingActions from "@/components/ListingActions";
import BundleDiscountForm from "@/components/BundleDiscountForm";
import TaxToggle from "@/components/TaxToggle";

export const dynamic = "force-dynamic";
export const metadata = { title: "Seller hub" };

const TIERS = [
  { tier: 1, cap: 2000, need: "Where every seller starts" },
  { tier: 2, cap: 25000, need: "50 sales to different buyers, 97% positive, 30 days" },
  { tier: 3, cap: 100000, need: "150 sales to different buyers, 98% positive, 90 days" },
];

export default async function SellPage() {
  const me = await requireUser("/sell");
  const { status, seller } = await getSeller();

  if (status === 404 || !seller) {
    return (
      <div className="mx-auto max-w-[640px] pt-12">
        <h1 className="text-[26px] font-bold">Start selling, {me.username}</h1>
        <p className="mt-2 text-muted">
          Sales are paid straight into your own Stripe account. Setting it up takes about five minutes and asks for
          your bank details and ID, handled by Stripe.
        </p>
        <ul className="mt-5 space-y-2 text-[15px]">
          <li>Listing is free. A $1.50 fee applies per card sold up to $100, then 3% (capped at $25).</li>
          <li>Every listing ships free, so build shipping into your price.</li>
          <li>New sellers can list cards up to $20 each, rising to $250 and then $1,000 as you build feedback.</li>
        </ul>
        <div className="mt-6"><OnboardButton label="Set up payments with Stripe" /></div>
      </div>
    );
  }

  if (!seller.charges_enabled) {
    return (
      <div className="mx-auto max-w-[640px] pt-12">
        <h1 className="text-[26px] font-bold">Finish your Stripe setup</h1>
        <p className="mt-2 text-muted">
          {seller.details_submitted
            ? "Stripe is reviewing your details. Listing unlocks as soon as they approve, usually within minutes."
            : "Your Stripe account isn't finished yet. Pick up where you left off."}
        </p>
        {!seller.details_submitted && <div className="mt-6"><OnboardButton label="Continue Stripe setup" /></div>}
      </div>
    );
  }

  const mine = await apiGet<{ listings: Listing[]; total: number }>("/v1/seller/listings?limit=100");
  const listings = mine.data?.listings ?? [];
  const [offersR, holdsR, ordersR] = await Promise.all([
    apiGet<{ offers: Offer[] }>("/v1/offers?side=selling"),
    apiGet<{ holds: Hold[] }>("/v1/holds?side=selling"),
    apiGet<{ orders: Order[] }>("/v1/orders?side=selling&status=to_ship"),
  ]);
  const toShip = ordersR.data?.orders ?? [];
  const offersWaiting = (offersR.data?.offers ?? []).filter((o) => o.can_respond).length;
  const awaitingPay = (holdsR.data?.holds ?? []).filter((h) => h.status === "awaiting_payment");
  const total = mine.data?.total ?? 0;
  const current = TIERS.find((t) => t.tier === seller.tier) ?? TIERS[0];
  const nextTier = TIERS.find((t) => t.tier === seller.tier + 1);

  return (
    <div className="pt-6">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-[26px] font-bold">Seller hub</h1>
          <p className="text-muted">{me.username} · {total} {total === 1 ? "listing" : "listings"}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Link href="/sell/orders" className="rounded-full border border-link px-5 py-2 font-semibold text-link hover:bg-wash">Orders</Link>
          <Link href="/sell/reports" className="rounded-full border border-link px-5 py-2 font-semibold text-link hover:bg-wash">Reports</Link>
          <Link href="/sell/import" className="rounded-full border border-link px-5 py-2 font-semibold text-link hover:bg-wash">Upload a CSV</Link>
          <Link href="/sell/new" className="rounded-full bg-link px-5 py-2 font-semibold text-white hover:bg-link-dark">List a card</Link>
        </div>
      </div>

      <div className="mt-6 grid gap-4 md:grid-cols-2">
        <div className="rounded-lg border border-line p-4">
          <h2 className="font-bold">Your price limit: {money(current.cap)} per card</h2>
          <p className="mt-1 text-[14px] text-muted">
            {nextTier
              ? `Next: up to ${money(nextTier.cap)} after ${nextTier.need}.`
              : "You're at the top tier. Cards up to the marketplace limit."}
          </p>
          <ol className="mt-3 flex gap-1" aria-label="Seller tiers">
            {TIERS.map((t) => (
              <li key={t.tier} className={`flex-1 rounded px-2 py-1 text-center text-[12px] ${t.tier <= seller.tier ? "bg-ink text-white" : "bg-wash text-muted"}`}>
                {money(t.cap).replace(".00", "")}
              </li>
            ))}
          </ol>
        </div>
        <div className="rounded-lg border border-line p-4">
          <h2 className="font-bold">Multi-card discount</h2>
          <p className="mt-1 text-[14px] text-muted">Buyers who order several cards from you ship in one package, so pass the savings on.</p>
          <BundleDiscountForm type={seller.bundle_discount_type} value={seller.bundle_discount_val} />
        </div>
        <div className="rounded-lg border border-line p-4 md:col-span-2">
          <h2 className="font-bold">Sales tax</h2>
          <p className="mt-1 text-[14px] text-muted">
            You&apos;re responsible for collecting and paying sales tax where your state or the buyer&apos;s state requires it.
            Stripe Tax can work it out on each sale and add it at checkout; it runs on your own Stripe account and
            Stripe charges for it. <a href="https://dashboard.stripe.com/tax" target="_blank" rel="noopener noreferrer" className="text-link underline">Set up Stripe Tax</a>
          </p>
          <TaxToggle on={seller.collect_tax} />
        </div>
      </div>

      {seller.review_status === "rejected" && (
        <p role="alert" className="mt-6 rounded-lg border border-slab/40 bg-slab/5 p-4 text-[14px] text-slab">
          Your seller account wasn&apos;t approved, so your listings are hidden and you can&apos;t list new cards.
          See <a href="/help/contact" className="underline">Contact us</a> if you think this is a mistake.
        </p>
      )}
      {(offersWaiting > 0 || awaitingPay.length > 0 || toShip.length > 0) && (
        <div className="mt-6 flex flex-wrap gap-3 text-[14px]">
          {toShip.length > 0 && (
            <Link href="/sell/orders" className="rounded-lg bg-ink px-4 py-2 font-semibold text-white hover:opacity-90">
              {toShip.length} {toShip.length === 1 ? "order" : "orders"} to ship
            </Link>
          )}
          {offersWaiting > 0 && (
            <Link href="/offers?side=selling" className="rounded-lg border border-ink px-4 py-2 font-semibold hover:bg-wash">
              {offersWaiting} {offersWaiting === 1 ? "offer needs" : "offers need"} a reply
            </Link>
          )}
          {awaitingPay.length > 0 && (
            <span className="rounded-lg bg-wash px-4 py-2">
              {awaitingPay.length} sold, awaiting the buyer&apos;s payment ({money(awaitingPay.reduce((s, h) => s + h.unit_cents * h.quantity, 0))})
            </span>
          )}
        </div>
      )}

      <h2 className="mt-8 border-b border-line pb-2 text-[20px] font-bold">Your listings</h2>
      {listings.length === 0 ? (
        <p className="py-10 text-center text-muted">
          Nothing listed yet. <Link href="/sell/new" className="text-link underline">List your first card</Link> or{" "}
          <Link href="/sell/import" className="text-link underline">upload a CSV</Link>.
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[720px] text-[14px]">
            <thead>
              <tr className="border-b border-line text-left text-[13px] text-muted">
                <th className="py-2 pr-3 font-semibold" colSpan={2}>Card</th>
                <th className="py-2 pr-3 font-semibold">Grade</th>
                <th className="py-2 pr-3 text-right font-semibold">Price</th>
                <th className="py-2 pr-3 text-right font-semibold">Qty</th>
                <th className="py-2 pr-3 font-semibold">Status</th>
                <th className="py-2 font-semibold"><span className="sr-only">Actions</span></th>
              </tr>
            </thead>
            <tbody>
              {listings.map((l) => (
                <tr key={l.id} className="border-b border-wash align-middle">
                  <td className="w-12 py-2 pr-3"><Photo l={l} className="h-14 w-10 rounded" /></td>
                  <td className="py-2 pr-3">
                    <Link href={`/item/${l.id}`} className="title-tight hover:underline">{l.title}</Link>
                    <div className="text-[12px] text-muted">{l.external_id ? `SKU ${l.external_id} · ` : ""}{l.set_name}</div>
                  </td>
                  <td className="py-2 pr-3"><GradeBadge l={l} /></td>
                  <td className="num py-2 pr-3 text-right font-semibold">{money(l.price_cents)}</td>
                  <td className="num py-2 pr-3 text-right">{l.quantity}</td>
                  <td className="py-2 pr-3">
                    {l.format === "auction" && l.auction ? (
                      <span className={l.auction.state === "live" ? "text-ship" : "text-muted"}>
                        {l.auction.state === "live"
                          ? <>Auction · {l.auction.bid_count} {l.auction.bid_count === 1 ? "bid" : "bids"} · <span suppressHydrationWarning>{timeLeft(l.auction.ends_at)}</span></>
                          : l.auction.state === "won" ? "Sold, awaiting payment" : l.auction.state === "unsold" ? "Ended, unsold" : "Ended"}
                      </span>
                    ) : (
                      <span className={l.status === "active" ? "text-ship" : "text-muted"}>
                        {l.status === "active" ? (l.offers_enabled ? "Active · offers on" : "Active") : l.status === "sold_out" ? "Sold out" : "Hidden"}
                      </span>
                    )}
                  </td>
                  <td className="py-2 text-right">
                    <ListingActions id={l.id} status={l.status} auction={l.format === "auction" ? l.auction : undefined} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
