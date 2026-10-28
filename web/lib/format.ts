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

/** "3m ago", "2h ago", "Oct 4" — short times for inbox lists. */
export function ago(iso: string): string {
  const then = new Date(iso).getTime();
  const s = Math.max(0, (Date.now() - then) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  if (s < 86400 * 7) return `${Math.floor(s / 86400)}d ago`;
  return new Date(iso).toLocaleDateString("en-US", { month: "short", day: "numeric" });
}

/** "2d 4h", "3h 12m", "4m 10s" — time left until an ISO date; "Ended" when past. */
export function timeLeft(iso: string, now: number = Date.now()): string {
  let s = Math.floor((Date.parse(iso) - now) / 1000);
  if (s <= 0) return "Ended";
  const d = Math.floor(s / 86400);
  s -= d * 86400;
  const h = Math.floor(s / 3600);
  s -= h * 3600;
  const m = Math.floor(s / 60);
  s -= m * 60;
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m ${s}s`;
}

/** "Sat, Nov 7, 8:15 PM" in the viewer's time zone. */
export function endTime(iso: string): string {
  return new Date(iso).toLocaleString("en-US", {
    weekday: "short", month: "short", day: "numeric", hour: "numeric", minute: "2-digit",
  });
}

/** Bid increments, matching the API. */
export function increment(c: number): number {
  if (c < 100) return 5;
  if (c < 500) return 25;
  if (c < 2500) return 50;
  if (c < 10000) return 100;
  if (c < 25000) return 250;
  if (c < 50000) return 500;
  return 1000;
}

/** "$12.50" → 1250; null when it isn't a dollar amount. */
export function toCents(v: string): number | null {
  const m = v.trim().replace(/[$,]/g, "").match(/^(\d+)(?:\.(\d{1,2}))?$/);
  if (!m) return null;
  return Number(m[1]) * 100 + Number((m[2] ?? "0").padEnd(2, "0"));
}
