// Product feed for Google Merchant Center free listings (Shopping tab, image
// results) and Microsoft Merchant Center. Every Buy It Now card that's for
// sale, in Google's RSS 2.0 product format. Auctions are left out: free
// listings need a fixed price.
import { API_URL } from "@/lib/api";
import { getCatalog, labelFor } from "@/lib/catalog";
import { CONDITIONS, gradeLabel } from "@/lib/format";
import { SITE_URL, describeListing } from "@/lib/seo";
import type { Listing } from "@/lib/types";

export const dynamic = "force-dynamic";

const MAX_ITEMS = 10000;

const xml = (s: string) =>
  s.replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f]/g, "").replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
const money = (cents: number) => `${(cents / 100).toFixed(2)} USD`;
const abs = (u: string) => (u.startsWith("http") ? u : `${SITE_URL}${u.startsWith("/") ? "" : "/"}${u}`);
const KIND: Record<Listing["category"], string> = { sports: "Sports cards", tcg: "Trading card games", nonsport: "Non-sport cards" };

async function activeListings(): Promise<Listing[]> {
  const out: Listing[] = [];
  for (let offset = 0; offset < MAX_ITEMS; offset += 100) {
    try {
      const res = await fetch(`${API_URL}/v1/listings?sort=newest&limit=100&offset=${offset}`, { next: { revalidate: 3600 } });
      if (!res.ok) break;
      const d = (await res.json()) as { listings: Listing[]; total: number };
      out.push(...d.listings);
      if (offset + 100 >= d.total) break;
    } catch {
      break;
    }
  }
  return out;
}

function item(l: Listing, label: string): string {
  const photos = l.photo_urls.map(abs);
  const onSale = typeof l.sale_price_cents === "number" && l.sale_price_cents > 0 && l.sale_price_cents < l.price_cents;
  const grade = l.grader ? `${l.grader} ${gradeLabel(l.grade)}` : CONDITIONS[l.condition ?? ""] ?? "";
  const lines = [
    `<g:id>${xml(l.id)}</g:id>`,
    `<g:title>${xml(l.title.slice(0, 150))}</g:title>`,
    `<g:description>${xml([describeListing(l, label), l.description].filter(Boolean).join("\n\n").slice(0, 5000))}</g:description>`,
    `<g:link>${xml(`${SITE_URL}/item/${l.id}`)}</g:link>`,
    `<g:image_link>${xml(photos[0])}</g:image_link>`,
    ...photos.slice(1, 11).map((p) => `<g:additional_image_link>${xml(p)}</g:additional_image_link>`),
    `<g:availability>in_stock</g:availability>`,
    `<g:price>${money(l.price_cents)}</g:price>`,
    ...(onSale ? [`<g:sale_price>${money(l.sale_price_cents!)}</g:sale_price>`] : []),
    `<g:condition>used</g:condition>`,
    `<g:identifier_exists>no</g:identifier_exists>`,
    `<g:product_type>${xml(`${KIND[l.category]} > ${label}${l.set_name ? ` > ${l.set_name}` : ""}`)}</g:product_type>`,
    ...(grade ? [`<g:product_detail><g:section_name>Card</g:section_name><g:attribute_name>${l.grader ? "Grade" : "Condition"}</g:attribute_name><g:attribute_value>${xml(grade)}</g:attribute_value></g:product_detail>`] : []),
    `<g:shipping><g:country>US</g:country><g:price>0.00 USD</g:price></g:shipping>`,
    `<g:custom_label_0>${l.category}</g:custom_label_0>`,
    `<g:custom_label_1>${l.grader ? "graded" : "raw"}</g:custom_label_1>`,
  ];
  return `<item>${lines.join("")}</item>`;
}

export async function GET() {
  const [listings, catalog] = await Promise.all([activeListings(), getCatalog()]);
  const items = listings
    .filter((l) => l.status === "active" && l.format === "fixed" && l.quantity > 0 && l.photo_urls.length > 0 && l.price_cents > 0)
    .map((l) => item(l, labelFor(catalog, l.game_or_sport)));
  const body =
    `<?xml version="1.0" encoding="UTF-8"?>\n<rss version="2.0" xmlns:g="http://base.google.com/ns/1.0"><channel>` +
    `<title>Slabs &amp; Singles</title><link>${SITE_URL}</link><description>Sports cards and TCG singles, free US shipping</description>` +
    items.join("\n") +
    `</channel></rss>\n`;
  return new Response(body, { headers: { "Content-Type": "application/xml; charset=utf-8", "Cache-Control": "public, max-age=3600" } });
}
