import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import type { Thread } from "@/lib/types";
import { ago } from "@/lib/format";

export const dynamic = "force-dynamic";
export const metadata = { title: "Messages" };

export default async function MessagesPage() {
  await requireUser("/messages");
  const r = await apiGet<{ threads: Thread[] }>("/v1/messages");
  const threads = r.data?.threads ?? [];
  return (
    <div className="mx-auto max-w-[860px] pt-6">
      <h1 className="text-[26px] font-bold">Messages</h1>
      <p className="text-muted">Questions about listings, between you and other members.</p>
      {threads.length === 0 ? (
        <p className="py-14 text-center text-muted">No messages yet. Use “Contact seller” on a listing to ask a question.</p>
      ) : (
        <ul className="mt-4 divide-y divide-line border-y border-line">
          {threads.map((t) => (
            <li key={t.id}>
              <Link href={`/messages/${t.id}`} className={`flex gap-4 px-2 py-3 hover:bg-wash ${t.unread ? "bg-link/5" : ""}`}>
                <div className="h-[70px] w-[50px] shrink-0 overflow-hidden rounded bg-wash">
                  {t.listing_photo && (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img src={t.listing_photo} alt="" className="h-full w-full object-contain" />
                  )}
                </div>
                <div className="min-w-0 flex-1">
                  <p className="flex items-baseline justify-between gap-3">
                    <span className={`text-[15px] ${t.unread ? "font-bold" : "font-semibold"}`}>
                      {t.other_username} <span className="font-normal text-muted">· {t.role === "seller" ? "buyer" : "seller"}</span>
                    </span>
                    <span className="shrink-0 text-[12px] text-muted">{ago(t.last_message_at)}</span>
                  </p>
                  <p className="title-tight truncate text-[13px] text-muted">{t.listing_title}</p>
                  <p className={`truncate text-[14px] ${t.unread ? "font-semibold" : ""}`}>{t.last_message}</p>
                </div>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
