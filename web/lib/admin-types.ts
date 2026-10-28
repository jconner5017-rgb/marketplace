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
