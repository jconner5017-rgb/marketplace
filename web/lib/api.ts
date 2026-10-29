import "server-only";
import { cookies, headers } from "next/headers";
import type { BidHistory, Facets, Listing, SearchResult, Seller, Summary, User } from "./types";

// The Go API, reached over Render's private network in production.
export const API_URL = (process.env.API_URL || "http://localhost:8080").replace(/\/$/, "");

/** Server-side GET to the API, forwarding the visitor's session cookie. */
export async function apiGet<T>(path: string): Promise<{ status: number; data: T | null; code?: string }> {
  const jar = await cookies();
  const session = jar.get("mp_session");
  const h = await headers();
  const fwd = h.get("x-forwarded-for");
  const ua = h.get("user-agent");
  try {
    const res = await fetch(API_URL + path, {
      headers: {
        ...(session ? { cookie: `mp_session=${session.value}` } : {}),
        ...(fwd ? { "x-forwarded-for": fwd } : {}),
        ...(ua ? { "user-agent": ua.slice(0, 200) } : {}),
      },
      cache: "no-store",
    });
    const json = await res.json().catch(() => null);
    if (!res.ok) return { status: res.status, data: null, code: json?.error?.code };
    return { status: res.status, data: json as T };
  } catch {
    return { status: 503, data: null };
  }
}

/** Server-side POST to the API with the visitor's session. */
export async function apiPost<T>(path: string, body: unknown = {}): Promise<{ status: number; data: T | null }> {
  const jar = await cookies();
  const session = jar.get("mp_session");
  try {
    const res = await fetch(API_URL + path, {
      method: "POST",
      headers: { "content-type": "application/json", ...(session ? { cookie: `mp_session=${session.value}` } : {}) },
      body: JSON.stringify(body),
      cache: "no-store",
    });
    const data = res.ok ? ((await res.json()) as T) : null;
    return { status: res.status, data };
  } catch {
    return { status: 503, data: null };
  }
}

/** Whether buyers can pay yet (CHECKOUT_ENABLED on the API). */
export async function checkoutEnabled(): Promise<boolean> {
  try {
    const res = await fetch(API_URL + "/v1/checkout/status", { next: { revalidate: 30 } });
    if (!res.ok) return false;
    return ((await res.json()) as { enabled: boolean }).enabled === true;
  } catch {
    return false;
  }
}

export async function getMe(): Promise<User | null> {
  const r = await apiGet<{ user: User }>("/v1/me");
  return r.data?.user ?? null;
}

/** The signed-in visitor plus header counts, or null when signed out. */
export async function getSummary(): Promise<Summary | null> {
  return (await apiGet<Summary>("/v1/me/summary")).data;
}

export async function getSeller(): Promise<{ status: number; seller: Seller | null }> {
  const r = await apiGet<{ seller: Seller }>("/v1/seller");
  return { status: r.status, seller: r.data?.seller ?? null };
}

export async function search(qs: string): Promise<SearchResult> {
  const r = await apiGet<SearchResult>("/v1/listings?" + qs);
  return r.data ?? { listings: [], total: 0, limit: 48, offset: 0 };
}

export async function facets(qs: string): Promise<Facets | null> {
  return (await apiGet<Facets>("/v1/listings/facets?" + qs)).data;
}

export async function getListing(id: string, countView = false): Promise<Listing | null> {
  const r = await apiGet<{ listing: Listing }>("/v1/listings/" + encodeURIComponent(id) + (countView ? "?view=1" : ""));
  return r.data?.listing ?? null;
}

export async function getBids(id: string): Promise<BidHistory | null> {
  return (await apiGet<BidHistory>("/v1/listings/" + encodeURIComponent(id) + "/bids")).data;
}
