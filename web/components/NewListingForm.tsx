"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { send } from "@/lib/client";
import { CONDITIONS, categoryInfo, money, toCents } from "@/lib/format";
import type { FeeSchedule, SiteCatalog } from "@/lib/catalog";
import type { Listing } from "@/lib/types";
import Field from "./Field";
import PhotoUploader from "./sell/PhotoUploader";

/** What "Identify from photos" fills the form with (see POST /v1/seller/identify). */
type Identified = {
  category: string; game_or_sport: string; title: string; set_name: string; year: number | null; card_number: string;
  graded: boolean; grader: string; grade: number | null; cert_number: string; condition: string; attributes: string[]; notes: string;
};

const GRADERS = ["PSA", "BGS", "SGC", "CGC", "TAG", "OTHER"];
type ShipEstimate = { method: "stamp" | "label"; cost_cents: number; label_cents: number; carrier?: string; service?: string; live: boolean; from_set: boolean; signature: boolean };
/** Stripe's US card fee on a payment, taken from the seller's Stripe balance. */
const stripeFee = (cents: number) => (cents > 0 ? Math.round((cents * 290) / 10000) + 30 : 0);
/** Used until the live quote arrives. */
const FALLBACK: Record<string, number> = { envelope: 82, bubble: 525, box: 700 };

/** What each package choice means; sizes match how labels are bought (internal/shipping ParcelFor). */
const PACKAGES: Record<string, { size: string; adds: string; carrier: string; pack: string }> = {
  envelope: {
    size: "#10 or #6¾ letter envelope",
    adds: "",
    carrier: "USPS First-Class letter with an 82¢ Forever stamp, or a $1.31 nonmachinable stamp for thick cards and very rigid toploaders. Print a tracked envelope from the order for USPS scans.",
    pack: "One raw card in a penny sleeve and a toploader (recommended for thin, non-relic cards). Thick or relic/memorabilia cards ship by envelope at your own risk. No slabs.",
  },
  bubble: {
    size: "7¼ × 5¼ × ¾ in padded mailer",
    adds: "We add 1 oz for the mailer.",
    carrier: "Label bought on the order, usually USPS Ground Advantage, with tracking.",
    pack: "Penny sleeve and toploader (or semi-rigid), taped between two pieces of cardboard. Up to about 10 raw cards.",
  },
  box: {
    size: "8 × 6 × 2 in box",
    adds: "We add 4 oz for the box and padding.",
    carrier: "Label bought on the order, usually USPS Ground Advantage, with tracking. Signature over $750.",
    pack: "For graded slabs, several toploaders or lots over 10 cards. Bubble-wrap each slab so nothing moves when shaken.",
  },
};

type Kind = "amount" | "percent";

/** Reads a threshold: dollars → cents, percent → whole percent. */
function threshold(kind: Kind, v: string): number | null {
  if (!v.trim()) return null;
  if (kind === "percent") {
    const n = Number(v.trim().replace("%", ""));
    return Number.isInteger(n) ? n : null;
  }
  return toCents(v);
}

const dollars = (c: number | null | undefined) => (c == null ? "" : (c / 100).toFixed(2));
const ruleText = (kind: Kind | null | undefined, v: number | null | undefined) => (v == null ? "" : kind === "amount" ? dollars(v) : String(v));

/** The listing form, for a new card or (with `initial`) editing one. */
export default function NewListingForm({ capCents, catalog, fees, initial, prefill, startAsLot = false }: {
  capCents: number; catalog: SiteCatalog; fees: FeeSchedule; startAsLot?: boolean; initial?: Listing;
  /** "Sell one like this": card details copied from another listing (a new listing, not an edit). */
  prefill?: Pick<Listing, "category" | "game_or_sport" | "set_name" | "year" | "card_number" | "title" | "attributes">;
}) {
  const editing = !!initial;
  const seed = initial ?? prefill;
  const a = initial?.auction;
  const os = initial?.offer_settings;
  // Once an auction has bids, only the description and photos can change.
  const locked = !!a && a.bid_count > 0;
  const FEE = (cents: number) =>
    cents <= fees.threshold_cents
      ? Math.min(fees.flat_cents, cents)
      : Math.min(fees.cap_cents, cents, Math.max(fees.flat_cents, Math.floor((cents * fees.percent_bps) / 10000)));
  const router = useRouter();
  const [graded, setGraded] = useState(initial ? initial.grader != null : false);
  const [category, setCategory] = useState<string>(seed?.category ?? "tcg");
  const [price, setPrice] = useState(initial && initial.format === "fixed" ? dollars(initial.price_cents) : "");
  const [preset, setPreset] = useState(initial?.package_preset ?? (startAsLot ? "box" : "envelope"));
  const [isLot, setIsLot] = useState(initial ? initial.lot_count != null : startAsLot);
  const [lotCount, setLotCount] = useState(initial?.lot_count ? String(initial.lot_count) : "");
  const [format, setFormat] = useState<"fixed" | "auction">(initial?.format ?? "fixed");
  const [start, setStart] = useState(dollars(a?.start_cents));
  const [reserve, setReserve] = useState(dollars(a?.reserve_cents));
  const [binPrice, setBinPrice] = useState(dollars(a?.buy_now_cents));
  const [days, setDays] = useState(a?.duration_days ?? 7);
  const [relist, setRelist] = useState(a?.auto_relist ?? false);
  const [offersOn, setOffersOn] = useState(os?.enabled ?? false);
  const [acceptKind, setAcceptKind] = useState<Kind>(os?.accept_kind ?? "percent");
  const [acceptVal, setAcceptVal] = useState(ruleText(os?.accept_kind, os?.accept_value));
  const [declineKind, setDeclineKind] = useState<Kind>(os?.decline_kind ?? "percent");
  const [declineVal, setDeclineVal] = useState(ruleText(os?.decline_kind, os?.decline_value));
  const [photos, setPhotos] = useState<string[]>(initial?.photo_urls ?? []);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [fields, setFields] = useState<Record<string, string>>({});
  const formRef = useRef<HTMLFormElement>(null);
  const [idBusy, setIdBusy] = useState(false);
  const [idMsg, setIdMsg] = useState("");
  const [identified, setIdentified] = useState<Identified | null>(null);

  // "Identify from photos": the server reads the card with AI; the seller checks the result.
  async function identify() {
    setIdBusy(true);
    setIdMsg("");
    const r = await send<{ result: Identified; left_today: number }>("POST", "/v1/seller/identify", { photo_urls: photos.slice(0, 2) });
    setIdBusy(false);
    if (!r.ok) { setIdMsg(r.message); return; }
    const x = r.data.result;
    if (x.category) setCategory(x.category);
    setGraded(x.graded);
    setIdentified(x);
    setIdMsg(`Filled in from your photos. Check every detail before you list.${x.notes ? ` ${x.notes}` : ""} (${r.data.left_today} left today)`);
  }
  // Runs after the category and grading switches have re-drawn their fields.
  useEffect(() => {
    const f = formRef.current;
    if (!identified || !f) return;
    const put = (name: string, v: string | number | null | undefined) => {
      if (v === null || v === undefined || v === "") return;
      const el = f.elements.namedItem(name);
      if (el instanceof HTMLInputElement || el instanceof HTMLSelectElement || el instanceof HTMLTextAreaElement) el.value = String(v);
    };
    put("game_or_sport", identified.game_or_sport);
    put("title", identified.title);
    put("set_name", identified.set_name);
    put("year", identified.year);
    put("card_number", identified.card_number);
    if (identified.graded) {
      put("grader", identified.grader);
      put("grade", identified.grade);
      put("cert_number", identified.cert_number);
    } else {
      put("condition", identified.condition);
    }
    f.querySelectorAll<HTMLInputElement>('input[name="attributes"]').forEach((cb) => { cb.checked = identified.attributes.includes(cb.value); });
    f.querySelector("fieldset")?.scrollIntoView({ behavior: "smooth", block: "start" });
    setIdentified(null);
  }, [identified]);

  const cents = format === "auction" ? toCents(start) : toCents(price);
  const [weight, setWeight] = useState(String(initial?.weight_oz ?? 1));
  const [ship, setShip] = useState<ShipEstimate | null>(null);
  const [thick, setThick] = useState(false);
  const [freeLeft, setFreeLeft] = useState(0);
  useEffect(() => {
    send<{ fee_free_sales_left: number }>("GET", "/v1/seller/promos").then((r) => { if (r.ok) setFreeLeft(r.data.fee_free_sales_left); });
  }, []);
  // Real shipping cost: a stamp, or a label quote from the seller's ship-from address.
  useEffect(() => {
    const t = setTimeout(async () => {
      const q = new URLSearchParams({ preset, weight_oz: String(Number(weight) || 1), graded: String(graded), nonmachinable: String(thick), price_cents: String(cents ?? 0) });
      const res = await send<{ estimate: ShipEstimate }>("GET", `/v1/seller/shipping/estimate?${q}`);
      setShip(res.ok ? res.data.estimate : null);
    }, 400);
    return () => clearTimeout(t);
  }, [preset, weight, graded, thick, cents]);
  const shipCost = ship?.cost_cents ?? (graded && preset === "envelope" ? FALLBACK.box : preset === "envelope" && thick ? 131 : FALLBACK[preset] ?? 0);
  const stripe = cents != null ? stripeFee(cents) : 0;
  const fee = cents != null && freeLeft === 0 ? FEE(cents) : 0;
  const net = cents != null ? cents - fee - stripe - shipCost : null;
  // Over $20, buyers on the seller's trusted list may choose an envelope (up to $100).
  const trustedEnv = cents != null && cents > 2000 && cents <= 10000 && !graded && preset === "bubble";

  async function submit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const s = (k: string) => String(f.get(k) ?? "").trim();
    const opt = (k: string) => s(k) || null;
    setMessage("");
    setFields({});
    if (locked && initial) {
      setBusy(true);
      const res = await send<{ listing: Listing }>("PATCH", `/v1/seller/listings/${initial.id}`, { description: s("description"), photo_urls: photos });
      setBusy(false);
      if (!res.ok) {
        setMessage(res.status === 422 ? "Some details need fixing." : res.message);
        setFields(res.fields);
        return;
      }
      router.push(`/item/${initial.id}`);
      router.refresh();
      return;
    }
    if (cents == null) {
      setFields(format === "auction" ? { "auction.start_cents": "Enter a starting bid like 0.99" } : { price_cents: "Enter a price like 12.50" });
      return;
    }
    const auction = format === "auction" ? {
      start_cents: cents,
      reserve_cents: reserve.trim() ? toCents(reserve) : null,
      buy_now_cents: binPrice.trim() ? toCents(binPrice) : null,
      duration_days: days,
      auto_relist: relist,
    } : null;
    const offers = format === "fixed" && offersOn ? {
      enabled: true,
      accept_kind: acceptVal.trim() ? acceptKind : null,
      accept_value: threshold(acceptKind, acceptVal),
      decline_kind: declineVal.trim() ? declineKind : null,
      decline_value: threshold(declineKind, declineVal),
    } : { enabled: false, accept_kind: null, accept_value: null, decline_kind: null, decline_value: null };
    const body = {
      category: s("category"),
      game_or_sport: s("game_or_sport"),
      set_name: s("set_name"),
      year: s("year") ? Number(s("year")) : null,
      card_number: isLot ? null : editing ? s("card_number") : opt("card_number"),
      lot_count: isLot ? Number(lotCount) || null : editing ? 0 : null,
      title: s("title"),
      description: s("description"),
      condition: graded ? opt("condition") : s("condition"),
      grader: graded ? s("grader") : null,
      grade: graded && s("grade") ? Number(s("grade")) : null,
      cert_number: graded ? (editing ? s("cert_number") : opt("cert_number")) : null,
      ...(editing && !graded && initial?.grader ? { clear_grading: true } : {}),
      attributes: f.getAll("attributes").map(String),
      photo_urls: photos,
      price_cents: cents,
      quantity: format === "auction" ? 1 : Number(s("quantity") || "1"),
      format,
      auction,
      offers,
      package_preset: preset,
      weight_oz: Number(s("weight_oz") || "1"),
      ...(editing ? {} : { external_id: opt("external_id") }),
    };
    setBusy(true);
    const res = initial
      ? await send<{ listing: Listing }>("PATCH", `/v1/seller/listings/${initial.id}`, body)
      : await send<{ listing: Listing }>("POST", "/v1/seller/listings", body);
    setBusy(false);
    if (!res.ok) {
      setMessage(res.status === 422 ? "Some details need fixing." : res.message);
      setFields(res.fields);
      window.scrollTo({ top: 0, behavior: "smooth" });
      return;
    }
    router.push(`/item/${res.data.listing.id}`);
    router.refresh();
  }

  const select = "mt-1 w-full rounded-md border border-line px-3 py-2";
  return (
    <form ref={formRef} onSubmit={submit} noValidate className="mt-6 space-y-8">
      {message && (
        <p role="alert" className="rounded-md border border-slab/40 bg-slab/5 px-3 py-2 text-[14px] text-slab">{message}</p>
      )}

      {locked && (
        <p className="rounded-lg bg-wash p-4 text-[14px]">
          This auction has bids, so you can only add to the description and change the photos. Everything else stays as bidders saw it.
        </p>
      )}
      {!locked && <>
      <fieldset className="grid gap-4 sm:grid-cols-2">
        <legend className="mb-2 text-[18px] font-bold">{isLot ? "The lot" : "The card"}</legend>
        <div className="sm:col-span-2">
          <div className="flex gap-2" role="radiogroup" aria-label="Single card or lot">
            {([["Single card", false], ["Lot (several cards)", true]] as const).map(([label, v]) => (
              <button
                key={label}
                type="button"
                role="radio"
                aria-checked={isLot === v}
                onClick={() => {
                  setIsLot(v);
                  if (v && preset === "envelope") setPreset("box");
                }}
                className={`rounded-full border px-4 py-1.5 text-[14px] font-semibold ${isLot === v ? "border-ink bg-ink text-paper" : "border-line hover:border-ink"}`}
              >
                {label}
              </button>
            ))}
          </div>
          {isLot && (
            <p className="mt-2 text-[13px] text-muted">
              A lot sells several cards together as one item, like an eBay lot. Show every card (or the stack) in the photos and list what&apos;s included in the description.
            </p>
          )}
        </div>
        {!editing && (
          <p className="text-[14px] text-muted sm:col-span-2">
            Tip: add the front and back photos below first, then tap <span className="font-semibold text-ink">Identify from photos</span> to fill this in.
          </p>
        )}
        <Field label="Category" name="category" error={fields.category}>
          <select id="category" name="category" className={select} value={category} onChange={(e) => setCategory(e.target.value)}>
            <option value="tcg">Trading card game</option>
            <option value="sports">Sports card</option>
            <option value="nonsport">Non-sport card</option>
          </select>
        </Field>
        <Field label={categoryInfo(category)?.subject ?? "Game"} name="game_or_sport" error={fields.game_or_sport}>
          <select id="game_or_sport" name="game_or_sport" key={category} className={select} defaultValue={seed?.category === category ? seed.game_or_sport : ""}>
            <option value="" disabled>Choose…</option>
            {(category === "sports" ? catalog.sports : category === "nonsport" ? catalog.nonsport : catalog.games).map((o) => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
        </Field>
        <Field label="Set" name="set_name" placeholder="Base Set, Topps Chrome…" defaultValue={seed?.set_name} error={fields.set_name} />
        <div className="grid grid-cols-2 gap-4">
          <Field label="Year" name="year" inputMode="numeric" placeholder="1999" defaultValue={seed?.year ?? ""} error={fields.year} />
          {isLot ? (
            <Field label="Number of cards" name="lot_count" inputMode="numeric" placeholder="25" value={lotCount}
              onChange={(e) => {
                const v = e.target.value.replace(/[^0-9]/g, "");
                setLotCount(v);
                const n = Number(v);
                if (n >= 2) setWeight(String(Math.max(1, Math.round(n * 0.1 * 10) / 10)));
              }}
              error={fields.lot_count} />
          ) : (
            <Field label="Card number" name="card_number" placeholder="4/102" defaultValue={seed?.card_number ?? ""} error={fields.card_number} />
          )}
        </div>
        <Field
          className="sm:col-span-2"
          label="Title"
          name="title"
          maxLength={200}
          placeholder={isLot ? "25-card 2024 Prizm football base lot" : "Charizard Holo Base Set"}
          defaultValue={seed?.title}
          hint="Buyers search by title. Include the player or card name and anything notable."
          error={fields.title}
        />
      </fieldset>

      <fieldset>
        <legend className="mb-2 text-[18px] font-bold">Grading and condition</legend>
        <div className="flex gap-2" role="radiogroup" aria-label="Graded or raw">
          {[["Raw", false], ["Graded", true]].map(([l, v]) => (
            <button
              key={String(l)}
              type="button"
              role="radio"
              aria-checked={graded === v}
              onClick={() => setGraded(v as boolean)}
              className={`rounded-full border px-4 py-1.5 text-[14px] font-semibold ${graded === v ? "border-ink bg-ink text-white" : "border-line hover:border-ink"}`}
            >
              {String(l)}
            </button>
          ))}
        </div>
        <div className="mt-4 grid gap-4 sm:grid-cols-3">
          {graded ? (
            <>
              <Field label="Grader" name="grader" error={fields.grader}>
                <select id="grader" name="grader" className={select} defaultValue={initial?.grader ?? "PSA"}>
                  {GRADERS.map((g) => <option key={g} value={g}>{g === "OTHER" ? "Other" : g}</option>)}
                </select>
              </Field>
              <Field label="Grade" name="grade" inputMode="decimal" placeholder="9.5" defaultValue={initial?.grade ?? ""} error={fields.grade} hint="1 to 10, in steps of 0.5" />
              <Field label="Cert number" name="cert_number" placeholder="12345678" defaultValue={initial?.cert_number ?? ""} error={fields.cert_number} />
            </>
          ) : (
            <Field label="Condition" name="condition" error={fields.condition}>
              <select id="condition" name="condition" className={select} defaultValue={initial?.condition ?? "NM"}>
                {Object.entries(CONDITIONS).map(([k, v]) => <option key={k} value={k}>{v} ({k})</option>)}
              </select>
            </Field>
          )}
        </div>
        <div className="mt-4">
          <p className="text-[14px] font-semibold">Features</p>
          <div className="mt-2 flex flex-wrap gap-x-5 gap-y-2">
            {Object.entries(catalog.features).map(([k, v]) => (
              <label key={k} className="flex items-center gap-1.5 text-[14px]">
                <input type="checkbox" name="attributes" value={k} defaultChecked={seed?.attributes?.includes(k)} /> {v}
              </label>
            ))}
          </div>
          {fields.attributes && <p className="mt-1 text-[13px] text-slab">{fields.attributes}</p>}
        </div>
      </fieldset>
      </>}

      <fieldset className="space-y-4">
        <legend className="mb-2 text-[18px] font-bold">Photos and description</legend>
        <PhotoUploader value={photos} onChange={setPhotos} error={fields.photo_urls} />
        {!locked && photos.length > 0 && (
          <div className="rounded-lg border border-line bg-wash p-4">
            <button type="button" onClick={identify} disabled={idBusy}
              className="rounded-full bg-link px-4 py-2 text-[14px] font-semibold text-white hover:bg-link-dark disabled:opacity-60">
              {idBusy ? "Identifying…" : "Identify from photos"}
            </button>
            <p role="status" className="mt-2 text-[13px] text-muted">
              {idMsg || "Fills in the card details from your front and back photos with AI. Check them before you list."}
            </p>
          </div>
        )}
        {initial?.description_html && (
          <p className="mb-2 rounded-lg bg-wash p-3 text-[13px] text-muted">
            This listing has a formatted description (sent from FireCardManager). Leave the text below as it is to keep the formatting;
            changing it replaces the formatted version with plain text.
          </p>
        )}
        <Field label="Description (optional)" name="description" error={fields.description}>
          <textarea id="description" name="description" rows={4} maxLength={5000} defaultValue={initial?.description ?? ""} className="mt-1 w-full rounded-md border border-line px-3 py-2" />
        </Field>
      </fieldset>

      {!locked && (
      <fieldset className="grid gap-4 sm:grid-cols-2">
        <legend className="mb-2 text-[18px] font-bold">Format, price and shipping</legend>
        <div className="sm:col-span-2">
          <div className="flex gap-2" role="radiogroup" aria-label="Selling format">
            {([["fixed", "Buy It Now"], ["auction", "Auction"]] as const).map(([v, l]) => (
              <button key={v} type="button" role="radio" aria-checked={format === v} onClick={() => setFormat(v)} disabled={editing && format !== v}
                className={`rounded-full border px-4 py-1.5 text-[14px] font-semibold ${format === v ? "border-ink bg-ink text-white" : "border-line hover:border-ink"}`}>
                {l}
              </button>
            ))}
          </div>
          <p className="mt-1 text-[12px] text-muted">
            {editing ? "The format can't change. To switch, remove this listing and list the card again. " : ""}
            {format === "auction"
              ? "Bidders compete for one card over 5, 7 or 10 days. Once someone bids, you can only add to the description and photos."
              : "A fixed price. Turn on offers to let buyers suggest a lower price."}
          </p>
        </div>
        {format === "fixed" ? (
          <>
            <Field
              label="Price"
              name="price"
              inputMode="decimal"
              placeholder="12.50"
              value={price}
              onChange={(e) => setPrice(e.target.value)}
              hint={`Free shipping is included. Your limit is ${money(capCents)}.`}
              error={fields.price_cents}
            />
            <Field label="Quantity" name="quantity" inputMode="numeric" defaultValue={String(initial?.quantity ?? 1)} error={fields.quantity} />
            <div className="rounded-lg border border-line p-4 sm:col-span-2">
              <label className="flex items-center gap-2 text-[14px] font-semibold">
                <input type="checkbox" checked={offersOn} onChange={(e) => setOffersOn(e.target.checked)} /> Accept offers
              </label>
              {offersOn && (
                <div className="mt-3 grid gap-4 sm:grid-cols-2">
                  <Rule title="Automatically accept offers of at least" kind={acceptKind} setKind={setAcceptKind}
                    value={acceptVal} setValue={setAcceptVal} error={fields["offers.accept"]} id="accept" />
                  <Rule title="Automatically decline offers below" kind={declineKind} setKind={setDeclineKind}
                    value={declineVal} setValue={setDeclineVal} error={fields["offers.decline"]} id="decline" />
                  <p className="text-[12px] text-muted sm:col-span-2">
                    Leave either blank to decide yourself. Offers in between wait for you to accept, decline or counter within 48 hours.
                    Buyers never see these amounts.
                  </p>
                </div>
              )}
              {fields.offers && <p className="mt-1 text-[13px] text-slab">{fields.offers}</p>}
            </div>
          </>
        ) : (
          <>
            <Field label="Starting bid" name="start" inputMode="decimal" placeholder="0.99" value={start}
              onChange={(e) => setStart(e.target.value)} hint={`Your limit is ${money(capCents)}.`} error={fields["auction.start_cents"]} />
            <Field label="Auction length" name="duration" error={fields["auction.duration_days"]}>
              <select id="duration" value={days} onChange={(e) => setDays(Number(e.target.value))} className={select}>
                {[5, 7, 10].map((d) => <option key={d} value={d}>{d} days</option>)}
              </select>
            </Field>
            <Field label="Reserve price (optional)" name="reserve" inputMode="decimal" placeholder="Hidden minimum" value={reserve}
              onChange={(e) => setReserve(e.target.value)} hint="The card won't sell below this. Buyers only see whether it's met." error={fields["auction.reserve_cents"]} />
            <Field label="Buy It Now price (optional)" name="bin" inputMode="decimal" placeholder="Ends the auction" value={binPrice}
              onChange={(e) => setBinPrice(e.target.value)} hint="Available until the first bid." error={fields["auction.buy_now_cents"]} />
            <label className="flex items-center gap-2 text-[14px] sm:col-span-2">
              <input type="checkbox" checked={relist} onChange={(e) => setRelist(e.target.checked)} /> Relist once automatically if it doesn&apos;t sell
            </label>
            <p className="text-[12px] text-muted sm:col-span-2">
              A bid in the last 2 minutes adds 2 minutes, so nobody wins by sniping. If bidding reaches your limit of {money(capCents)}, the auction ends and that bidder wins.
            </p>
          </>
        )}
        <Field label="Package" name="package_preset" error={fields.package_preset}>
          <select id="package_preset" value={preset} onChange={(e) => setPreset(e.target.value)} className={select}>
            {!isLot && <option value="envelope">Stamped envelope (one raw card up to $20; up to $100 for trusted buyers)</option>}
            <option value="bubble">Bubble mailer (7¼ × 5¼ in)</option>
            <option value="box">Small box (8 × 6 × 2 in)</option>
          </select>
        </Field>
        <Field label="Weight (oz)" name="weight_oz" inputMode="decimal" value={weight} onChange={(e) => setWeight(e.target.value)} error={fields.weight_oz}
          hint={isLot ? "All the cards before packing. We estimate about 0.1 oz a raw card; you can change the box and weight when you buy the label." : preset === "envelope" ? "The card and its sleeve." : "Each card as packed (a raw card in a toploader is about 1 oz, a slab about 3–4 oz)."} />
        {PACKAGES[preset] && (
          <div className="rounded-lg border border-line p-3 text-[13px] sm:col-span-2">
            <p><span className="font-semibold">Size:</span> {PACKAGES[preset].size}{PACKAGES[preset].adds ? `. ${PACKAGES[preset].adds}` : ""}</p>
            <p className="mt-1"><span className="font-semibold">Shipping:</span> {PACKAGES[preset].carrier}{ship?.method === "label" ? ` About ${money(ship.cost_cents)} for one card.` : ""}</p>
            <p className="mt-1"><span className="font-semibold">Packing:</span> {PACKAGES[preset].pack}</p>
            {preset === "envelope" && !graded && (
              <label className="mt-2 flex items-center gap-2">
                <input type="checkbox" checked={thick} onChange={(e) => setThick(e.target.checked)} />
                Thick card or very rigid toploader: I&apos;ll use a $1.31 nonmachinable stamp
              </label>
            )}
            <p className="mt-1 text-muted">Graded cards and orders of two or more cards always go by label; an order with any box item ships in a box.{" "}
              <a href="/help/packing" target="_blank" className="text-link underline">Packing guide</a></p>
          </div>
        )}
        {!editing && <Field label="Your SKU (optional)" name="external_id" hint="Your own ID, used by CSV uploads and FireCardManager." error={fields.external_id} />}

        <div className="rounded-lg bg-wash p-4 text-[14px] sm:col-span-2" aria-live="polite">
          <p className="font-semibold">Estimated payout</p>
          {cents != null ? (
            <dl className="num mt-2 grid grid-cols-[1fr_auto] gap-y-1">
              <dt>{format === "auction" ? "If it sells at the starting bid" : "Sale price"}</dt><dd className="text-right">{money(cents)}</dd>
              <dt>Marketplace fee{freeLeft > 0 && <span className="text-ship"> (promo: {freeLeft} fee-free {freeLeft === 1 ? "sale" : "sales"} left)</span>}</dt>
              <dd className="text-right">{freeLeft > 0 ? <><s className="text-muted">{money(FEE(cents))}</s> $0.00</> : `−${money(fee)}`}</dd>
              <dt>Card processing (Stripe, 2.9% + 30¢)</dt><dd className="text-right">−{money(stripe)}</dd>
              <dt>{preset !== "envelope" || graded
                ? `Shipping label${ship?.service ? ` (${ship.carrier ? ship.carrier + " " : ""}${ship.service})` : " (estimate)"}`
                : thick ? "Nonmachinable stamp" : "Forever stamp"}</dt><dd className="text-right">−{money(shipCost)}</dd>
              <dt className="font-bold">You receive about</dt><dd className="text-right font-bold">{money(Math.max(0, net ?? 0))}</dd>
              {trustedEnv && (
                <>
                  <dt className="mt-1 border-t border-line pt-1 text-muted">To a trusted buyer who picks an envelope (82¢ stamp)</dt>
                  <dd className="mt-1 border-t border-line pt-1 text-right text-muted">{money(Math.max(0, (cents ?? 0) - fee - stripe - 82))}</dd>
                </>
              )}
            </dl>
          ) : (
            <p className="mt-1 text-muted">Enter a {format === "auction" ? "starting bid" : "price"} to see what you&apos;ll receive.</p>
          )}
          <p className="mt-2 text-[12px] text-muted">
            {preset === "envelope" && !graded
              ? (thick ? "Shipping is a $1.31 nonmachinable stamp (82¢ Forever rate plus USPS's 49¢ nonmachinable surcharge)." : "Shipping is one 82¢ Forever stamp.") + " Stripe's fee comes out of your Stripe balance."
              : ship?.live
                ? `Label quoted from ${ship.from_set ? "your ship-from address" : "the middle of the US (add your ship-from address under Sell → Shipping for your own quote)"} to the far side of the country, including 2.9% + 30¢ card processing on the label. Nearer buyers cost less.`
                : "Typical label price, including card processing on the label. Stripe's fee comes out of your Stripe balance."}
          </p>
        </div>
      </fieldset>
      )}

      <div className="flex gap-3">
        <button type="submit" disabled={busy} className="rounded-full bg-link px-8 py-2.5 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
          {busy ? (editing ? "Saving…" : "Publishing…") : editing ? "Save changes" : "Publish listing"}
        </button>
        <button type="button" onClick={() => router.push("/sell")} className="rounded-full px-5 py-2.5 font-semibold text-link hover:bg-wash">
          Cancel
        </button>
      </div>
    </form>
  );
}

function Rule({ title, kind, setKind, value, setValue, error, id }: {
  title: string; kind: Kind; setKind: (k: Kind) => void; value: string; setValue: (v: string) => void; error?: string; id: string;
}) {
  return (
    <div>
      <label htmlFor={`rule-${id}`} className="block text-[14px] font-semibold">{title}</label>
      <div className="mt-1 flex gap-2">
        <input id={`rule-${id}`} inputMode="decimal" value={value} onChange={(e) => setValue(e.target.value)}
          placeholder={kind === "percent" ? "90" : "12.00"}
          className={`min-w-0 flex-1 rounded-md border px-3 py-2 ${error ? "border-slab" : "border-line"}`} />
        <select aria-label="Dollars or percent" value={kind} onChange={(e) => setKind(e.target.value as Kind)} className="rounded-md border border-line px-2">
          <option value="percent">% of price</option>
          <option value="amount">dollars</option>
        </select>
      </div>
      {error && <p className="mt-1 text-[13px] text-slab">{error}</p>}
    </div>
  );
}
