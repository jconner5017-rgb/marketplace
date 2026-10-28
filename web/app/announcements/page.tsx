import Link from "next/link";
import { apiGet } from "@/lib/api";

export const dynamic = "force-dynamic";
export const metadata = { title: "Announcements" };

type Announcement = { id: number; title: string; body: string; link: string | null; created_at: string };

export default async function Announcements() {
  const r = await apiGet<{ announcements: Announcement[] }>("/v1/announcements");
  const items = r.data?.announcements ?? [];
  return (
    <div className="mx-auto max-w-[760px] pt-8">
      <h1 className="text-[28px] font-bold">Announcements</h1>
      <p className="mt-1 text-muted">News and updates from Slabs &amp; Singles.</p>
      {items.length === 0 ? (
        <p className="py-12 text-center text-muted">No announcements yet.</p>
      ) : (
        <ol className="mt-6 divide-y divide-line border-y border-line">
          {items.map((a) => (
            <li key={a.id} className="py-5">
              <p className="text-[13px] text-muted">
                {new Date(a.created_at).toLocaleDateString("en-US", { month: "long", day: "numeric", year: "numeric" })}
              </p>
              <h2 className="mt-1 text-[18px] font-bold">{a.title}</h2>
              {a.body && <p className="mt-1 whitespace-pre-line">{a.body}</p>}
              {a.link && (
                a.link.startsWith("/")
                  ? <Link href={a.link} className="mt-2 inline-block text-link underline">Read more</Link>
                  : <a href={a.link} rel="noopener noreferrer" className="mt-2 inline-block text-link underline">Read more</a>
              )}
            </li>
          ))}
        </ol>
      )}
    </div>
  );
}
