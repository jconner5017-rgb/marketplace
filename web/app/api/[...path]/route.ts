// Same-origin proxy: the browser calls /api/v1/..., this forwards to the Go
// API. Keeping one origin means the session cookie is first-party and the
// API never needs CORS.
import { type NextRequest } from "next/server";

const API_URL = (process.env.API_URL || "http://localhost:8080").replace(/\/$/, "");

const PASS_REQUEST = ["content-type", "cookie", "user-agent", "stripe-signature", "idempotency-key", "authorization"];
const PASS_RESPONSE = ["content-type", "content-disposition", "cache-control", "location"];

async function proxy(req: NextRequest, ctx: { params: Promise<{ path: string[] }> }) {
  const { path } = await ctx.params;
  const target = `${API_URL}/${path.map(encodeURIComponent).join("/")}${req.nextUrl.search}`;

  const headers = new Headers();
  for (const h of PASS_REQUEST) {
    const v = req.headers.get(h);
    if (v) headers.set(h, v);
  }
  const fwd = req.headers.get("x-forwarded-for");
  if (fwd) headers.set("x-forwarded-for", fwd);

  const hasBody = !["GET", "HEAD"].includes(req.method);
  const body = hasBody ? await req.arrayBuffer() : undefined;
  const call = () => fetch(target, { method: req.method, headers, body, redirect: "manual", cache: "no-store" });
  let res: Response;
  try {
    try {
      res = await call();
    } catch (e) {
      // Reads can be repeated: one retry covers the moment a deploy swaps instances.
      if (hasBody) throw e;
      await new Promise((ok) => setTimeout(ok, 1000));
      res = await call();
    }
  } catch {
    return Response.json(
      { error: { code: "api_unreachable", message: "The marketplace service is not responding. Try again in a minute." } },
      { status: 502 },
    );
  }

  const out = new Headers();
  for (const h of PASS_RESPONSE) {
    const v = res.headers.get(h);
    if (v) out.set(h, v);
  }
  for (const c of res.headers.getSetCookie()) out.append("set-cookie", c);
  const payload = res.status === 204 ? null : await res.arrayBuffer();
  return new Response(payload, { status: res.status, headers: out });
}

export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const PATCH = proxy;
export const DELETE = proxy;
export const dynamic = "force-dynamic";
