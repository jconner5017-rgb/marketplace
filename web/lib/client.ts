// Browser-side calls to the API through the same-origin /api proxy.
import type { ApiError } from "./types";

export type Result<T> =
  | { ok: true; data: T }
  | { ok: false; status: number; message: string; fields: Record<string, string> };

export async function send<T>(method: string, path: string, body?: unknown, contentType = "application/json"): Promise<Result<T>> {
  let res: Response;
  try {
    res = await fetch("/api" + path, {
      method,
      headers: { "content-type": contentType },
      body: body === undefined ? (method === "GET" ? undefined : "{}") : typeof body === "string" ? body : JSON.stringify(body),
    });
  } catch {
    return { ok: false, status: 0, message: "Can't reach the marketplace. Check your connection and try again.", fields: {} };
  }
  if (res.status === 204) return { ok: true, data: undefined as T };
  const json = await res.json().catch(() => null);
  if (res.ok) return { ok: true, data: json as T };
  const err = (json as ApiError | null)?.error;
  return {
    ok: false,
    status: res.status,
    message: err?.message ?? `Something went wrong (${res.status}).`,
    fields: err?.fields ?? {},
  };
}

/** Only allow redirects back into this site. */
export function safeNext(next: string | null | undefined, fallback = "/"): string {
  if (!next || !next.startsWith("/") || next.startsWith("//")) return fallback;
  return next;
}
