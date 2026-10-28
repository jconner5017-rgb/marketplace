import Link from "next/link";
import { notFound } from "next/navigation";
import type { Metadata } from "next";
import { ARTICLES, article } from "@/lib/help";

type Props = { params: Promise<{ slug: string }> };

export function generateStaticParams() {
  return ARTICLES.map((a) => ({ slug: a.slug }));
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const a = article((await params).slug);
  return a ? { title: a.title, description: a.summary } : { title: "Help" };
}

export default async function HelpArticle({ params }: Props) {
  const a = article((await params).slug);
  if (!a) notFound();
  const related = ARTICLES.filter((x) => x.section === a.section && x.slug !== a.slug);
  return (
    <div className="grid gap-10 pt-6 lg:grid-cols-[1fr_260px]">
      <article className="max-w-[68ch]">
        <nav aria-label="Breadcrumb" className="text-[13px] text-muted">
          <Link href="/help" className="hover:underline">Help center</Link> <span className="mx-1">›</span> {a.section}
        </nav>
        <h1 className="mt-2 text-[28px] font-bold leading-tight">{a.title}</h1>
        <p className="mt-2 text-[16px] text-muted">{a.summary}</p>
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
