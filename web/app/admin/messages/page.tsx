import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { when } from "@/components/admin/Badges";

export const dynamic = "force-dynamic";
export const metadata = { title: "Message logs" };

type Thread = {
  id: string; listing_id: string | null; listing_title: string; buyer: string; buyer_id: string; seller: string; seller_id: string;
  messages: number; blocked: number; last_message_at: string;
};

export default async function MessagesPage({ searchParams }: { searchParams: Promise<{ user?: string; blocked?: string }> }) {
  await requireStaff("/admin/messages", "messages.view");
  const sp = await searchParams;
  const qs = new URLSearchParams();
  if (sp.user) qs.set("user", sp.user);
  if (sp.blocked) qs.set("blocked", "1");
  const r = await apiGet<{ threads: Thread[] }>(`/v1/admin/messages?${qs}`);
  const rows = r.data?.threads ?? [];
  return (
    <div>
      <h1 className="text-[26px] font-bold">Message logs</h1>
      <p className="text-muted">Conversations between buyers and sellers. Opening one asks for a reason and is recorded in the audit log.</p>
      <form action="/admin/messages" className="mt-4 flex flex-wrap items-end gap-3 text-[14px]">
        <label><span className="block text-[13px] font-semibold">Member</span>
          <input name="user" defaultValue={sp.user} placeholder="Username or member ID" className="mt-1 w-56 rounded-md border border-line px-3 py-2" /></label>
        <label className="flex items-center gap-1.5 pb-2"><input type="checkbox" name="blocked" value="1" defaultChecked={!!sp.blocked} /> Only with blocked messages</label>
        <button className="rounded-full bg-link px-5 py-2 font-semibold text-white hover:bg-link-dark">Search</button>
      </form>
      <ul className="mt-4 divide-y divide-line border-y border-line">
        {rows.map((t) => (
          <li key={t.id} className="flex flex-wrap items-center gap-3 py-3 text-[14px]">
            <div className="min-w-0 flex-1">
              <p className="font-semibold">{t.listing_title}</p>
              <p className="text-muted">
                <Link href={`/admin/users/${t.buyer_id}`} className="underline">{t.buyer}</Link> (buyer) and{" "}
                <Link href={`/admin/users/${t.seller_id}`} className="underline">{t.seller}</Link> (seller) · {t.messages} messages
                {t.blocked > 0 && <span className="font-semibold text-slab"> · {t.blocked} blocked</span>} · <span suppressHydrationWarning>{when(t.last_message_at)}</span>
              </p>
            </div>
            <Link href={`/admin/messages/${t.id}`} className="rounded-full border border-ink px-4 py-1.5 text-[13px] font-semibold hover:bg-wash">Open</Link>
          </li>
        ))}
      </ul>
      {rows.length === 0 && <p className="py-10 text-center text-muted">No conversations match.</p>}
    </div>
  );
}
