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

/** The fee schedule in force (cached 5 minutes). */
export async function getFees(): Promise<FeeSchedule> {
  try {
    const res = await fetch(API_URL + "/v1/fees", { next: { revalidate: 300 } });
    if (res.ok) return ((await res.json()) as { current: FeeSchedule }).current;
  } catch {}
  return { flat_cents: 150, threshold_cents: 10000, percent_bps: 300, cap_cents: 2500 };
}
