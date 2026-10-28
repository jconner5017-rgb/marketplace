import type { MetadataRoute } from "next";

const SITE = (process.env.SITE_URL || "https://slabsnsingles.com").replace(/\/$/, "");

// Filters that turn a search into an endless set of near-identical URLs.
// Plain keyword and category pages (q, category, game) stay crawlable.
const FILTERS = [
  "grader", "min_grade", "condition", "attribute", "min_price", "max_price",
  "sort", "page", "per", "view", "exclude", "mode", "in", "year", "year_min",
  "year_max", "set", "number", "cert", "seller", "discount", "cat",
];

export default function robots(): MetadataRoute.Robots {
  return {
    rules: [
      {
        userAgent: "*",
        disallow: [
          "/api/",
          "/search/advanced",
          ...FILTERS.flatMap((f) => [`/search?${f}=`, `/search?*&${f}=`]),
          "/cart",
          "/watchlist",
          "/messages",
          "/offers",
          "/purchases",
          "/checkout/",
          "/sell",
          "/seller/",
          "/admin/",
          "/signin",
          "/register",
        ],
      },
    ],
    sitemap: `${SITE}/sitemap.xml`,
  };
}
