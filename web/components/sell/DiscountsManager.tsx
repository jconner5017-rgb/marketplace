"use client";

// Marketing > Discounts (2026-11-20): coupons, order discounts and sale
// events. Buyers get the single best deal; the seller pays for it and our fee
// is on what the buyer pays.

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { send } from "@/lib/client";
import { money } from "@/lib/format";
import type { Coupon, Discounts, OrderDiscount, StoreSale } from "@/lib/types";

type Kind = "coupon" | "order" | "sale";

const input = "w-full rounded-lg border border-line px-3 py-2 text-[15px]";
const label = "mb-1 block text-[13px] font-semibold";
const help = "text-[13px] text-muted";
const btn = "rounded-full bg-ink px-5 py-2 text-[14px] font-semibold text-white hover:opacity-90 disabled:opacity-60";

const local = (d: Date) => new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
const when = (iso?: string) => (iso ? new Date(iso).toLocaleString("en-US", { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" }) : "");
const cents = (s: string) => Math.round(Number(s) * 100);

function offText(percent?: number, amount?: number) {
  return amount ? `${money(amount)} off` : `${percent}% off`;
}
function cardsText(games: string[]) {
  return games.length === 0 ? "Every card" : games.join(", ");
}

/** "% off" or "$ off" with its number. */
function AmountPicker({ id, kind, setKind, value, setValue, maxPct }: {
  id: string; kind: "percent" | "amount"; setKind: (k: "percent" | "amount") => void; value: string; setValue: (v: string) => void; maxPct: number;
}) {
  return (
    <div>
      <label htmlFor={id} className={label}>Discount</label>
      <div className="flex gap-2">
        <select aria-label="Percent or dollars" className="rounded-lg border border-line px-2 py-2 text-[15px]" value={kind}
          onChange={(e) => setKind(e.target.value as "percent" | "amount")}>
          <option value="percent">% off</option>
          <option value="amount">$ off</option>
        </select>
        <input id={id} className={input} inputMode="decimal" required value={value} onChange={(e) => setValue(e.target.value)}
          placeholder={kind === "percent" ? `5 to ${maxPct}` : "5.00"} />
      </div>
    </div>
  );
}

/** All cards, or some of the sports & games. */
function CategoryPicker({ all, chosen, setChosen }: { all: Discounts["categories"]; chosen: string[]; setChosen: (g: string[]) => void }) {
  const every = chosen.length === 0;
  const toggle = (g: string) => setChosen(chosen.includes(g) ? chosen.filter((x) => x !== g) : [...chosen, g]);
  const groups: [string, string[]][] = [["Sports", all.sports], ["Trading card games", all.tcg], ["Non-sport", all.nonsport]];
  return (
    <fieldset className="md:col-span-2">
      <legend className={label}>Cards</legend>
      <label className="mr-4 inline-flex items-center gap-2 text-[14px]">
        <input type="radio" checked={every} onChange={() => setChosen([])} /> Every card
      </label>
      <label className="inline-flex items-center gap-2 text-[14px]">
        <input type="radio" checked={!every} onChange={() => !every || setChosen([all.sports[0] ?? all.tcg[0]].filter((x): x is string => !!x))} /> Only some categories
      </label>
      {!every && (
        <div className="mt-2 grid gap-3 rounded-lg border border-line p-3 sm:grid-cols-3">
          {groups.filter(([, list]) => list.length > 0).map(([name, list]) => (
            <div key={name}>
              <p className="mb-1 text-[12px] font-semibold uppercase text-muted">{name}</p>
              {list.map((g) => (
                <label key={g} className="flex items-center gap-2 text-[14px]">
                  <input type="checkbox" checked={chosen.includes(g)} onChange={() => toggle(g)} /> {g}
                </label>
              ))}
            </div>
          ))}
        </div>
      )}
    </fieldset>
  );
}

function CouponForm({ cats, done }: { cats: Discounts["categories"]; done: () => void }) {
  const router = useRouter();
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [kind, setKind] = useState<"percent" | "amount">("percent");
  const [value, setValue] = useState("10");
  const [games, setGames] = useState<string[]>([]);
  const [ends, setEnds] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    const r = await send("POST", "/v1/seller/coupons", {
      code, label: name, games, ends_at: ends ? new Date(ends).toISOString() : null,
      ...(kind === "percent" ? { percent: Math.round(Number(value)) } : { amount_cents: cents(value) }),
    });
    setBusy(false);
    if (!r.ok) return setErr(Object.values(r.fields)[0] ?? r.message);
    done();
    router.refresh();
  }
  return (
    <form onSubmit={save} className="grid gap-3 md:grid-cols-2">
      <div>
        <label htmlFor="c-code" className={label}>Code</label>
        <input id="c-code" className={`${input} font-mono uppercase`} required maxLength={20} value={code} placeholder="SAVE10"
          onChange={(e) => setCode(e.target.value.toUpperCase().replace(/[^A-Z0-9_-]/g, ""))} />
        <p className={help}>What buyers type in their cart. Letters, numbers, - or _.</p>
      </div>
      <div>
        <label htmlFor="c-name" className={label}>Name (just for you)</label>
        <input id="c-name" className={input} maxLength={60} value={name} placeholder="Facebook group" onChange={(e) => setName(e.target.value)} />
      </div>
      <AmountPicker id="c-amt" kind={kind} setKind={setKind} value={value} setValue={setValue} maxPct={50} />
      <div>
        <label htmlFor="c-end" className={label}>Ends (blank = no end)</label>
        <input id="c-end" type="datetime-local" className={input} value={ends} onChange={(e) => setEnds(e.target.value)} />
      </div>
      <CategoryPicker all={cats} chosen={games} setChosen={setGames} />
      <p className={`${help} md:col-span-2`}>Each buyer can use it once. A $ off coupon comes off the cards it covers, never more than half their price.</p>
      {err && <p className="text-[14px] text-slab md:col-span-2" role="alert">{err}</p>}
      <div className="flex gap-3 md:col-span-2">
        <button disabled={busy} className={btn}>{busy ? "Saving…" : "Create coupon"}</button>
        <button type="button" className="text-[14px] text-muted hover:underline" onClick={done}>Cancel</button>
      </div>
    </form>
  );
}

function OrderForm({ done }: { done: () => void }) {
  const router = useRouter();
  const [min, setMin] = useState("20");
  const [range, setRange] = useState<"any" | "upto" | "between">("upto");
  const [lo, setLo] = useState("");
  const [hi, setHi] = useState("5.00");
  const [kind, setKind] = useState<"percent" | "amount">("percent");
  const [value, setValue] = useState("25");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const preview = () => {
    const off = kind === "percent" ? `${value || "?"}% off` : `$${value || "?"} off`;
    const which = range === "upto" ? ` up to $${hi || "?"}` : range === "between" ? ` priced $${lo || "?"} to $${hi || "?"}` : "";
    return `Buy ${min || "?"}+ cards${which}, get ${off} them`;
  };
  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    const r = await send("POST", "/v1/seller/order-discounts", {
      min_cards: Math.round(Number(min)),
      price_min_cents: range === "between" && lo ? cents(lo) : null,
      price_max_cents: range !== "any" && hi ? cents(hi) : null,
      ...(kind === "percent" ? { percent: Math.round(Number(value)) } : { amount_cents: cents(value) }),
    });
    setBusy(false);
    if (!r.ok) return setErr(Object.values(r.fields)[0] ?? r.message);
    done();
    router.refresh();
  }
  return (
    <form onSubmit={save} className="grid gap-3 md:grid-cols-2">
      <div>
        <label htmlFor="o-min" className={label}>At least this many cards</label>
        <input id="o-min" className={input} inputMode="numeric" required value={min} onChange={(e) => setMin(e.target.value.replace(/\D/g, ""))} />
      </div>
      <AmountPicker id="o-amt" kind={kind} setKind={setKind} value={value} setValue={setValue} maxPct={50} />
      <fieldset className="md:col-span-2">
        <legend className={label}>Which cards count</legend>
        <div className="flex flex-wrap items-center gap-4 text-[14px]">
          <label className="inline-flex items-center gap-2"><input type="radio" checked={range === "any"} onChange={() => setRange("any")} /> Any price</label>
          <label className="inline-flex items-center gap-2"><input type="radio" checked={range === "upto"} onChange={() => setRange("upto")} /> Priced up to</label>
          <label className="inline-flex items-center gap-2"><input type="radio" checked={range === "between"} onChange={() => setRange("between")} /> Priced from … to</label>
        </div>
        {range !== "any" && (
          <div className="mt-2 flex max-w-sm items-center gap-2">
            {range === "between" && (
              <><span className="text-muted">$</span><input aria-label="Lowest price" className={input} inputMode="decimal" required value={lo} placeholder="10.00" onChange={(e) => setLo(e.target.value)} /><span className="text-muted">to</span></>
            )}
            <span className="text-muted">$</span>
            <input aria-label="Highest price" className={input} inputMode="decimal" required value={hi} placeholder="5.00" onChange={(e) => setHi(e.target.value)} />
          </div>
        )}
      </fieldset>
      <p className="rounded-lg bg-wash px-3 py-2 text-[14px] md:col-span-2"><b>Buyers see:</b> {preview()} at checkout.</p>
      <p className={`${help} md:col-span-2`}>Only the cards that match count toward the total and get the discount. Never more than half their price.</p>
      {err && <p className="text-[14px] text-slab md:col-span-2" role="alert">{err}</p>}
      <div className="flex gap-3 md:col-span-2">
        <button disabled={busy} className={btn}>{busy ? "Saving…" : "Create order discount"}</button>
        <button type="button" className="text-[14px] text-muted hover:underline" onClick={done}>Cancel</button>
      </div>
    </form>
  );
}

function SaleForm({ cats, done }: { cats: Discounts["categories"]; done: () => void }) {
  const router = useRouter();
  const [name, setName] = useState("");
  const [kind, setKind] = useState<"percent" | "amount">("percent");
  const [value, setValue] = useState("15");
  const [games, setGames] = useState<string[]>([]);
  const [starts, setStarts] = useState("");
  const [ends, setEnds] = useState(local(new Date(Date.now() + 3 * 864e5)));
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    const r = await send("POST", "/v1/seller/store/sales", {
      name, games, starts_at: starts ? new Date(starts).toISOString() : null, ends_at: new Date(ends).toISOString(),
      ...(kind === "percent" ? { percent: Math.round(Number(value)) } : { amount_cents: cents(value) }),
    });
    setBusy(false);
    if (!r.ok) return setErr(Object.values(r.fields)[0] ?? r.message);
    done();
    router.refresh();
  }
  return (
    <form onSubmit={save} className="grid gap-3 md:grid-cols-2">
      <div>
        <label htmlFor="s-name" className={label}>Sale name</label>
        <input id="s-name" className={input} required maxLength={60} value={name} placeholder="Weekend sale" onChange={(e) => setName(e.target.value)} />
      </div>
      <AmountPicker id="s-amt" kind={kind} setKind={setKind} value={value} setValue={setValue} maxPct={50} />
      <div>
        <label htmlFor="s-start" className={label}>Starts (blank = now)</label>
        <input id="s-start" type="datetime-local" className={input} value={starts} onChange={(e) => setStarts(e.target.value)} />
      </div>
      <div>
        <label htmlFor="s-end" className={label}>Ends</label>
        <input id="s-end" type="datetime-local" className={input} required value={ends} onChange={(e) => setEnds(e.target.value)} />
      </div>
      <CategoryPicker all={cats} chosen={games} setChosen={setGames} />
      <p className={`${help} md:col-span-2`}>
        Up to 31 days, one sale at a time. Cards on sale show a SALE badge and the sale price everywhere. Fixed-price listings only; a $ off sale never takes more than half off a card.
      </p>
      {err && <p className="text-[14px] text-slab md:col-span-2" role="alert">{err}</p>}
      <div className="flex gap-3 md:col-span-2">
        <button disabled={busy} className={btn}>{busy ? "Saving…" : "Schedule sale"}</button>
        <button type="button" className="text-[14px] text-muted hover:underline" onClick={done}>Cancel</button>
      </div>
    </form>
  );
}

function saleState(s: StoreSale) {
  const now = Date.now();
  if (s.cancelled_at) return "Ended early";
  if (new Date(s.ends_at).getTime() <= now) return "Ended";
  if (new Date(s.starts_at).getTime() > now) return "Scheduled";
  return "Running";
}

function couponState(c: Coupon) {
  if (!c.active) return "Off";
  if (c.ends_at && new Date(c.ends_at).getTime() <= Date.now()) return "Ended";
  if (new Date(c.starts_at).getTime() > Date.now()) return "Scheduled";
  return "On";
}

export default function DiscountsManager({ data }: { data: Discounts }) {
  const router = useRouter();
  const [menu, setMenu] = useState(false);
  const [form, setForm] = useState<Kind | null>(null);
  const [err, setErr] = useState("");
  const menuRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!menu) return;
    const close = (e: MouseEvent) => { if (!menuRef.current?.contains(e.target as Node)) setMenu(false); };
    document.addEventListener("mousedown", close);
    return () => document.removeEventListener("mousedown", close);
  }, [menu]);

  async function act(method: string, path: string, body?: unknown) {
    setErr("");
    const r = await send(method, path, body);
    if (!r.ok) return setErr(r.message);
    router.refresh();
  }
  const pick = (k: Kind) => { setMenu(false); setForm(k); };
  const titles: Record<Kind, string> = { coupon: "New coupon", order: "New order discount", sale: "New sale event" };

  return (
    <section className="rounded-xl border border-line p-4 md:col-span-2">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="font-bold">Discounts</h2>
          <p className="mt-1 text-[14px] text-muted">Buyers get the single best deal: a sale price, a coupon or an order discount. You pay for it, and our fee is on what the buyer pays.</p>
        </div>
        <div className="relative" ref={menuRef}>
          <button type="button" aria-haspopup="menu" aria-expanded={menu} onClick={() => setMenu(!menu)}
            className="rounded-full bg-link px-5 py-2 text-[14px] font-semibold text-white hover:bg-link-dark">
            Create discount <span aria-hidden>{menu ? "▴" : "▾"}</span>
          </button>
          {menu && (
            <ul role="menu" className="absolute right-0 z-20 mt-2 w-60 overflow-hidden rounded-lg border border-line bg-paper py-1 text-[14px] shadow-lg">
              <li role="none"><button role="menuitem" type="button" className="block w-full px-4 py-2 text-left hover:bg-wash" onClick={() => pick("coupon")}>
                Coupon<span className="block text-[12px] text-muted">A code for % or $ off</span></button></li>
              <li role="none"><button role="menuitem" type="button" className="block w-full px-4 py-2 text-left hover:bg-wash" onClick={() => pick("order")}>
                Order discount<span className="block text-[12px] text-muted">Buy X cards, save at checkout</span></button></li>
              <li role="none"><button role="menuitem" type="button" disabled={!data.has_store}
                className="block w-full px-4 py-2 text-left hover:bg-wash disabled:cursor-not-allowed disabled:opacity-60" onClick={() => pick("sale")}>
                Sale event<span className="block text-[12px] text-muted">{data.has_store ? "Marked-down prices with a SALE badge" : "For business stores"}</span></button></li>
            </ul>
          )}
        </div>
      </div>

      {form && (
        <div className="mt-4 rounded-lg border border-line bg-wash/40 p-4">
          <h3 className="mb-3 font-bold">{titles[form]}</h3>
          {form === "coupon" && <CouponForm cats={data.categories} done={() => setForm(null)} />}
          {form === "order" && <OrderForm done={() => setForm(null)} />}
          {form === "sale" && <SaleForm cats={data.categories} done={() => setForm(null)} />}
        </div>
      )}
      {err && <p className="mt-3 text-[14px] text-slab" role="alert">{err}</p>}

      <div className="mt-5 space-y-6">
        <div>
          <h3 className="text-[15px] font-bold">Coupons</h3>
          {data.coupons.length === 0 ? (
            <p className={`mt-1 ${help}`}>No coupons yet.</p>
          ) : (
            <div className="mt-2 overflow-x-auto">
              <table className="w-full text-left text-[14px]">
                <thead className="text-[12px] text-muted"><tr className="border-b border-line">
                  <th className="py-1.5 pr-3 font-semibold">Code</th><th className="pr-3 font-semibold">Deal</th><th className="pr-3 font-semibold">Cards</th>
                  <th className="pr-3 font-semibold">Ends</th><th className="pr-3 font-semibold">Used</th><th className="pr-3 font-semibold">Status</th><th />
                </tr></thead>
                <tbody>
                  {data.coupons.map((c) => (
                    <tr key={c.id} className="border-b border-wash">
                      <td className="py-2 pr-3"><span className="font-mono font-semibold">{c.code}</span>{c.label && <span className="block text-[12px] text-muted">{c.label}</span>}</td>
                      <td className="pr-3">{offText(c.percent, c.amount_cents)}</td>
                      <td className="pr-3">{cardsText(c.games)}</td>
                      <td className="pr-3">{c.ends_at ? when(c.ends_at) : "No end"}</td>
                      <td className="num pr-3">{c.uses}{c.uses > 0 && <span className="block text-[12px] text-muted">{money(c.discount_cents)} off</span>}</td>
                      <td className="pr-3">{couponState(c)}</td>
                      <td className="text-right">
                        <button type="button" className="text-[13px] font-semibold text-link hover:underline"
                          onClick={() => act("PATCH", `/v1/seller/coupons/${c.id}`, { active: !c.active })}>{c.active ? "Switch off" : "Switch on"}</button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        <div>
          <h3 className="text-[15px] font-bold">Order discounts</h3>
          {data.order_discounts.length === 0 ? (
            <p className={`mt-1 ${help}`}>None yet. Example: 25% off 20+ cards up to $5.</p>
          ) : (
            <ul className="mt-2 divide-y divide-wash text-[14px]">
              {data.order_discounts.map((r: OrderDiscount) => (
                <li key={r.id} className="flex items-center justify-between gap-3 py-2">
                  <span>{r.label}</span>
                  <button type="button" className="text-[13px] font-semibold text-link hover:underline" onClick={() => act("DELETE", `/v1/seller/order-discounts/${r.id}`)}>Remove</button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div>
          <h3 className="text-[15px] font-bold">Sale events</h3>
          {!data.has_store ? (
            <p className={`mt-1 ${help}`}>Sale events are for business stores. <a href="/sell/store" className="text-link hover:underline">Open a free store</a> to run them.</p>
          ) : data.sales.length === 0 ? (
            <p className={`mt-1 ${help}`}>No sales yet.</p>
          ) : (
            <div className="mt-2 overflow-x-auto">
              <table className="w-full text-left text-[14px]">
                <thead className="text-[12px] text-muted"><tr className="border-b border-line">
                  <th className="py-1.5 pr-3 font-semibold">Sale</th><th className="pr-3 font-semibold">Deal</th><th className="pr-3 font-semibold">Cards</th>
                  <th className="pr-3 font-semibold">When</th><th className="pr-3 font-semibold">Status</th><th />
                </tr></thead>
                <tbody>
                  {data.sales.map((s) => {
                    const st = saleState(s);
                    return (
                      <tr key={s.id} className="border-b border-wash">
                        <td className="py-2 pr-3 font-semibold">{s.name}</td>
                        <td className="pr-3">{offText(s.percent, s.amount_cents)}</td>
                        <td className="pr-3">{s.category ? s.category : cardsText(s.games ?? [])}</td>
                        <td className="pr-3">{when(s.starts_at)} – {when(s.ends_at)}</td>
                        <td className="pr-3">{st}</td>
                        <td className="text-right">
                          {(st === "Running" || st === "Scheduled") && (
                            <button type="button" className="text-[13px] font-semibold text-link hover:underline" onClick={() => act("DELETE", `/v1/seller/store/sales/${s.id}`)}>
                              {st === "Running" ? "End now" : "Cancel"}
                            </button>
                          )}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
