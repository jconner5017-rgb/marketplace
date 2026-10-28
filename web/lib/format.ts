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

/** Every sport a sports card can be listed under (matches the API). */
export const SPORTS: { value: string; label: string }[] = [
  { value: "Baseball", label: "Baseball" },
  { value: "Football", label: "Football" },
  { value: "Basketball", label: "Basketball" },
  { value: "Hockey", label: "Hockey" },
  { value: "Soccer", label: "Soccer" },
  { value: "Auto Racing", label: "Auto Racing" },
  { value: "Wrestling", label: "Wrestling" },
  { value: "MMA", label: "MMA (Mixed Martial Arts)" },
  { value: "Golf", label: "Golf" },
  { value: "Boxing", label: "Boxing" },
  { value: "Tennis", label: "Tennis" },
  { value: "Cycling", label: "Cycling" },
];

/** Every trading card game (matches the API). "Other TCG" covers new games. */
export const GAMES: { value: string; label: string; short: string }[] = [
  { value: "Pokemon", label: "Pokémon TCG", short: "Pokémon" },
  { value: "Magic", label: "Magic: The Gathering", short: "Magic" },
  { value: "Yu-Gi-Oh!", label: "Yu-Gi-Oh! TCG", short: "Yu-Gi-Oh!" },
  { value: "One Piece", label: "One Piece Card Game", short: "One Piece" },
  { value: "Lorcana", label: "Disney Lorcana", short: "Lorcana" },
  { value: "Dragon Ball", label: "Dragon Ball Super / Z", short: "Dragon Ball" },
  { value: "Digimon", label: "Digimon Card Game", short: "Digimon" },
  { value: "Weiss Schwarz", label: "Weiß Schwarz", short: "Weiß Schwarz" },
  { value: "Flesh and Blood", label: "Flesh and Blood", short: "Flesh and Blood" },
  { value: "Final Fantasy", label: "Final Fantasy TCG", short: "Final Fantasy" },
  { value: "Star Wars", label: "Star Wars CCG / Unlimited", short: "Star Wars" },
  { value: "Lord of the Rings", label: "The Lord of the Rings TCG", short: "Lord of the Rings" },
  { value: "Naruto", label: "Naruto CCG", short: "Naruto" },
  { value: "UniVersus", label: "UniVersus (UFS)", short: "UniVersus" },
  { value: "Cardfight!! Vanguard", label: "Cardfight!! Vanguard", short: "Vanguard" },
  { value: "Other TCG", label: "Other trading card games", short: "Other TCG" },
];

const LABELS: Record<string, string> = Object.fromEntries([...SPORTS, ...GAMES].map((x) => [x.value, x.label]));
export function gameLabel(value: string): string {
  return LABELS[value] ?? value;
}

export function sportHref(v: string) {
  return `/search?category=sports&game=${encodeURIComponent(v)}`;
}
export function gameHref(v: string) {
  return `/search?category=tcg&game=${encodeURIComponent(v)}`;
}

/** Quick links shown next to the Sports and Trading cards menus. */
export const QUICK_LINKS: { label: string; href: string }[] = [
  { label: "Baseball", href: sportHref("Baseball") },
  { label: "Football", href: sportHref("Football") },
  { label: "Basketball", href: sportHref("Basketball") },
  { label: "Pokémon", href: gameHref("Pokemon") },
  { label: "Magic", href: gameHref("Magic") },
  { label: "Graded", href: "/search?min_grade=1" },
  { label: "Under $5", href: "/search?max_price=5&sort=price_asc" },
];
