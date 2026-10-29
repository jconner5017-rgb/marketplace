import "server-only";
import { API_URL } from "./api";
import { ATTRIBUTES, GAMES, SPORTS } from "./format";

export type Choice = { value: string; label: string; short: string };
export type SiteCatalog = { sports: Choice[]; games: Choice[]; features: Record<string, string> };
export type FeeSchedule = { flat_cents: number; threshold_cents: number; percent_bps: number; cap_cents: number };

type ApiCategory = { kind: "sports" | "tcg"; value: string; label: string; short_label: string; sort: number };
type ApiFeature = { key: string; label: string };

const FALLBACK: SiteCatalog = {
  sports: SPORTS.map((s) => ({ ...s, short: s.value })),
  games: GAMES.map((g) => ({ value: g.value, label: g.label, short: g.short })),
  features: ATTRIBUTES,
};

/** Sports, games and card features as staff have set them (cached 5 minutes). */
export async function getCatalog(): Promise<SiteCatalog> {
  try {
    const res = await fetch(API_URL + "/v1/catalog", { next: { revalidate: 300 } });
    if (!res.ok) return FALLBACK;
    const d = (await res.json()) as { categories?: ApiCategory[]; features?: ApiFeature[] };
    if (!d.categories?.length) return FALLBACK;
    const pick = (k: string) => d.categories!.filter((c) => c.kind === k).map((c) => ({ value: c.value, label: c.label, short: c.short_label }));
    return {
      sports: pick("sports"),
      games: pick("tcg"),
      features: Object.fromEntries((d.features ?? []).map((f) => [f.key, f.label])),
    };
  } catch {
    return FALLBACK;
  }
}

/** Display label for a stored sport or game value. */
export function labelFor(cat: SiteCatalog, value: string): string {
  return [...cat.sports, ...cat.games].find((c) => c.value === value)?.label ?? value;
}

const FEE_FALLBACK: FeeSchedule = { flat_cents: 150, threshold_cents: 10000, percent_bps: 300, cap_cents: 2500 };

export type FeeInfo = { current: FeeSchedule; upcoming: (FeeSchedule & { effective_at: string })[] };

/** The fee schedule in force and any scheduled changes (cached 1 minute). */
export async function getFeeInfo(): Promise<FeeInfo> {
  try {
    const res = await fetch(API_URL + "/v1/fees", { next: { revalidate: 60 } });
    if (res.ok) {
      const d = (await res.json()) as Partial<FeeInfo>;
      if (d.current) return { current: d.current, upcoming: d.upcoming ?? [] };
    }
  } catch {}
  return { current: FEE_FALLBACK, upcoming: [] };
}

/** The fee schedule in force. */
export async function getFees(): Promise<FeeSchedule> {
  return (await getFeeInfo()).current;
}

function dollars(cents: number): string {
  return cents % 100 === 0 ? `$${(cents / 100).toLocaleString("en-US")}` : `$${(cents / 100).toFixed(2)}`;
}

/** Fee amounts as words, for text: { flat: "$1.50", threshold: "$100", percent: "3%", cap: "$25" }. */
export function feeWords(f: FeeSchedule): Record<"flat" | "threshold" | "percent" | "cap", string> {
  return {
    flat: dollars(f.flat_cents),
    threshold: dollars(f.threshold_cents),
    percent: `${+(f.percent_bps / 100).toFixed(2)}%`,
    cap: dollars(f.cap_cents),
  };
}

/** One sentence describing the fees. */
export function feeSentence(f: FeeSchedule): string {
  const w = feeWords(f);
  return `${w.flat} per card sold up to ${w.threshold}, then ${w.percent} (capped at ${w.cap})`;
}

/** Replaces {fee.flat}, {fee.threshold}, {fee.percent} and {fee.cap} in help text. */
export function fillFees(text: string, f: FeeSchedule): string {
  const w = feeWords(f);
  return text.replace(/\{fee\.(flat|threshold|percent|cap)\}/g, (_, k: keyof typeof w) => w[k]);
}
