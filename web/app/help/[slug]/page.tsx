import Link from "next/link";
import { notFound } from "next/navigation";
import type { Metadata } from "next";
import { ARTICLES, article } from "@/lib/help";
import { feeWords, fillFees, getFeeInfo } from "@/lib/catalog";

type Props = { params: Promise<{ slug: string }> };

// Built on request so fee amounts always match the fee schedule in the back office.
export const dynamic = "force-dynamic";

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const a = article((await params).slug);
  if (!a) return { title: "Help" };
  const { current } = await getFeeInfo();
  return { title: a.title, description: fillFees(a.summary, current) };
}

export default async function HelpArticle({ params }: Props) {
  const raw = article((await params).slug);
  if (!raw) notFound();
  const { current, upcoming } = await getFeeInfo();
  const fill = (t: string) => fillFees(t, current);
  const a = {
    ...raw,
    summary: fill(raw.summary),
    body: raw.body.map((b) =>
      "h" in b ? { h: fill(b.h) } : "list" in b ? { list: b.list.map(fill) } : "note" in b ? { note: fill(b.note) } : { p: fill(b.p) }),
  };
  const next = raw.slug === "fees" ? upcoming[0] : undefined;
  const related = ARTICLES.filter((x) => x.section === a.section && x.slug !== a.slug);
  return (
    <div className="grid gap-10 pt-6 lg:grid-cols-[1fr_260px]">
      <article className="max-w-[68ch]">
        <nav aria-label="Breadcrumb" className="text-[13px] text-muted">
          <Link href="/help" className="hover:underline">Help center</Link> <span className="mx-1">›</span> {a.section}
        </nav>
        <h1 className="mt-2 text-[28px] font-bold leading-tight">{a.title}</h1>
        <p className="mt-2 text-[16px] text-muted">{a.summary}</p>
        {next && (() => {
          const w = feeWords(next);
          const on = new Date(next.effective_at).toLocaleDateString("en-US", { month: "long", day: "numeric", year: "numeric" });
          return (
            <p className="mt-4 rounded-md border border-ink px-3 py-2 text-[14px]">
              <strong>Fees change on {on}:</strong> {w.flat} per card up to {w.threshold}, then {w.percent} capped at {w.cap}.
              Sales before then use the fees below.
            </p>
          );
        })()}
        {a.draft && (
          <p className="mt-4 rounded-md border border-line bg-wash px-3 py-2 text-[14px]">This page is still being written.</p>
        )}
        <div className="mt-6 space-y-4 text-[16px] leading-relaxed">
          {a.body.map((b, i) =>
            "h" in b ? <h2 key={i} className="pt-2 text-[19px] font-bold">{b.h}</h2>
            : "list" in b ? (
              <ul key={i} className="list-disc space-y-1.5 pl-6">{b.list.map((li) => <li key={li}>{li}</li>)}</ul>
            )
            : "note" in b ? <p key={i} className="rounded-md bg-wash px-4 py-3 text-[14px]">{b.note}</p>
            : <p key={i}>{b.p}</p>,
          )}
        </div>
      </article>
      <aside>
        <h2 className="text-[14px] font-bold">More in {a.section}</h2>
        <ul className="mt-3 space-y-2 text-[14px]">
          {related.map((r) => <li key={r.slug}><Link href={`/help/${r.slug}`} className="text-link hover:underline">{r.title}</Link></li>)}
        </ul>
        <Link href="/help" className="mt-5 inline-block text-[14px] font-semibold text-link hover:underline">All help topics</Link>
      </aside>
    </div>
  );
}
