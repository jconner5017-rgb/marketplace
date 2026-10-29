"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState } from "react";

// Each entry shows only to roles with its permission ("" = all staff).
export const ADMIN_NAV: { group: string; items: { href: string; label: string; perm: string }[] }[] = [
  { group: "Overview", items: [{ href: "/admin", label: "Dashboard", perm: "" }] },
  { group: "People", items: [
    { href: "/admin/users", label: "Members", perm: "users.view" },
    { href: "/admin/sellers", label: "Seller review", perm: "sellers.review" },
    { href: "/admin/staff", label: "Staff and roles", perm: "staff.manage" },
    { href: "/admin/appeals", label: "Appeals", perm: "users.restrict" },
    { href: "/admin/outside-feedback", label: "Outside feedback", perm: "sellers.review" },
  ] },
  { group: "Listings", items: [
    { href: "/admin/listings", label: "Listings", perm: "users.view" },
    { href: "/admin/reports", label: "Reports", perm: "reports.manage" },
    { href: "/admin/catalog", label: "Categories and features", perm: "users.view" },
  ] },
  { group: "Orders", items: [
    { href: "/admin/orders", label: "Orders", perm: "orders.view" },
    { href: "/admin/shipping", label: "Shipping", perm: "orders.view" },
    { href: "/admin/refunds", label: "Refunds and chargebacks", perm: "orders.view" },
    { href: "/admin/fees", label: "Fees", perm: "orders.view" },
  ] },
  { group: "Disputes", items: [
    { href: "/admin/cases", label: "Cases", perm: "cases.manage" },
    { href: "/admin/messages", label: "Message logs", perm: "messages.view" },
  ] },
  { group: "Trust & records", items: [
    { href: "/admin/fraud", label: "Fraud alerts", perm: "reports.manage" },
    { href: "/admin/feedback", label: "Feedback", perm: "feedback.moderate" },
    { href: "/admin/audit", label: "Audit log", perm: "audit.view" },
    { href: "/admin/security", label: "Network allowlist", perm: "staff.manage" },
  ] },
  { group: "Site", items: [{ href: "/admin/announce", label: "Announcements", perm: "announce" }] },
];

const ROLE: Record<string, string> = {
  super_admin: "Super Admin", moderator: "Moderator", support: "Support Agent", auditor: "Financial Auditor",
};

export default function AdminNav({ permissions, role, name }: { permissions: string[]; role: string; name: string }) {
  const path = usePathname();
  const [open, setOpen] = useState(false);
  const groups = ADMIN_NAV.map((g) => ({ ...g, items: g.items.filter((i) => !i.perm || permissions.includes(i.perm)) }))
    .filter((g) => g.items.length);
  const active = (href: string) => (href === "/admin" ? path === "/admin" : path.startsWith(href));
  const current = groups.flatMap((g) => g.items).find((i) => active(i.href));

  return (
    <nav aria-label="Admin" className="mb-4 lg:mb-0">
      <div className="flex items-center justify-between rounded-lg bg-ink px-4 py-3 text-white lg:block">
        <div>
          <p className="text-[12px] uppercase tracking-wide text-white/60">Back office</p>
          <p className="text-[14px] font-semibold">{name} · {ROLE[role] ?? role}</p>
        </div>
        <button type="button" onClick={() => setOpen((o) => !o)} aria-expanded={open}
          className="rounded-full border border-white/40 px-3 py-1 text-[13px] lg:hidden">
          {open ? "Close" : current?.label ?? "Menu"}
        </button>
      </div>
      <div className={`${open ? "block" : "hidden"} mt-2 rounded-lg border border-line p-3 lg:mt-3 lg:block lg:border-0 lg:p-0`}>
        {groups.map((g) => (
          <div key={g.group} className="mb-4">
            <p className="mb-1 px-2 text-[12px] font-semibold uppercase tracking-wide text-muted">{g.group}</p>
            <ul>
              {g.items.map((i) => (
                <li key={i.href}>
                  <Link href={i.href} onClick={() => setOpen(false)} aria-current={active(i.href) ? "page" : undefined}
                    className={`block rounded-md px-2 py-1.5 text-[14px] ${active(i.href) ? "bg-wash font-semibold" : "hover:bg-wash"}`}>
                    {i.label}
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        ))}
        <Link href="/" className="block px-2 text-[13px] text-link hover:underline">← Back to the site</Link>
      </div>
    </nav>
  );
}
