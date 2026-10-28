"use client";

import ActionForm from "./ActionForm";
import type { MemberDetail } from "@/lib/admin-types";

const iso = (v: string) => (v ? new Date(v).toISOString() : null);

/** The action panels on a member's page, shown by permission. */
export function MemberActions({ m, permissions, myRole }: { m: MemberDetail; permissions: string[]; myRole: string }) {
  const can = (p: string) => permissions.includes(p);
  const staffTarget = !!m.staff_role;
  const mayTouch = !staffTarget || myRole === "super_admin";
  const statuses: [string, string][] = [["active", "Active"], ["frozen", "Frozen (read-only)"], ["suspended", "Suspended until a date"]];
  if (can("users.ban")) statuses.push(["banned", "Banned"]);
  const box = "rounded-lg border border-line p-4";

  if (!mayTouch) {
    return <p className="rounded-lg bg-wash p-3 text-[14px]">Only a Super Admin can change a staff member&apos;s account.</p>;
  }
  return (
    <>
      {can("users.restrict") && (
        <section className={box}>
          <h2 className="mb-2 font-bold">Account status</h2>
          <ActionForm
            endpoint={`/v1/admin/users/${m.id}/status`}
            fields={[
              { name: "status", label: "Set status to", type: "select", options: statuses, initial: m.status },
              { name: "until", label: "Suspended until", type: "datetime", showIf: (v) => v.status === "suspended" },
              { name: "reason", label: "Reason (saved in the audit log)", type: "textarea", placeholder: "What happened" },
            ]}
            transform={(v) => ({ ...v, until: v.until ? iso(String(v.until)) : undefined })}
            submit="Update status"
            danger
            done="Status updated."
          />
          <p className="mt-2 text-[12px] text-muted">
            Frozen members can browse but not buy, sell, bid or message; their listings are hidden. Suspending or banning also signs them out.
          </p>
        </section>
      )}
      {can("users.restrict") && (
        <section className={box}>
          <h2 className="mb-2 font-bold">Sign out everywhere</h2>
          <ActionForm endpoint={`/v1/admin/users/${m.id}/sign-out`}
            fields={[{ name: "reason", label: "Reason", placeholder: "Possible account takeover" }]}
            submit={`End ${m.counts.sessions ?? 0} session${(m.counts.sessions ?? 0) === 1 ? "" : "s"}`} done="Signed out." compact />
        </section>
      )}
      {can("users.edit") && (
        <section className={box}>
          <h2 className="mb-2 font-bold">Edit profile</h2>
          <ActionForm
            endpoint={`/v1/admin/users/${m.id}`}
            method="PATCH"
            fields={[
              { name: "username", label: "Username", initial: m.username },
              { name: "email", label: "Email", type: "email", initial: m.email, hint: m.owner ? "The owner's email is set on the server." : "Changing it marks the email unverified." },
              { name: "email_verified", label: "Email verified", type: "checkbox", initial: m.email_verified },
              { name: "reason", label: "Reason", placeholder: "Member asked by message" },
            ]}
            transform={(v) => ({
              username: v.username, email: v.email, reason: v.reason,
              ...(v.email_verified !== m.email_verified ? { email_verified: v.email_verified } : {}),
            })}
            submit="Save changes"
          />
        </section>
      )}
      {m.seller && can("sellers.review") && (
        <section className={box}>
          <h2 className="mb-2 font-bold">Seller review</h2>
          <ActionForm
            endpoint={`/v1/admin/sellers/${m.id}/review`}
            fields={[
              { name: "decision", label: "Decision", type: "select", options: [["approve", "Approve"], ["reject", "Reject (hides listings)"], ["pending", "Back to pending"]],
                initial: m.seller.review_status === "approved" ? "approve" : m.seller.review_status === "rejected" ? "reject" : "approve" },
              { name: "note", label: "Note (required to reject)", type: "textarea" },
            ]}
            submit="Save review"
            done="Review saved."
          />
        </section>
      )}
      {staffTarget && can("staff.manage") && (
        <section className={box}>
          <h2 className="mb-2 font-bold">Authenticator</h2>
          <p className="mb-2 text-[13px] text-muted">{m.mfa_enrolled ? "Set up." : "Not set up yet."} Reset it if they lost their phone and backup codes.</p>
          <ActionForm endpoint={`/v1/admin/users/${m.id}/mfa-reset`} fields={[{ name: "reason", label: "Reason", placeholder: "Lost phone" }]}
            submit="Reset authenticator" danger done="Reset. They'll set it up at their next staff sign-in." compact />
        </section>
      )}
    </>
  );
}
