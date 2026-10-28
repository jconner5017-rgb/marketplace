import "server-only";
import { notFound, redirect } from "next/navigation";
import { apiGet } from "./api";
import type { User } from "./types";

export type StaffSession = {
  user: User;
  staff_role: "super_admin" | "moderator" | "support" | "auditor";
  permissions: string[];
  mfa: "setup_required" | "verify_required" | "locked" | "ok";
  mfa_until: string | null;
  mfa_configured: boolean;
};

export const ROLE_LABEL: Record<string, string> = {
  super_admin: "Super Admin", moderator: "Moderator", support: "Support Agent", auditor: "Financial Auditor",
};

/** The staff session, or null for non-staff. Signed-out visitors are sent to sign in. */
export async function getStaffSession(next: string): Promise<StaffSession | null> {
  const r = await apiGet<StaffSession>("/v1/admin/session");
  if (r.status === 401) redirect(`/signin?next=${encodeURIComponent(next)}`);
  return r.data;
}

/** For admin pages: staff only, authenticator check passed, and (optionally) a permission. */
export async function requireStaff(next: string, perm?: string): Promise<StaffSession> {
  const s = await getStaffSession(next);
  if (!s) notFound();
  if (s.mfa !== "ok") redirect(`/admin/mfa?next=${encodeURIComponent(next)}`);
  if (perm && !s.permissions.includes(perm)) redirect("/admin?denied=1");
  return s;
}
