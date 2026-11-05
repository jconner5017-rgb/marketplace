import Link from "next/link";
import { requireStaff } from "@/lib/staff";
import BlogEditor from "@/components/admin/BlogEditor";

export const dynamic = "force-dynamic";
export const metadata = { title: "New blog post" };

export default async function NewBlogPost() {
  await requireStaff("/admin/blog/new", "announce");
  return (
    <div>
      <Link href="/admin/blog" className="text-[14px] text-link hover:underline">← Blog</Link>
      <h1 className="mb-4 mt-1 text-[26px] font-bold">New post</h1>
      <BlogEditor post={null} />
    </div>
  );
}
