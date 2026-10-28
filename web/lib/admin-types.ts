// Shapes returned by the admin API.

export type MemberDetail = {
  id: string; email: string; username: string; status: string; staff_role: string | null; created_at: string;
  is_seller: boolean; tier: number | null; review_status: string | null; active_listings: number; last_login: string | null;
  email_verified: boolean; suspended_until: string | null; status_reason: string | null; status_changed_at: string | null;
  seller: null | {
    stripe_account_id: string; charges_enabled: boolean; details_submitted: boolean; tier: number; review_status: string;
    review_note: string | null; reviewed_at: string | null; bundle_discount_type: string | null; since: string;
  };
  counts: Record<string, number>;
  logins: { at: string; ip: string; user_agent: string; success: boolean; reason: string }[];
  notes: { id: number; author: string; body: string; at: string }[];
  history: { id: number; at: string; actor_email: string; action: string; reason: string; details: Record<string, unknown> }[];
  mfa_enrolled: boolean;
  owner: boolean;
};

export type CaseRow = {
  id: string; number: number; order_id: string; order_number: number; kind: string; status: string; summary: string;
  buyer: string; seller: string; opened_by: string; assigned_to: string; assigned_id: string | null; outcome: string | null;
  total_cents: number; created_at: string; updated_at: string; closed_at: string | null;
};

export type RefundReport = {
  id: string; order_id: string | null; order_number: number | null; source: string; kind: string; amount_cents: number;
  stripe_ref: string | null; note: string; reported_by: string; seller: string; created_at: string;
};

export type FeedbackRow = {
  id: string; order_id: string; order_number: number; from: string; from_id: string; to: string; to_id: string; role: string;
  rating: number; comment: string; reply: string | null; created_at: string; removed_at: string | null; removed_reason: string | null; open_reports: number;
};

export type OrderDetail = import("@/components/admin/OrderBits").OrderRow & {
  subtotal_cents: number; discount_cents: number; ship_to: Record<string, string>; stripe_account: string | null;
  stripe_checkout_id: string | null; stripe_payment_intent: string | null; stripe_charge: string | null; cancel_reason: string | null;
  delivered_at: string | null; cancelled_at: string | null;
  lines: { listing_id: string | null; listing_version: number | null; source: string; title: string; unit_cents: number; quantity: number }[];
  shipments: { id: string; method: string; carrier: string | null; service: string | null; tracking_number: string | null; label_cost_cents: number | null;
    status: string; last_event: string | null; last_event_at: string | null; created_at: string }[];
  refunds: RefundReport[]; cases: CaseRow[]; feedback: FeedbackRow[]; thread_ids: string[];
};

export const CASE_KIND: Record<string, string> = { inr: "Item not received", snad: "Not as described", other: "Other problem" };
export const CASE_STATUS: Record<string, string> = {
  open: "Open", waiting_seller: "Waiting on seller", waiting_buyer: "Waiting on buyer",
  resolved_buyer: "Resolved for buyer", resolved_seller: "Resolved for seller", closed: "Closed",
};
export const REFUND_KIND: Record<string, string> = {
  refund: "Refund", partial_refund: "Partial refund", chargeback: "Chargeback opened", chargeback_won: "Chargeback won by seller", chargeback_lost: "Chargeback lost",
};
