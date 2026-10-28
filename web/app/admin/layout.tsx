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
  if (r.status === 403 || r.status === 404) notFound();
  if (r.status >= 500) {
    return (
      <div className="mx-auto max-w-[520px] py-16 text-center">
        <h1 className="text-[22px] font-bold">The back office can&apos;t reach the server</h1>
        <p className="mt-2 text-muted">The marketplace API isn&apos;t answering right now. Try again in a minute.</p>
      </div>
    );
  }
  const s = r.data;
  if (!s || s.mfa !== "ok") return <div className="pt-6">{children}</div>;
  return (
    <div className="pt-4 lg:grid lg:grid-cols-[220px_minmax(0,1fr)] lg:gap-8">
      <AdminNav permissions={s.permissions} role={s.staff_role} name={s.user.username} />
      <div className="min-w-0 pt-2">{children}</div>
    </div>
  );
}
