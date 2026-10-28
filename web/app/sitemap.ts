import type { MetadataRoute } from "next";
import { gameHref, sportHref } from "@/lib/format";
import { ARTICLES } from "@/lib/help";
import { API_URL } from "@/lib/api";
import { getCatalog } from "@/lib/catalog";
import { SITE_URL } from "@/lib/seo";

// Built on request (the API isn't reachable during the build); the listing
// fetches below are cached for an hour.
export const dynamic = "force-dynamic";

const esc = (p: string) => SITE_URL + p.replace(/&/g, "&amp;");

/** Active listings, newest first, up to 10,000. */
async function listingUrls(): Promise<MetadataRoute.Sitemap> {
  const out: MetadataRoute.Sitemap = [];
  for (let offset = 0; offset < 10000; offset += 100) {
    try {
      const res = await fetch(`${API_URL}/v1/listings?sort=newest&limit=100&offset=${offset}`, { next: { revalidate: 3600 } });
      if (!res.ok) break;
      const d = (await res.json()) as { listings: { id: string; updated_at: string }[]; total: number };
      for (const l of d.listings) out.push({ url: `${SITE_URL}/item/${l.id}`, lastModified: l.updated_at, changeFrequency: "daily", priority: 0.6 });
      if (offset + 100 >= d.total) break;
    } catch {
      break;
    }
  }
  return out;
}

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const cat = await getCatalog();
  const pages = [
    "/",
    "/search?category=sports",
    "/search?category=tcg",
    ...cat.sports.map((s) => sportHref(s.value)),
    ...cat.games.map((g) => gameHref(g.value)),
    "/help",
    ...ARTICLES.filter((a) => !a.draft).map((a) => `/help/${a.slug}`),
    "/announcements",
    "/site-map",
  ].map((p) => ({ url: esc(p), changeFrequency: "daily" as const, priority: p === "/" ? 1 : 0.8 }));
  return [...pages, ...(await listingUrls())];
}
