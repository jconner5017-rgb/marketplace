"use client";
// One shared copy of the visitor's watchlist IDs, so every heart on a page
// reads from a single request.
import { useSyncExternalStore } from "react";

type State = { loaded: boolean; signedIn: boolean; ids: Set<string> };
let state: State = { loaded: false, signedIn: true, ids: new Set() };
let loading: Promise<void> | null = null;
const subs = new Set<() => void>();

function set(next: State) {
  state = next;
  subs.forEach((f) => f());
}

function load() {
  if (loading) return;
  loading = fetch("/api/v1/watchlist/ids", { headers: { accept: "application/json" } })
    .then(async (r) => {
      if (r.status === 401) return set({ loaded: true, signedIn: false, ids: new Set() });
      const j = r.ok ? await r.json() : null;
      set({ loaded: true, signedIn: true, ids: new Set<string>(j?.ids ?? []) });
    })
    .catch(() => set({ ...state, loaded: true }));
}

export function useWatched(): State {
  return useSyncExternalStore(
    (f) => {
      subs.add(f);
      load();
      return () => subs.delete(f);
    },
    () => state,
    () => state,
  );
}

export async function toggleWatch(id: string): Promise<{ ok: boolean; message?: string }> {
  const on = !state.ids.has(id);
  const ids = new Set(state.ids);
  if (on) ids.add(id);
  else ids.delete(id);
  set({ ...state, ids }); // optimistic
  const r = await fetch(`/api/v1/watchlist/${id}`, { method: on ? "PUT" : "DELETE", headers: { "content-type": "application/json" }, body: "{}" }).catch(() => null);
  if (!r || !r.ok) {
    const back = new Set(state.ids);
    if (on) back.delete(id);
    else back.add(id);
    set({ ...state, ids: back });
    const j = r ? await r.json().catch(() => null) : null;
    return { ok: false, message: j?.error?.message };
  }
  return { ok: true };
}
