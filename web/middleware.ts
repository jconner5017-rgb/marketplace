import { NextResponse, type NextRequest } from "next/server";

// Crawler tally (2026-11-19): bandwidth jumped to ~2,000 page requests an hour
// around the clock, and Render's logs don't show who is asking. This counts
// page requests by visitor type and writes one summary line to the log every
// 10 minutes ("[visitors] ..."), plus the most common unrecognised user agents.
// Nothing is blocked or changed.

const BOTS: [string, RegExp][] = [
  ["Googlebot", /googlebot|google-inspectiontool|googleother|adsbot-google|mediapartners-google|storebot-google/i],
  ["Bingbot", /bingbot|bingpreview|msnbot/i],
  ["Applebot", /applebot/i],
  ["DuckDuckBot", /duckduckbot/i],
  ["Yandex", /yandex/i],
  ["Baidu", /baiduspider/i],
  ["Facebook", /facebookexternalhit|facebookcatalog|meta-externalagent|meta-externalfetcher/i],
  ["GPTBot/OpenAI", /gptbot|chatgpt-user|oai-searchbot/i],
  ["ClaudeBot", /claudebot|claude-user|claude-searchbot|anthropic/i],
  ["PerplexityBot", /perplexity/i],
  ["CCBot", /ccbot/i],
  ["Bytespider", /bytespider|tiktokspider/i],
  ["Amazonbot", /amazonbot/i],
  ["AhrefsBot", /ahrefs/i],
  ["SemrushBot", /semrush/i],
  ["MJ12bot", /mj12bot/i],
  ["DotBot", /dotbot/i],
  ["PetalBot", /petalbot/i],
  ["DataForSeo", /dataforseo/i],
  ["Uptime/monitor", /uptime|pingdom|statuscake|render\/health|go-http-client|kube-probe/i],
];
const GENERIC_BOT = /bot|crawl|spider|slurp|fetch|scrape|python|curl|wget|httpclient|headless/i;

const WINDOW_MS = 10 * 60 * 1000;
let since = Date.now();
let counts = new Map<string, number>();
let unknown = new Map<string, number>();

function classify(ua: string): string {
  if (!ua) return "no user agent";
  for (const [name, re] of BOTS) if (re.test(ua)) return name;
  if (GENERIC_BOT.test(ua)) return "other bot";
  return "browsers";
}

function flush(now: number) {
  const total = [...counts.values()].reduce((a, b) => a + b, 0);
  if (total > 0) {
    const parts = [...counts.entries()].sort((a, b) => b[1] - a[1]).map(([k, v]) => `${k} ${v}`);
    const top = [...unknown.entries()].sort((a, b) => b[1] - a[1]).slice(0, 5).map(([k, v]) => `${v}x "${k}"`);
    console.log(`[visitors] ${Math.round((now - since) / 60000)} min, ${total} page requests: ${parts.join(", ")}` +
      (top.length ? ` | top other bots: ${top.join("; ")}` : ""));
  }
  since = now;
  counts = new Map();
  unknown = new Map();
}

export function middleware(req: NextRequest) {
  try {
    const ua = req.headers.get("user-agent") ?? "";
    const kind = classify(ua);
    counts.set(kind, (counts.get(kind) ?? 0) + 1);
    if (kind === "other bot" || kind === "no user agent") {
      const key = ua.slice(0, 120);
      if (unknown.size < 200 || unknown.has(key)) unknown.set(key, (unknown.get(key) ?? 0) + 1);
    }
    const now = Date.now();
    if (now - since >= WINDOW_MS) flush(now);
  } catch {
    // never let the tally get in the way of a page
  }
  return NextResponse.next();
}

// Pages only: not static files, images, icons or the API proxy.
export const config = {
  matcher: ["/((?!_next/|api/|favicon|icon|apple-icon|.*\\.(?:png|jpg|jpeg|webp|gif|svg|ico|css|js|txt|xml|woff2?)$).*)"],
};
