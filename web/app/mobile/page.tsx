import Link from "next/link";
import { SITE_NAME } from "@/lib/format";

export const metadata = {
  title: "Mobile account management",
  description: `Everything you can do on ${SITE_NAME} from your phone: buy, sell, list with your camera, ship and manage your account.`,
};

type Item = { title: string; body: string; href: string; cta: string };

const BUYING: Item[] = [
  { title: "Search and browse", body: "Find cards by player, set, game or grade, and filter by price.", href: "/search", cta: "Search cards" },
  { title: "Watchlist and saved searches", body: "Tap the heart on any card to watch it, and save searches to come back to.", href: "/watchlist", cta: "Open watchlist" },
  { title: "Make offers", body: "Send an offer on cards that accept them and answer counteroffers.", href: "/offers", cta: "See offers" },
  { title: "Cart and checkout", body: "Buy from several sellers in one cart and pay securely through our payment partner.", href: "/cart", cta: "Open cart" },
  { title: "Track purchases", body: "See order status and tracking, and leave feedback when your cards arrive.", href: "/purchases", cta: "View purchases" },
];

const SELLING: Item[] = [
  { title: "List straight from your camera", body: "Take photos of the front and back. Auto-crop trims the background on a plain table.", href: "/sell/new", cta: "List a card" },
  { title: "Identify from photos", body: "Tap Identify from photos and we fill in the player, set, year and number for you to check.", href: "/sell/new", cta: "Try it" },
  { title: "Orders and shipping", body: "See what sold, mark orders shipped and add tracking numbers.", href: "/sell/orders", cta: "Seller orders" },
  { title: "Your listings", body: "Change prices, run sales, and end or relist cards.", href: "/sell/listings", cta: "Manage listings" },
  { title: "Messages", body: "Answer buyer questions quickly from anywhere.", href: "/messages", cta: "Open messages" },
];

const ACCOUNT: Item[] = [
  { title: "Stay signed in", body: "Sign in once and your phone stays signed in for 30 days unless you sign out.", href: "/signin", cta: "Sign in" },
  { title: "Email settings", body: "Choose which emails you get about offers, orders and saved searches.", href: "/account/email", cta: "Email settings" },
  { title: "Account standing", body: "Check your seller standing and any notices on your account.", href: "/account/standing", cta: "Account standing" },
  { title: "Reset your password", body: "Forgot it? Reset it by email in a minute.", href: "/forgot-password", cta: "Reset password" },
];

const DESKTOP = [
  "Bulk importing a spreadsheet or a FireCardManager export",
  "Printing shipping envelopes and packing slips",
  "Detailed sales reports",
];

function Section({ title, items }: { title: string; items: Item[] }) {
  return (
    <section className="mt-10">
      <h2 className="border-b border-line pb-2 text-[20px] font-bold">{title}</h2>
      <ul className="mt-4 grid gap-4 sm:grid-cols-2">
        {items.map((it) => (
          <li key={it.title} className="rounded-lg border border-line p-4">
            <h3 className="text-[16px] font-semibold">{it.title}</h3>
            <p className="mt-1 text-[15px] text-muted">{it.body}</p>
            <Link href={it.href} className="mt-3 inline-block py-1 text-[15px] font-semibold text-link underline">{it.cta}</Link>
          </li>
        ))}
      </ul>
    </section>
  );
}

export default function MobilePage() {
  return (
    <div className="mx-auto max-w-[900px] pt-8">
      <h1 className="text-[28px] font-bold">Mobile account management</h1>
      <p className="mt-2 text-[16px] text-muted">
        {SITE_NAME} works in your phone&apos;s web browser, with no app to install. Here&apos;s what you can do from your phone.
      </p>
      <div className="mt-5 rounded-lg bg-wash p-4 text-[15px]">
        <p className="font-semibold">Tip: add us to your home screen</p>
        <p className="mt-1 text-muted">
          iPhone: tap Share, then Add to Home Screen. Android: tap the ⋮ menu, then Add to Home screen. The icon opens the site in one tap.
        </p>
      </div>
      <Section title="Buying" items={BUYING} />
      <Section title="Selling" items={SELLING} />
      <Section title="Your account" items={ACCOUNT} />
      <section className="mt-10">
        <h2 className="border-b border-line pb-2 text-[20px] font-bold">Easier on a computer</h2>
        <p className="mt-3 text-[15px] text-muted">These work on a phone too, but a bigger screen helps:</p>
        <ul className="mt-2 list-disc space-y-1 pl-5 text-[15px]">
          {DESKTOP.map((d) => <li key={d}>{d}</li>)}
        </ul>
      </section>
    </div>
  );
}
