import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { when } from "@/components/admin/Badges";
import type { Post } from "@/lib/blog";

export const dynamic = "force-dynamic";
export const metadata = { title: "Blog" };

export default async function AdminBlog() {
  await requireStaff("/admin/blog", "announce");
  const r = await apiGet<{ posts: Post[] }>("/v1/admin/blog");
  const posts = r.data?.posts ?? [];
  return (
    <div>
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-[26px] font-bold">Blog</h1>
          <p className="text-muted">Guides that bring buyers and sellers in from search engines. Live posts appear at <Link href="/blog" className="text-link underline">/blog</Link> and in the sitemap.</p>
        </div>
        <Link href="/admin/blog/new" className="rounded-full bg-link px-5 py-2 font-semibold text-white hover:bg-link-dark">New post</Link>
      </div>
      <table className="mt-5 w-full text-[14px]">
        <thead>
          <tr className="border-b border-line text-left text-[13px] text-muted">
            <th className="py-2 pr-3 font-semibold">Post</th>
            <th className="py-2 pr-3 font-semibold">Search phrase</th>
            <th className="py-2 pr-3 font-semibold">Status</th>
            <th className="py-2 font-semibold">Published</th>
          </tr>
        </thead>
        <tbody>
          {posts.map((p) => (
            <tr key={p.id} className="border-b border-wash align-top">
              <td className="py-2 pr-3">
                <Link href={`/admin/blog/${p.id}`} className="font-semibold text-link hover:underline">{p.title}</Link>
                <div className="text-[12px] text-muted">/blog/{p.slug} · {p.minutes} min read</div>
              </td>
              <td className="py-2 pr-3 text-muted">{p.keyword || "—"}</td>
              <td className="py-2 pr-3">{p.status === "published" ? <span className="text-ship">Live</span> : <span className="text-muted">Draft</span>}</td>
              <td className="py-2" suppressHydrationWarning>{p.published_at ? when(p.published_at) : "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {posts.length === 0 && <p className="py-8 text-center text-muted">No posts yet.</p>}
    </div>
  );
}
