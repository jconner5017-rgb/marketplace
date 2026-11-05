import type { Metadata } from "next";
import Link from "next/link";
import { apiGet } from "@/lib/api";
import { fillFees, getFeeInfo } from "@/lib/catalog";
import { SITE_URL } from "@/lib/seo";
import type { Post } from "@/lib/blog";

export const dynamic = "force-dynamic";
export const metadata: Metadata = {
  title: "Blog: guides for card buyers and sellers",
  description: "Guides to buying and selling sports cards, graded slabs and trading card game singles online: pricing, grading, shipping and fees.",
  alternates: { canonical: `${SITE_URL}/blog` },
};

const day = (d: string | null) => (d ? new Date(d).toLocaleDateString("en-US", { month: "long", day: "numeric", year: "numeric" }) : "");

export default async function BlogIndex() {
  const [r, { current }] = await Promise.all([apiGet<{ posts: Post[] }>("/v1/blog"), getFeeInfo()]);
  const posts = r.data?.posts ?? [];
  return (
    <div className="max-w-[860px] pb-12 pt-6">
      <h1 className="text-[30px] font-bold">Slabs &amp; Singles blog</h1>
      <p className="mt-1 text-[16px] text-muted">Guides to buying and selling sports cards, graded slabs and trading card game singles.</p>
      {posts.length === 0 ? (
        <p className="py-12 text-center text-muted">No posts yet.</p>
      ) : (
        <ul className="mt-6 divide-y divide-line border-y border-line">
          {posts.map((p) => (
            <li key={p.id} className="py-5">
              <Link href={`/blog/${p.slug}`} className="group block">
                <h2 className="text-[20px] font-bold leading-snug group-hover:underline">{p.title}</h2>
                <p className="mt-1 text-[13px] text-muted">{day(p.published_at)} · {p.minutes} min read</p>
                <p className="mt-2 line-clamp-3 text-[15px]">{fillFees(p.description || p.intro, current)}</p>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
