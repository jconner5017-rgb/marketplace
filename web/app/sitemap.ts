import type { MetadataRoute } from "next";
import { GAMES, SPORTS, gameHref, sportHref } from "@/lib/format";
import { ARTICLES } from "@/lib/help";

const SITE = (process.env.SITE_URL || "https://slabsnsingles.com").replace(/\/$/, "");

export default function sitemap(): MetadataRoute.Sitemap {
  const paths = [
    "/",
    "/search?category=sports",
    "/search?category=tcg",
    ...SPORTS.map((s) => sportHref(s.value)),
    ...GAMES.map((g) => gameHref(g.value)),
    "/help",
    ...ARTICLES.filter((a) => !a.draft).map((a) => `/help/${a.slug}`),
    "/announcements",
    "/site-map",
  ];
  return paths.map((p) => ({ url: SITE + p.replace(/&/g, "&amp;") }));
}
