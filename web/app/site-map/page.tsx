import Link from "next/link";
import { FOOTER } from "@/components/Footer";
import { gameHref, sportHref } from "@/lib/format";
import { getCatalog } from "@/lib/catalog";

export const metadata = { title: "Site map" };

const MEMBER = [
  { label: "Watchlist", href: "/watchlist" },
  { label: "Cart", href: "/cart" },
  { label: "Messages", href: "/messages" },
  { label: "Seller hub", href: "/sell" },
  { label: "List a card", href: "/sell/new" },
  { label: "Upload a CSV", href: "/sell/import" },
];

export default async function SiteMap() {
  const cat = await getCatalog();
  const cols = [
    { title: "Sports cards", links: cat.sports.map((s) => ({ label: s.label, href: sportHref(s.value) })) },
    { title: "Trading card games", links: cat.games.map((g) => ({ label: g.label, href: gameHref(g.value) })) },
    { title: "Your account", links: MEMBER },
    ...FOOTER,
  ];
  return (
    <div className="pt-8">
      <h1 className="text-[28px] font-bold">Site map</h1>
      <div className="mt-8 grid grid-cols-2 gap-x-8 gap-y-10 sm:grid-cols-3 lg:grid-cols-4">
        {cols.map((c) => (
          <section key={c.title}>
            <h2 className="border-b border-line pb-2 text-[15px] font-bold">{c.title}</h2>
            <ul className="mt-3 space-y-1.5 text-[14px]">
              {c.links.map((l) => <li key={l.label}><Link href={l.href} className="text-link hover:underline">{l.label}</Link></li>)}
            </ul>
          </section>
        ))}
      </div>
    </div>
  );
}
