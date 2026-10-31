import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import EmailPrefs from "@/components/EmailPrefs";

export const dynamic = "force-dynamic";
export const metadata = { title: "Email settings", robots: { index: false } };

export default async function EmailSettings() {
  await requireUser("/account/email");
  const r = await apiGet<{ announcements: boolean; email: string; confirmed: boolean }>("/v1/me/email-prefs");
  const p = r.data;
  return (
    <div className="mx-auto max-w-[560px] pt-8">
      <h1 className="text-[26px] font-bold">Email settings</h1>
      {p ? (
        <>
          <p className="mt-2 text-[15px]">
            Emails go to <strong>{p.email}</strong>{p.confirmed ? " (confirmed)." : " (not confirmed yet)."}
          </p>
          <EmailPrefs announcements={p.announcements} />
          <p className="mt-4 text-[13px] text-muted">
            Account emails, like confirming your address and password resets, always come through.
          </p>
        </>
      ) : (
        <p className="mt-3 text-muted">We couldn&apos;t load your settings. Try again in a moment.</p>
      )}
    </div>
  );
}
