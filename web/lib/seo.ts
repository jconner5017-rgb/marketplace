import type { Listing } from "./types";
import { CONDITIONS, SITE_NAME, gradeLabel } from "./format";

export const SITE_URL = (process.env.SITE_URL || "https://slabsnsingles.com").replace(/\/$/, "");

/** A one-line description for search results and link previews. */
export function describeListing(l: Listing, gameLabel: string): string {
  const grade = l.grader ? `${l.grader} ${gradeLabel(l.grade)}` : `Raw, ${CONDITIONS[l.condition ?? ""] ?? l.condition ?? ""}`;
  const how = l.format === "auction" ? "Auction" : l.offers_enabled ? "Buy It Now or Best Offer" : "Buy It Now";
  const bits = [l.year, l.set_name, l.card_number ? `#${l.card_number}` : null].filter(Boolean).join(" ");
  return `${l.title} · ${bits} · ${gameLabel} · ${grade}. ${how}, free US shipping on ${SITE_NAME}.`.slice(0, 300);
}

/** schema.org Product data so search engines can show price and availability. */
export function productJsonLd(l: Listing, gameLabel: string) {
  const url = `${SITE_URL}/item/${l.id}`;
  const available = l.status === "active" && l.quantity > 0 && (l.format === "fixed" || l.auction?.state === "live");
  const offer: Record<string, unknown> = {
    "@type": "Offer",
    url,
    priceCurrency: "USD",
    price: (l.price_cents / 100).toFixed(2),
    availability: available ? "https://schema.org/InStock" : "https://schema.org/SoldOut",
    itemCondition: "https://schema.org/UsedCondition",
    seller: { "@type": "Organization", name: l.seller_username },
    shippingDetails: {
      "@type": "OfferShippingDetails",
      shippingRate: { "@type": "MonetaryAmount", value: "0", currency: "USD" },
      shippingDestination: { "@type": "DefinedRegion", addressCountry: "US" },
    },
  };
  if (l.format === "auction" && l.auction) offer.priceValidUntil = l.auction.ends_at.slice(0, 10);
  const props: { "@type": string; name: string; value: string }[] = [
    { "@type": "PropertyValue", name: l.category === "tcg" ? "Game" : "Sport", value: gameLabel },
    { "@type": "PropertyValue", name: "Set", value: l.set_name },
  ];
  if (l.grader) props.push({ "@type": "PropertyValue", name: "Grade", value: `${l.grader} ${gradeLabel(l.grade)}` });
  else if (l.condition) props.push({ "@type": "PropertyValue", name: "Condition", value: CONDITIONS[l.condition] ?? l.condition });
  return {
    "@context": "https://schema.org",
    "@type": "Product",
    name: l.title,
    image: l.photo_urls.slice(0, 4),
    description: describeListing(l, gameLabel),
    sku: l.id,
    category: l.category === "tcg" ? "Trading card games" : "Sports trading cards",
    additionalProperty: props,
    offers: offer,
  };
}

/** JSON for a <script type="application/ld+json">, safe to inline. */
export function ldJson(data: unknown): string {
  return JSON.stringify(data).replace(/</g, "\\u003c");
}
