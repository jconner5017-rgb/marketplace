"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { send } from "@/lib/client";
import type { Post } from "@/lib/blog";
import { blocksToText, textToBlocks } from "@/lib/blogText";

const input = "w-full rounded-lg border border-line px-3 py-2 text-[15px]";
const label = "block text-[14px] font-semibold";
const help = "mt-0.5 text-[12px] text-muted";

/** Write or edit a blog post. */
export default function BlogEditor({ post }: { post: Post | null }) {
  const router = useRouter();
  const [v, setV] = useState({
    title: post?.title ?? "", slug: post?.slug ?? "", keyword: post?.keyword ?? "",
    description: post?.description ?? "", intro: post?.intro ?? "", body: post ? blocksToText(post.body) : "",
    status: post?.status ?? "draft",
  });
  const [fields, setFields] = useState<Record<string, string>>({});
  const [msg, setMsg] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const set = (k: keyof typeof v) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => setV({ ...v, [k]: e.target.value });
  const fe = (k: string) => fields[k] && <p role="alert" className="mt-0.5 text-[13px] text-slab">{fields[k]}</p>;

  async function save(status?: "draft" | "published") {
    setBusy(true);
    setErr("");
    setMsg("");
    setFields({});
    const payload = { title: v.title, slug: v.slug, keyword: v.keyword, description: v.description, intro: v.intro,
      body: textToBlocks(v.body), status: status ?? v.status };
    const r = post
      ? await send<{ post: Post }>("PATCH", `/v1/admin/blog/${post.id}`, payload)
      : await send<{ post: Post }>("POST", "/v1/admin/blog", payload);
    setBusy(false);
    if (!r.ok) {
      setFields(r.fields);
      return setErr(r.message);
    }
    setV({ ...v, slug: r.data.post.slug, status: r.data.post.status });
    setMsg(r.data.post.status === "published" ? "Saved and live." : "Saved as a draft.");
    if (!post) router.push(`/admin/blog/${r.data.post.id}`);
    else router.refresh();
  }

  async function remove() {
    if (!post || !window.confirm(`Delete "${post.title}"? This can't be undone.`)) return;
    const r = await send("DELETE", `/v1/admin/blog/${post.id}`);
    if (!r.ok) return setErr(r.message);
    router.push("/admin/blog");
  }

  return (
    <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_300px]">
      <div className="space-y-4">
        <div>
          <label htmlFor="b-title" className={label}>Title</label>
          <input id="b-title" className={input} value={v.title} onChange={set("title")} maxLength={140} />
          <p className={help}>Put the search phrase near the start. {v.title.length}/140</p>
          {fe("title")}
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <div>
            <label htmlFor="b-slug" className={label}>Address</label>
            <div className="flex items-center gap-1 text-[14px]"><span className="text-muted">/blog/</span>
              <input id="b-slug" className={input} value={v.slug} onChange={set("slug")} placeholder="made from the title if blank" /></div>
            <p className={help}>Changing it on a live post breaks links to the old address.</p>
            {fe("slug")}
          </div>
          <div>
            <label htmlFor="b-kw" className={label}>Target search phrase</label>
            <input id="b-kw" className={input} value={v.keyword} onChange={set("keyword")} maxLength={120} placeholder="how to sell tcg singles online" />
            {fe("keyword")}
          </div>
        </div>
        <div>
          <label htmlFor="b-desc" className={label}>Search description</label>
          <textarea id="b-desc" className={`${input} min-h-[64px]`} value={v.description} onChange={set("description")} maxLength={300} />
          <p className={help}>Shown under the title in Google. About 155 characters is best. {v.description.length}/300</p>
          {fe("description")}
        </div>
        <div>
          <label htmlFor="b-intro" className={label}>Introduction</label>
          <textarea id="b-intro" className={`${input} min-h-[90px]`} value={v.intro} onChange={set("intro")} maxLength={2000} />
          {fe("intro")}
        </div>
        <div>
          <label htmlFor="b-body" className={label}>Article</label>
          <textarea id="b-body" className={`${input} min-h-[480px] font-mono text-[13px] leading-relaxed`} value={v.body} onChange={set("body")} spellCheck />
          {fe("body")}
        </div>
        {err && <p role="alert" className="text-[14px] text-slab">{err}</p>}
        {msg && <p role="status" className="text-[14px] text-ship">{msg}</p>}
        <div className="flex flex-wrap items-center gap-2">
          <button disabled={busy} onClick={() => save("published")} className="rounded-full bg-link px-5 py-2 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
            {v.status === "published" ? "Save" : "Publish"}
          </button>
          <button disabled={busy} onClick={() => save("draft")} className="rounded-full border border-ink px-5 py-2 font-semibold hover:bg-wash disabled:opacity-60">
            {v.status === "published" ? "Unpublish (back to draft)" : "Save draft"}
          </button>
          {post && v.status === "published" && <Link href={`/blog/${v.slug}`} target="_blank" className="text-[14px] text-link underline">View post</Link>}
          {post && <button onClick={remove} className="ml-auto text-[14px] text-slab hover:underline">Delete post</button>}
        </div>
      </div>
      <aside className="h-fit rounded-xl bg-wash p-4 text-[13px]">
        <h2 className="text-[14px] font-bold">Formatting</h2>
        <ul className="mt-2 space-y-1.5 font-mono">
          <li>## A heading</li>
          <li>A paragraph. Leave a blank line between paragraphs.</li>
          <li>- A list item</li>
          <li>&gt; A highlighted note</li>
          <li>[Shop cards](/search) optional text</li>
        </ul>
        <h2 className="mt-4 text-[14px] font-bold">Live fees</h2>
        <p className="mt-1">Write <span className="font-mono">{"{fee.flat}"}</span>, <span className="font-mono">{"{fee.threshold}"}</span>, <span className="font-mono">{"{fee.percent}"}</span> or <span className="font-mono">{"{fee.cap}"}</span> and the current fee schedule fills them in.</p>
        <h2 className="mt-4 text-[14px] font-bold">Search tips</h2>
        <ul className="mt-1 list-disc space-y-1 pl-4">
          <li>Use the search phrase in the title, the description, the introduction and one heading.</li>
          <li>Answer the question in the first paragraphs.</li>
          <li>Link to the pages a reader should visit next (shop, sell, help).</li>
          <li>800 to 1,500 words is a good length for a guide.</li>
        </ul>
      </aside>
    </div>
  );
}
