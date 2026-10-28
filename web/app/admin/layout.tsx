import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { apiGet } from "@/lib/api";
import type { StaffSession } from "@/lib/staff";
import AdminNav from "@/components/admin/AdminNav";

export const dynamic = "force-dynamic";
export const metadata: Metadata = { title: { default: "Admin", template: "%s | Admin" }, robots: { index: false, follow: false } };

export default async function AdminLayout({ children }: { children: React.ReactNode }) {
  const r = await apiGet<StaffSession>("/v1/admin/session");
  // Signed-out visitors and non-staff see a plain 404, so the back office isn't advertised.
  if (r.status === 403 || (r.status !== 401 && !r.data)) notFound();
  const s = r.data;
  if (!s || s.mfa !== "ok") return <div className="pt-6">{children}</div>;
  return (
    <div className="pt-4 lg:grid lg:grid-cols-[220px_minmax(0,1fr)] lg:gap-8">
      <AdminNav permissions={s.permissions} role={s.staff_role} name={s.user.username} />
      <div className="min-w-0 pt-2">{children}</div>
    </div>
  );
}
