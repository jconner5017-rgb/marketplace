import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import EmailBlastForm from "@/components/admin/EmailBlastForm";

export const dynamic = "force-dynamic";
export const metadata = { title: "Email members" };

type Blast = { id: number; subject: string; audience: string; recipients: number; sent: number; failed: number; queued: number; by: string; created_at: string };
type Data = { enabled: boolean; postal: boolean; broadcasts: Blast[]; audiences: Record<string, number>; queued: number };

const when = new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", hour: "numeric", minute: "2-digit", timeZone: "America/New_York" });

export default async function AdminEmail() {
  await requireStaff("/admin/email", "announce");
  const r = await apiGet<Data>("/v1/admin/email");
  const d = r.data;
  return (
    <div className="max-w-[760px]">
      <h1 className="text-[26px] font-bold">Email members</h1>
      <p className="mt-1 text-muted">
        Sends to members who confirmed their email (or joined before confirmation started) and haven&apos;t unsubscribed.
        Every email carries an unsubscribe link and the mailing address, as US law requires.
      </p>
      {d && !d.enabled && (
        <p className="mt-4 rounded-lg bg-wash p-3 text-[14px]">
          Email isn&apos;t set up yet. Add <code>EMAIL_PROVIDER</code> (postmark or resend), <code>EMAIL_API_KEY</code>,{" "}
          <code>EMAIL_FROM</code> and <code>EMAIL_POSTAL_ADDRESS</code> to the API service in Render.
        </p>
      )}
      {d && d.enabled && !d.postal && (
        <p className="mt-4 rounded-lg bg-wash p-3 text-[14px]">Add <code>EMAIL_POSTAL_ADDRESS</code> in Render before sending: it&apos;s printed at the bottom of every blast.</p>
      )}
      {d && <EmailBlastForm audiences={d.audiences} />}

      <h2 className="mt-10 text-[20px] font-bold">Sent</h2>
      {d && d.queued > 0 && <p className="text-[13px] text-muted">{d.queued} emails waiting to go out (about 200 a minute).</p>}
      {!d || d.broadcasts.length === 0 ? (
        <p className="mt-2 text-muted">Nothing sent yet.</p>
      ) : (
        <table className="mt-3 w-full text-left text-[14px]">
          <thead className="text-[13px] text-muted">
            <tr><th className="py-2">Subject</th><th>To</th><th>Delivered</th><th>By</th><th>When</th></tr>
          </thead>
          <tbody>
            {d.broadcasts.map((b) => (
              <tr key={b.id} className="border-t border-line">
                <td className="py-2 pr-3">{b.subject}</td>
                <td className="pr-3">{b.audience === "sellers" ? "Sellers" : "Everyone"} · {b.recipients}</td>
                <td className="num pr-3">{b.sent} sent{b.queued ? ` · ${b.queued} waiting` : ""}{b.failed ? ` · ${b.failed} failed` : ""}</td>
                <td className="pr-3">{b.by}</td>
                <td className="whitespace-nowrap" suppressHydrationWarning>{when.format(new Date(b.created_at))}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
