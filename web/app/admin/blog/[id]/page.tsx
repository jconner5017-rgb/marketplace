import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import BlogEditor from "@/components/admin/BlogEditor";
import type { Post } from "@/lib/blog";

export const dynamic = "force-dynamic";
export const metadata = { title: "Edit blog post" };

export default async function EditBlogPost({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  await requireStaff(`/admin/blog/${id}`, "announce");
  const r = await apiGet<{ post: Post }>(`/v1/admin/blog/${encodeURIComponent(id)}`);
  if (!r.data) notFound();
  return (
    <div>
      <Link href="/admin/blog" className="text-[14px] text-link hover:underline">← Blog</Link>
      <h1 className="mb-4 mt-1 text-[26px] font-bold">Edit post</h1>
      <BlogEditor post={r.data.post} />
    </div>
  );
}
