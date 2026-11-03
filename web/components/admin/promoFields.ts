import type { FieldDef } from "./ActionForm";
import { AUDIENCE, type Promo } from "@/lib/promos";

/** The create and edit form share these fields. */
export function promoFields(p?: Promo): FieldDef[] {
  return [
    { name: "code", label: "Code", placeholder: "WELCOME25", initial: p?.code ?? "", hint: "3 to 32 letters, numbers, dashes or underscores. Sellers can type it in any case." },
    { name: "label", label: "Who it's for (staff only)", placeholder: "New individual sellers, or the shop's name", initial: p?.label ?? "" },
    { name: "audience", label: "Account type", type: "select", options: AUDIENCE, initial: p?.audience ?? "any",
      hint: "For your records and reports. To keep a business offer private, make it single use." },
    { name: "fee_free_sales", label: "Fee-free sales per seller", as: "number", placeholder: "25", initial: p ? String(p.fee_free_sales) : "",
      hint: "Each card sold uses one. A 3-card order uses 3." },
    { name: "max_redemptions", label: "How many sellers can use it (blank: unlimited)", as: "number", omitEmpty: true,
      initial: p?.max_redemptions ? String(p.max_redemptions) : "", placeholder: "1 for a single business" },
    { name: "valid_days", label: "Fee-free sales last this many days after redeeming (blank: no limit)", as: "number", omitEmpty: true,
      initial: p?.valid_days ? String(p.valid_days) : "", placeholder: "90" },
    { name: "expires_at", label: "Last day to redeem (blank: no end date)", omitEmpty: true,
      initial: p?.expires_at ? new Intl.DateTimeFormat("en-CA", { timeZone: "America/New_York" }).format(new Date(p.expires_at)) : "",
      placeholder: "2026-12-31", hint: "YYYY-MM-DD. Works through the end of that day, Eastern time." },
    { name: "new_sellers_only", label: "New sellers only (no paid sales yet)", type: "checkbox", initial: p ? p.new_sellers_only : true },
    { name: "active", label: "Active (sellers can redeem it)", type: "checkbox", initial: p ? p.active : true },
  ];
}
