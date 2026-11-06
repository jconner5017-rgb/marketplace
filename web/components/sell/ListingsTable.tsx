"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { send } from "@/lib/client";
import { money, timeLeft } from "@/lib/format";
import type { Listing, StoreCategory } from "@/lib/types";
import { GradeBadge, Photo } from "@/components/CardBits";
import ListingActions from "@/components/ListingActions";
import { StoreCategoryPicker } from "@/components/sell/StoreForms";

type Panel = "" | "price" | "category" | "remove";

/** Runs one request per listing, a few at a time, and counts what worked. */
async function each(ids: string[], run: (id: string) => Promise<{ ok: boolean; message?: string }>) {
  let done = 0;
  const errors: string[] = [];
  const queue = [...ids];
  async function worker() {
    for (let id = queue.shift(); id; id = queue.shift()) {
      const r = await run(id);
      if (r.ok) done++;
      else errors.push(r.message || "Couldn't update one listing.");
    }
  }
  await Promise.all(Array.from({ length: Math.min(6, ids.length) }, worker));
  return { done, errors };
}

/** Seller Hub listings table: big photos, tick boxes, and bulk edits for the ticked rows. */
export default function ListingsTable({ listings, categories }: { listings: Listing[]; categories: StoreCategory[] }) {
  const router = useRouter();
  const [sel, setSel] = useState<Set<string>>(new Set());
  const [anchor, setAnchor] = useState<number | null>(null);
  const [panel, setPanel] = useState<Panel>("");
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState<{ ok: boolean; text: string } | null>(null);
  const [priceMode, setPriceMode] = useState<"set" | "down" | "up">("set");
  const [priceValue, setPriceValue] = useState("");
  const [catValue, setCatValue] = useState("");
  const allRef = useRef<HTMLInputElement>(null);

  // A new page or view of listings starts with nothing ticked.
  const key = listings.map((l) => l.id).join(",");
  useEffect(() => {
    setSel((s) => new Set([...s].filter((id) => key.includes(id))));
  }, [key]);

  const n = sel.size;
  const all = listings.length > 0 && n === listings.length;
  useEffect(() => {
    if (allRef.current) allRef.current.indeterminate = n > 0 && !all;
  }, [n, all]);

  function toggle(i: number, shift: boolean) {
    const id = listings[i].id;
    setSel((prev) => {
      const next = new Set(prev);
      if (shift && anchor !== null) {
        const on = !prev.has(id);
        for (let k = Math.min(anchor, i); k <= Math.max(anchor, i); k++) {
          if (on) next.add(listings[k].id);
          else next.delete(listings[k].id);
        }
      } else if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
    setAnchor(i);
  }

  const chosen = () => listings.filter((l) => sel.has(l.id));

  async function finish(label: string, r: { done: number; errors: string[] }) {
    setBusy(false);
    setPanel("");
    const plural = (k: number) => (k === 1 ? "listing" : "listings");
    if (r.errors.length === 0) {
      setNote({ ok: true, text: `${label} ${r.done} ${plural(r.done)}.` });
      setSel(new Set());
    } else {
      setNote({
        ok: false,
        text: `${label} ${r.done} ${plural(r.done)}. ${r.errors.length} couldn't be changed: ${r.errors[0]}`,
      });
    }
    router.refresh();
  }

  async function patchAll(label: string, body: (l: Listing) => object | null) {
    setBusy(true);
    setNote(null);
    const items = chosen();
    const byId = new Map(items.map((l) => [l.id, l]));
    const r = await each(items.map((l) => l.id), async (id) => {
      const b = body(byId.get(id)!);
      if (!b) return { ok: false, message: "Auctions can't be changed this way." };
      const res = await send("PATCH", `/v1/seller/listings/${id}`, b);
      return { ok: res.ok, message: res.ok ? undefined : res.message };
    });
    await finish(label, r);
  }

  async function applyPrice() {
    const v = Number(priceValue);
    if (!(v > 0)) return setNote({ ok: false, text: priceMode === "set" ? "Enter a price." : "Enter a percent." });
    await patchAll("Repriced", (l) => {
      if (l.format === "auction") return null;
      let cents = l.price_cents;
      if (priceMode === "set") cents = Math.round(v * 100);
      else if (priceMode === "down") cents = Math.round(l.price_cents * (1 - v / 100));
      else cents = Math.round(l.price_cents * (1 + v / 100));
      return { price_cents: Math.max(1, cents) };
    });
    setPriceValue("");
  }

  async function applyCategory() {
    setBusy(true);
    setNote(null);
    const ids = [...sel];
    const res = await send("POST", "/v1/seller/store/categorize", { category_id: catValue ? Number(catValue) : 0, listing_ids: ids });
    await finish("Moved", res.ok ? { done: ids.length, errors: [] } : { done: 0, errors: [res.message] });
  }

  async function removeAll() {
    setBusy(true);
    setNote(null);
    const r = await each([...sel], async (id) => {
      const res = await send("DELETE", `/v1/seller/listings/${id}`);
      return { ok: res.ok, message: res.ok ? undefined : res.message };
    });
    await finish("Removed", r);
  }

  const btn = "rounded-full border border-line bg-paper px-3 py-1.5 font-semibold hover:bg-wash disabled:opacity-50";

  return (
    <div className="mt-3">
      {/* Bulk bar: shows once anything is ticked, and stays in view while scrolling. */}
      {n > 0 && (
        <div className="sticky top-0 z-20 -mx-2 mb-2 rounded-lg border border-link bg-paper px-3 py-2 text-[14px] shadow-md">
          <div className="flex flex-wrap items-center gap-2">
            <span className="mr-1 font-semibold">{n} selected</span>
            <button type="button" disabled={busy} onClick={() => setPanel(panel === "price" ? "" : "price")} className={btn}>Change price</button>
            <button type="button" disabled={busy} onClick={() => patchAll("Turned offers on for", (l) => (l.format === "auction" ? null : { offers: { enabled: true } }))} className={btn}>Offers on</button>
            <button type="button" disabled={busy} onClick={() => patchAll("Turned offers off for", (l) => (l.format === "auction" ? null : { offers: { enabled: false } }))} className={btn}>Offers off</button>
            <button type="button" disabled={busy} onClick={() => patchAll("Hid", () => ({ status: "hidden" }))} className={btn}>Hide</button>
            <button type="button" disabled={busy} onClick={() => patchAll("Showed", () => ({ status: "active" }))} className={btn}>Show</button>
            {categories.length > 0 && (
              <button type="button" disabled={busy} onClick={() => setPanel(panel === "category" ? "" : "category")} className={btn}>Store category</button>
            )}
            <button type="button" disabled={busy} onClick={() => setPanel(panel === "remove" ? "" : "remove")} className={`${btn} text-slab`}>Remove</button>
            <button type="button" onClick={() => { setSel(new Set()); setPanel(""); }} className="ml-auto text-link underline">Clear</button>
          </div>

          {panel === "price" && (
            <form className="mt-2 flex flex-wrap items-center gap-2 border-t border-line pt-2" onSubmit={(e) => { e.preventDefault(); applyPrice(); }}>
              <label htmlFor="bulk-price-mode" className="sr-only">How to change the price</label>
              <select id="bulk-price-mode" value={priceMode} onChange={(e) => setPriceMode(e.target.value as typeof priceMode)} className="rounded-md border border-line bg-paper px-2 py-1.5">
                <option value="set">Set price to $</option>
                <option value="down">Lower price by %</option>
                <option value="up">Raise price by %</option>
              </select>
              <label htmlFor="bulk-price-value" className="sr-only">{priceMode === "set" ? "New price in dollars" : "Percent"}</label>
              <input id="bulk-price-value" inputMode="decimal" value={priceValue} onChange={(e) => setPriceValue(e.target.value.replace(/[^0-9.]/g, ""))}
                placeholder={priceMode === "set" ? "4.99" : "10"} className="w-24 rounded-md border border-line bg-paper px-2 py-1.5 text-[16px] sm:text-[14px]" />
              <button disabled={busy} className="rounded-full bg-link px-4 py-1.5 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
                {busy ? "Saving…" : `Apply to ${n}`}
              </button>
              <span className="text-[12px] text-muted">Auctions are skipped. To run a sale instead, use Marketing.</span>
            </form>
          )}
          {panel === "category" && (
            <div className="mt-2 flex flex-wrap items-center gap-2 border-t border-line pt-2">
              <label htmlFor="bulk-cat" className="sr-only">Store category</label>
              <select id="bulk-cat" value={catValue} onChange={(e) => setCatValue(e.target.value)} className="rounded-md border border-line bg-paper px-2 py-1.5">
                <option value="">No category</option>
                {categories.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
              </select>
              <button type="button" disabled={busy} onClick={applyCategory} className="rounded-full bg-link px-4 py-1.5 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
                {busy ? "Saving…" : `Move ${n}`}
              </button>
            </div>
          )}
          {panel === "remove" && (
            <div className="mt-2 flex flex-wrap items-center gap-2 border-t border-line pt-2">
              <span>Remove {n} {n === 1 ? "listing" : "listings"} from the marketplace? Auctions with bids are kept.</span>
              <button type="button" disabled={busy} onClick={removeAll} className="rounded-full bg-slab px-4 py-1.5 font-semibold text-white disabled:opacity-60">
                {busy ? "Removing…" : `Remove ${n}`}
              </button>
              <button type="button" onClick={() => setPanel("")} className="text-muted underline">Keep them</button>
            </div>
          )}
        </div>
      )}
      {note && (
        <p role="status" className={`mb-2 text-[14px] ${note.ok ? "text-ship" : "text-slab"}`}>{note.text}</p>
      )}

      <div className="overflow-x-auto">
        <table className="w-full min-w-[760px] text-[14px]">
          <thead>
            <tr className="border-b border-line text-left text-[13px] text-muted">
              <th className="w-10 py-2 pr-2">
                <input ref={allRef} type="checkbox" checked={all} aria-label="Select all listings on this page"
                  onChange={() => setSel(all ? new Set() : new Set(listings.map((l) => l.id)))} className="h-[18px] w-[18px] cursor-pointer accent-[var(--color-link)]" />
              </th>
              <th className="py-2 pr-3 font-semibold" colSpan={2}>Card</th>
              <th className="py-2 pr-3 font-semibold">Grade</th>
              <th className="py-2 pr-3 text-right font-semibold">Price</th>
              <th className="py-2 pr-3 text-right font-semibold">Qty</th>
              <th className="py-2 pr-3 font-semibold">Status</th>
              {categories.length > 0 && <th className="py-2 pr-3 font-semibold">Store category</th>}
              <th className="py-2 font-semibold"><span className="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            {listings.map((l, i) => {
              const on = sel.has(l.id);
              return (
                <tr key={l.id} className={`border-b border-wash align-middle ${on ? "bg-link/10" : ""}`}>
                  <td className="py-2 pr-2">
                    <input type="checkbox" checked={on} aria-label={`Select ${l.title}`}
                      onChange={() => {}}
                      onClick={(e) => toggle(i, e.shiftKey)}
                      className="h-[18px] w-[18px] cursor-pointer accent-[var(--color-link)]" />
                  </td>
                  <td className="w-[92px] py-2 pr-3">
                    <Link href={`/item/${l.id}`} aria-hidden="true" tabIndex={-1}>
                      <Photo l={l} className="h-[112px] w-[80px] rounded" />
                    </Link>
                  </td>
                  <td className="py-2 pr-3">
                    <Link href={`/item/${l.id}`} className="title-tight text-[15px] hover:underline">{l.title}</Link>
                    <div className="mt-0.5 text-[12px] text-muted">{l.external_id ? `SKU ${l.external_id} · ` : ""}{l.set_name}</div>
                  </td>
                  <td className="py-2 pr-3"><GradeBadge l={l} /></td>
                  <td className="num py-2 pr-3 text-right font-semibold">
                    {l.sale_price_cents != null && l.sale_percent ? <><span className="text-slab">{money(l.sale_price_cents)}</span><div className="text-[12px] font-normal text-muted"><s>{money(l.price_cents)}</s></div></> : money(l.price_cents)}
                  </td>
                  <td className="num py-2 pr-3 text-right">{l.quantity}</td>
                  <td className="py-2 pr-3">
                    {l.format === "auction" && l.auction ? (
                      <span className={l.auction.state === "live" ? "text-ship" : "text-muted"}>
                        {l.auction.state === "live"
                          ? <>Auction · {l.auction.bid_count} {l.auction.bid_count === 1 ? "bid" : "bids"} · <span suppressHydrationWarning>{timeLeft(l.auction.ends_at)}</span></>
                          : l.auction.state === "won" ? "Sold, awaiting payment" : l.auction.state === "unsold" ? "Ended, unsold" : "Ended"}
                      </span>
                    ) : (
                      <span className={`whitespace-nowrap ${l.status === "active" ? "text-ship" : "text-muted"}`}>
                        {l.status === "active" ? (l.offers_enabled ? "Active · offers on" : "Active") : l.status === "sold_out" ? "Sold out" : l.status === "ended" ? "Ended" : "Hidden"}
                      </span>
                    )}
                  </td>
                  {categories.length > 0 && (
                    <td className="py-2 pr-3"><StoreCategoryPicker listingId={l.id} value={l.store_category_id} categories={categories} /></td>
                  )}
                  <td className="py-2 text-right">
                    <ListingActions id={l.id} status={l.status} auction={l.format === "auction" ? l.auction : undefined} />
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      <p className="mt-2 text-[12px] text-muted">Tip: Shift+click a second box to tick every listing in between.</p>
    </div>
  );
}
