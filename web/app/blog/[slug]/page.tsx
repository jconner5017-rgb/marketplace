import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGet } from "@/lib/api";
import { fillFees, getFeeInfo } from "@/lib/catalog";
import { SITE_URL } from "@/lib/seo";
import type { Block } from "@/lib/help";
import type { Post } from "@/lib/blog";
import Blocks from "@/components/Blocks";

export const dynamic = "force-dynamic";

type Props = { params: Promise<{ slug: string }> };

async function load(slug: string) {
  return (await apiGet<{ post: Post }>(`/v1/blog/${encodeURIComponent(slug)}`)).data?.post ?? null;
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const p = await load((await params).slug);
  if (!p) return { title: "Post not found" };
  const { current } = await getFeeInfo();
  const url = `${SITE_URL}/blog/${p.slug}`;
  const description = fillFees(p.description || p.intro, current).slice(0, 300);
  return {
    title: p.title,
    description,
    keywords: p.keyword ? [p.keyword] : undefined,
    alternates: { canonical: url },
    openGraph: { type: "article", title: p.title, description, url, publishedTime: p.published_at ?? undefined, modifiedTime: p.updated_at },
  };
}

export default async function BlogPost({ params }: Props) {
  const p = await load((await params).slug);
  if (!p) notFound();
  const [{ current }, more] = await Promise.all([getFeeInfo(), apiGet<{ posts: Post[] }>("/v1/blog")]);
  const fill = (t: string) => fillFees(t, current);
  const body: Block[] = p.body.map((b): Block => {
    if ("list" in b) return { list: b.list.map(fill) };
    if ("h" in b) return { h: fill(b.h) };
    if ("note" in b) return { note: fill(b.note) };
    if ("links" in b) return b;
    return { p: fill((b as { p: string }).p) };
  });
  const day = (d: string) => new Date(d).toLocaleDateString("en-US", { month: "long", day: "numeric", year: "numeric" });
  const url = `${SITE_URL}/blog/${p.slug}`;
  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "BlogPosting",
    headline: p.title,
    description: fill(p.description || p.intro),
    datePublished: p.published_at,
    dateModified: p.updated_at,
    mainEntityOfPage: url,
    url,
    author: { "@type": "Organization", name: "Slabs & Singles", url: SITE_URL },
    publisher: { "@type": "Organization", name: "Slabs & Singles", url: SITE_URL },
    ...(p.keyword ? { keywords: p.keyword } : {}),
  };
  const others = (more.data?.posts ?? []).filter((x) => x.id !== p.id).slice(0, 5);
  return (
    <div className="grid gap-10 pb-12 pt-6 lg:grid-cols-[1fr_260px]">
      <article className="max-w-[70ch]">
        <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd).replace(/</g, "\\u003c") }} />
        <nav aria-label="Breadcrumb" className="text-[13px] text-muted">
          <Link href="/blog" className="hover:underline">Blog</Link>
        </nav>
        <h1 className="mt-2 text-[30px] font-bold leading-tight">{p.title}</h1>
        <p className="mt-2 text-[13px] text-muted">
          {p.published_at && <>{day(p.published_at)} · </>}{p.minutes} min read
        </p>
        {p.intro && <p className="mt-5 text-[18px] leading-relaxed">{fill(p.intro)}</p>}
        <div className="mt-5"><Blocks body={body} /></div>
      </article>
      <aside>
        <div className="rounded-xl border border-line p-4">
          <h2 className="text-[15px] font-bold">Buy and sell on Slabs &amp; Singles</h2>
          <p className="mt-1 text-[14px] text-muted">Sports cards and TCG singles up to $1,000. Every card ships free.</p>
          <div className="mt-3 flex flex-col gap-2 text-[14px]">
            <Link href="/search" className="rounded-full bg-link px-4 py-2 text-center font-semibold text-white hover:bg-link-dark">Shop cards</Link>
            <Link href="/sell" className="rounded-full border border-link px-4 py-2 text-center font-semibold text-link hover:bg-wash">Start selling</Link>
          </div>
        </div>
        {others.length > 0 && (
          <>
            <h2 className="mt-6 text-[14px] font-bold">More from the blog</h2>
            <ul className="mt-2 space-y-2 text-[14px]">
              {others.map((o) => <li key={o.id}><Link href={`/blog/${o.slug}`} className="text-link hover:underline">{o.title}</Link></li>)}
            </ul>
          </>
        )}
      </aside>
    </div>
  );
}
