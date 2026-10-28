import Link from "next/link";
import { SITE_NAME } from "@/lib/format";

// Social accounts. Leave a URL empty to hide that link.
const SOCIAL = [
  { label: "Facebook", href: process.env.NEXT_PUBLIC_FACEBOOK_URL || "" },
  { label: "X (Twitter)", href: process.env.NEXT_PUBLIC_X_URL || "" },
  { label: "Instagram", href: process.env.NEXT_PUBLIC_INSTAGRAM_URL || "" },
].filter((s) => s.href);

export const FOOTER: { title: string; links: { label: string; href: string }[] }[] = [
  {
    title: "Buy",
    links: [
      { label: "Registration", href: "/help/registration" },
      { label: "How buying works", href: "/help/buying" },
      { label: "Shop sports cards", href: "/search?category=sports" },
      { label: "Shop TCG singles", href: "/search?category=tcg" },
      { label: "Sales and events", href: "/help/sales-events" },
    ],
  },
  {
    title: "Sell",
    links: [
      { label: "Start selling", href: "/sell" },
      { label: "How to sell", href: "/help/selling" },
      { label: "Selling fees", href: "/help/fees" },
      { label: "Seller levels", href: "/help/seller-levels" },
      { label: "Shipping your cards", href: "/help/shipping" },
      { label: "Bulk upload", href: "/help/bulk-upload" },
    ],
  },
  {
    title: "Help & Contact",
    links: [
      { label: "Help center", href: "/help" },
      { label: "Problems with an order", href: "/help/returns" },
      { label: "Staying safe", href: "/help/safety" },
      { label: "Taxes", href: "/help/taxes" },
      { label: "Contact us", href: "/help/contact" },
    ],
  },
  {
    title: "Community",
    links: [
      { label: "Announcements", href: "/announcements" },
      { label: "Community guidelines", href: "/help/community" },
    ],
  },
  {
    title: "About",
    links: [
      { label: "About us", href: "/help/about" },
      { label: "News", href: "/announcements" },
      { label: "Policies", href: "/help/policies" },
      { label: "Advertise with us", href: "/help/advertise" },
    ],
  },
  {
    title: "Tools & apps",
    links: [
      { label: "Developers", href: "/help/developers" },
      { label: "Security center", href: "/help/security" },
      { label: "Site map", href: "/site-map" },
    ],
  },
];

export default function Footer() {
  return (
    <footer className="mt-16 border-t border-line bg-wash">
      <div className="mx-auto max-w-[1280px] px-4 py-10">
        {/* Even columns that all start on the same line; wraps to 3 then 2 on small screens. */}
        <nav aria-label="Footer" className="grid grid-cols-2 gap-x-6 gap-y-8 sm:grid-cols-3 lg:grid-cols-6">
          {FOOTER.map((col) => (
            <div key={col.title}>
              <h2 className="text-[14px] font-bold">{col.title}</h2>
              <ul className="mt-3 space-y-2 text-[13px]">
                {col.links.map((l) => (
                  <li key={l.label}>
                    <Link href={l.href} className="text-muted hover:text-ink hover:underline">{l.label}</Link>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </nav>
        <div className="mt-10 flex flex-wrap items-center gap-x-6 gap-y-2 border-t border-line pt-5 text-[12px] text-muted">
          <span>© {new Date().getFullYear()} {SITE_NAME}</span>
          <span>US buyers and sellers only</span>
          <span>Payments by Stripe</span>
          {SOCIAL.length > 0 && (
            <span className="flex gap-4 sm:ml-auto">
              {SOCIAL.map((s) => (
                <a key={s.label} href={s.href} rel="noopener noreferrer" target="_blank" className="hover:text-ink hover:underline">{s.label}</a>
              ))}
            </span>
          )}
        </div>
      </div>
    </footer>
  );
}
