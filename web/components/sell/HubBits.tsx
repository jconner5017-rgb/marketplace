import Link from "next/link";
import { money } from "@/lib/format";
import { feeFor, feeSentence, labelCharge, stripeFee, type FeeSchedule } from "@/lib/catalog";

export const TIERS = [
  { tier: 1, cap: 2000, need: "Where every seller starts" },
  { tier: 2, cap: 25000, need: "50 sales to different buyers, 97% positive, 30 days" },
  { tier: 3, cap: 100000, need: "150 sales to different buyers, 98% positive, 90 days" },
];

/** Everything a sale costs the seller, with a worked example. */
export function SellingCosts({ fees }: { fees: FeeSchedule }) {
  const price = 2500, label = 586;
  const fee = feeFor(fees, price), stripe = stripeFee(price), lab = labelCharge(label);
  const row = "flex justify-between gap-4";
  return (
    <section className="mt-6 rounded-lg border border-line p-4 text-[14px]">
      <h2 className="text-[17px] font-bold">What selling costs</h2>
      <ul className="mt-2 space-y-2">
        <li><strong>Listing:</strong> free.</li>
        <li><strong>Our fee:</strong> {feeSentence(fees)}, only when a card sells.</li>
        <li><strong>Card processing:</strong> Stripe&apos;s fee, usually 2.9% + 30¢ per order, comes out of your Stripe balance.</li>
        <li>
          <strong>Shipping:</strong> buyers never pay it, so it&apos;s yours. One raw card up to $20 can go in an envelope with an
          82¢ Forever stamp, or a $1.31 nonmachinable stamp for thick cards and very rigid toploaders. Everything else ships with a tracked label you buy from the order at the carrier&apos;s price, plus
          2.9% + 30¢ card processing on the label. A bubble mailer by USPS Ground Advantage is usually about $5–7.
          Buyers on your <Link href="/sell/trusted" className="text-link underline">trusted buyer list</Link> can choose an envelope for one raw card up to $100.
        </li>
      </ul>
      <div className="mt-3 rounded-md bg-wash p-3">
        <p className="font-semibold">Example: a {money(price)} card in a bubble mailer</p>
        <dl className="num mt-1 space-y-0.5">
          <div className={row}><dt>Buyer pays</dt><dd>{money(price)}</dd></div>
          <div className={row}><dt>Our fee</dt><dd>−{money(fee)}</dd></div>
          <div className={row}><dt>Stripe processing (about)</dt><dd>−{money(stripe)}</dd></div>
          <div className={row}><dt>Label ({money(label)} + processing)</dt><dd>−{money(lab)}</dd></div>
          <div className={`${row} border-t border-line pt-1 font-bold`}><dt>You keep about</dt><dd>{money(price - fee - stripe - lab)}</dd></div>
        </dl>
      </div>
      <p className="mt-2 text-muted">Set your price with shipping in mind. <Link href="/help/fees" className="text-link underline">Fees in detail</Link></p>
    </section>
  );
}


/** A seller hub overview card: a linked title, then rows. */
export function HubCard({ title, href, children, className = "" }: { title: string; href?: string; children: React.ReactNode; className?: string }) {
  return (
    <section className={`rounded-xl border border-line bg-paper p-4 ${className}`}>
      <h2 className="flex items-center justify-between text-[18px] font-bold">
        {href ? <Link href={href} className="hover:underline">{title}</Link> : title}
        {href && <Link href={href} aria-label={`Open ${title}`} className="text-muted hover:text-ink">›</Link>}
      </h2>
      <div className="mt-2">{children}</div>
    </section>
  );
}

/** One linked row with a count on the right (dimmed when zero). */
export function CountRow({ href, label, n, strong }: { href: string; label: string; n?: number; strong?: boolean }) {
  return (
    <li className="flex items-center justify-between gap-3 border-b border-wash py-1.5 last:border-0">
      <Link href={href} className="text-link hover:underline">{label}</Link>
      {n != null && <span className={`num ${n === 0 ? "text-muted" : strong ? "font-bold text-ink" : "font-semibold"}`}>{n.toLocaleString()}</span>}
    </li>
  );
}

/** Stripe's requirement names, in plain words. */
export function stripeNeed(key: string): string {
  const k = key.replace(/^(individual|company|representative|person_[^.]+)\./, "");
  const map: Record<string, string> = {
    "business_profile.url": "Your website (use your seller page, slabsnsingles.com/seller/your-name)",
    "business_profile.mcc": "Your industry",
    "business_profile.product_description": "A description of what you sell",
    "external_account": "A bank account for payouts",
    "tos_acceptance.date": "Accepting Stripe's terms",
    "tos_acceptance.ip": "Accepting Stripe's terms",
    "verification.document": "A photo of your ID",
    "verification.additional_document": "A second document (such as a utility bill)",
    "id_number": "Your full Social Security number",
    "ssn_last_4": "The last 4 digits of your Social Security number",
    "dob.day": "Your date of birth", "dob.month": "Your date of birth", "dob.year": "Your date of birth",
    "address.line1": "Your address", "address.city": "Your address", "address.postal_code": "Your address", "address.state": "Your address",
    "phone": "Your phone number", "email": "Your email address",
    "first_name": "Your name", "last_name": "Your name",
  };
  return map[key] ?? map[k] ?? key.replace(/[._]/g, " ");
}
