"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

export const HUB_TABS = [
  { href: "/sell", label: "Overview" },
  { href: "/sell/orders", label: "Orders" },
  { href: "/sell/listings", label: "Listings" },
  { href: "/sell/marketing", label: "Marketing" },
  { href: "/sell/performance", label: "Performance" },
  { href: "/sell/payments", label: "Payments" },
  { href: "/sell/reports", label: "Reports" },
];

/** The seller hub's tab bar, shown on every seller hub page. */
export default function HubTabs({ tabs = HUB_TABS }: { tabs?: { href: string; label: string }[] }) {
  const path = usePathname() ?? "/sell";
  const active = (href: string) =>
    href === "/sell" ? path === "/sell" : path === href || path.startsWith(href + "/");
  return (
    <nav aria-label="Seller hub" className="mt-4 flex gap-1 overflow-x-auto border-b border-line text-[15px]">
      <span className="mr-3 self-center whitespace-nowrap py-2 text-[17px] font-bold">Seller Hub</span>
      {tabs.map((t) => (
        <Link key={t.href} href={t.href} aria-current={active(t.href) ? "page" : undefined}
          className={`whitespace-nowrap border-b-2 px-3 py-2 ${active(t.href) ? "border-ink font-semibold" : "border-transparent text-muted hover:text-ink"}`}>
          {t.label}
        </Link>
      ))}
    </nav>
  );
}
