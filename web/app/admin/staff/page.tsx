import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { ROLE, StatusBadge, when } from "@/components/admin/Badges";
import ActionForm from "@/components/admin/ActionForm";

export const dynamic = "force-dynamic";
export const metadata = { title: "Staff" };

type Row = { id: string; email: string; username: string; status: string; staff_role: string; last_login: string | null };

const ROLE_OPTIONS: [string, string][] = [
  ["support", "Support Agent"], ["moderator", "Moderator"], ["auditor", "Financial Auditor"], ["super_admin", "Super Admin"],
];

export default async function StaffPage() {
  const s = await requireStaff("/admin/staff", "staff.manage");
  const r = await apiGet<{ staff: Row[] }>("/v1/admin/staff");
  const rows = r.data?.staff ?? [];
  return (
    <div>
      <h1 className="text-[26px] font-bold">Staff and roles</h1>
      <p className="text-muted">Staff sign in with their own member account, then set up an authenticator the first time they open the back office.</p>

      <section className="mt-6 max-w-[520px] rounded-lg border border-line p-4">
        <h2 className="mb-2 font-bold">Add a staff member</h2>
        <ActionForm
          endpoint="/v1/admin/staff"
          fields={[
            { name: "email", label: "Their member email", type: "email", hint: "They need to register on the site first." },
            { name: "role", label: "Role", type: "select", options: ROLE_OPTIONS },
            { name: "reason", label: "Reason", placeholder: "Hired as support" },
          ]}
          submit="Add to staff"
          done="Added."
        />
      </section>

      <ul className="mt-6 space-y-3">
        {rows.map((m) => (
          <li key={m.id} className="grid gap-4 rounded-lg border border-line p-4 md:grid-cols-[minmax(0,1fr)_320px]">
            <div className="text-[14px]">
              <div className="flex flex-wrap items-center gap-2">
                <Link href={`/admin/users/${m.id}`} className="text-[16px] font-semibold text-link hover:underline">{m.username}</Link>
                <span className="rounded-full bg-ink px-2 py-0.5 text-[12px] font-semibold text-white">{ROLE[m.staff_role]}</span>
                {m.status !== "active" && <StatusBadge status={m.status} />}
              </div>
              <p className="text-muted">{m.email} · last sign-in <span suppressHydrationWarning>{when(m.last_login)}</span></p>
            </div>
            {m.id === s.user.id ? (
              <p className="text-[13px] text-muted">That&apos;s you. Another Super Admin can change your role.</p>
            ) : (
              <ActionForm
                endpoint="/v1/admin/staff"
                fields={[
                  { name: "role", label: "Role", type: "select", options: [...ROLE_OPTIONS, ["", "Remove from staff"]], initial: m.staff_role },
                  { name: "reason", label: "Reason", type: "text" },
                ]}
                extra={{ user_id: m.id }}
                submit="Update"
                done="Updated."
                compact
              />
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
