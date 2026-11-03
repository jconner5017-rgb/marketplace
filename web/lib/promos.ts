// Promo codes that give sellers fee-free sales (back office and seller hub).
export type Promo = {
  id: number; code: string; label: string; audience: "any" | "individual" | "business";
  fee_free_sales: number; max_redemptions: number | null; new_sellers_only: boolean;
  expires_at: string | null; valid_days: number | null; active: boolean; created_at: string;
  redemptions: number; sales_used: number;
};
export type Redemption = {
  id: number; code: string; sales_granted: number; sales_used: number; expires_at: string | null;
  redeemed_at: string; seller?: string; remaining?: number;
};

export const AUDIENCE: [string, string][] = [["any", "Anyone"], ["individual", "Individual sellers"], ["business", "Business accounts"]];

/** "2026-12-31" in US Eastern time, for the date fields. */
export function etDate(iso: string | null): string {
  if (!iso) return "";
  return new Intl.DateTimeFormat("en-CA", { timeZone: "America/New_York", year: "numeric", month: "2-digit", day: "2-digit" }).format(new Date(iso));
}

export function niceDate(iso: string | null): string {
  if (!iso) return "";
  return new Date(iso).toLocaleDateString("en-US", { timeZone: "America/New_York", month: "short", day: "numeric", year: "numeric" });
}
