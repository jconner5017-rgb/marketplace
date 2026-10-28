import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import type { Message, Thread } from "@/lib/types";
import ReplyForm from "@/components/ReplyForm";
import ReportButton from "@/components/ReportButton";

export const dynamic = "force-dynamic";
export const metadata = { title: "Conversation" };

export default async function ConversationPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  await requireUser(`/messages/${id}`);
  const r = await apiGet<{ thread: Thread; messages: Message[] }>(`/v1/messages/${encodeURIComponent(id)}`);
  if (!r.data) notFound();
  const { thread: t, messages } = r.data;
  return (
    <div className="mx-auto max-w-[760px] pt-6">
      <Link href="/messages" className="text-[14px] text-link hover:underline">‹ All messages</Link>
      <div className="mt-3 flex items-center gap-3 border-b border-line pb-3">
        <div className="h-[70px] w-[50px] shrink-0 overflow-hidden rounded bg-wash">
          {t.listing_photo && (
            // eslint-disable-next-line @next/next/no-img-element
            <img src={t.listing_photo} alt="" className="h-full w-full object-contain" />
          )}
        </div>
        <div>
          <h1 className="text-[20px] font-bold">{t.other_username}</h1>
          {t.listing_id ? (
            <Link href={`/item/${t.listing_id}`} className="title-tight text-[14px] text-link hover:underline">{t.listing_title}</Link>
          ) : (
            <p className="text-[14px] text-muted">{t.listing_title}</p>
          )}
        </div>
      </div>
      <ol className="space-y-3 py-5">
        {messages.map((m) => (
          <li key={m.id} className={`flex ${m.mine ? "justify-end" : "justify-start"}`}>
            <div className={`max-w-[80%] rounded-2xl px-4 py-2.5 ${m.mine ? "bg-link text-white" : "bg-wash"}`}>
              <p className="whitespace-pre-line text-[15px]">{m.body}</p>
              <p className={`mt-1 text-[11px] ${m.mine ? "text-white/75" : "text-muted"}`} suppressHydrationWarning>
                {new Date(m.created_at).toLocaleString("en-US", { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" })}
              </p>
              {!m.mine && <ReportButton type="message" id={String(m.id)} signedIn next={`/messages/${t.id}`} />}
            </div>
          </li>
        ))}
      </ol>
      <ReplyForm threadId={t.id} />
    </div>
  );
}
