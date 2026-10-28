import { notFound, redirect } from "next/navigation";
import { getStaffSession } from "@/lib/staff";
import { safeNext } from "@/lib/client";
import MfaForm from "@/components/admin/MfaForm";

export const dynamic = "force-dynamic";
export const metadata = { title: "Staff sign-in" };

export default async function MfaPage({ searchParams }: { searchParams: Promise<{ next?: string }> }) {
  const next = safeNext((await searchParams).next, "/admin");
  const s = await getStaffSession(`/admin/mfa?next=${encodeURIComponent(next)}`);
  if (!s) notFound();
  if (s.mfa === "ok") redirect(next.startsWith("/admin") ? next : "/admin");
  return (
    <div className="mx-auto max-w-[460px] pt-4">
      <h1 className="text-[26px] font-bold">{s.mfa === "setup_required" ? "Set up your authenticator" : "Staff sign-in check"}</h1>
      <p className="mt-1 text-muted">Signed in as {s.user.email}.</p>
      {!s.mfa_configured ? (
        <p className="mt-6 rounded-lg border border-slab/40 bg-slab/5 p-4 text-[14px] text-slab">
          Staff sign-in codes aren&apos;t turned on for this server yet (MFA_ENCRYPTION_KEY is missing on the API service).
        </p>
      ) : (
        <MfaForm mode={s.mfa === "setup_required" ? "setup" : s.mfa === "locked" ? "locked" : "verify"} next={next} />
      )}
    </div>
  );
}
