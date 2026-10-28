export const SITE_NAME = process.env.NEXT_PUBLIC_SITE_NAME || "Slabs & Singles";

const usd = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD" });
export function money(cents: number): string {
  return usd.format(cents / 100);
}

export const CONDITIONS: Record<string, string> = {
  NM: "Near Mint",
  LP: "Lightly Played",
  MP: "Moderately Played",
  HP: "Heavily Played",
  DMG: "Damaged",
};

export const ATTRIBUTES: Record<string, string> = {
  holo: "Holo",
  reverse_holo: "Reverse holo",
  first_edition: "1st Edition",
  shadowless: "Shadowless",
  foil: "Foil",
  full_art: "Full art",
  secret_rare: "Secret rare",
  rookie: "Rookie",
  auto: "Autograph",
  patch: "Patch",
  numbered: "Serial numbered",
  parallel: "Parallel",
  short_print: "Short print",
  error: "Error card",
};

export function gradeLabel(grade: number | null): string {
  if (grade == null) return "";
  return Number.isInteger(grade) ? String(grade) : grade.toFixed(1);
}

/** Category shortcuts shown under the header, eBay-style. */
export const CATEGORY_LINKS: { label: string; href: string }[] = [
  { label: "Baseball", href: "/search?category=sports&game=Baseball" },
  { label: "Basketball", href: "/search?category=sports&game=Basketball" },
  { label: "Football", href: "/search?category=sports&game=Football" },
  { label: "Hockey", href: "/search?category=sports&game=Hockey" },
  { label: "Soccer", href: "/search?category=sports&game=Soccer" },
  { label: "Pokémon", href: "/search?category=tcg&game=Pokemon" },
  { label: "Magic", href: "/search?category=tcg&game=Magic" },
  { label: "Yu-Gi-Oh!", href: "/search?category=tcg&game=Yu-Gi-Oh!" },
  { label: "One Piece", href: "/search?category=tcg&game=One%20Piece" },
  { label: "Lorcana", href: "/search?category=tcg&game=Lorcana" },
  { label: "Graded", href: "/search?min_grade=1" },
  { label: "Under $5", href: "/search?max_price=5&sort=price_asc" },
];
