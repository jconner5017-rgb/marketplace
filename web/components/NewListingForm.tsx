"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";
import { CONDITIONS, money, toCents } from "@/lib/format";
import type { FeeSchedule, SiteCatalog } from "@/lib/catalog";
import type { Listing } from "@/lib/types";
import Field from "./Field";
import PhotoUploader from "./sell/PhotoUploader";

const GRADERS = ["PSA", "BGS", "SGC", "CGC", "TAG", "OTHER"];
const LABEL_ESTIMATE: Record<string, number> = { envelope: 0, bubble: 480, box: 650 };

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
export default function NewListingForm({ capCents, catalog, fees, initial }: {
  capCents: number; catalog: SiteCatalog; fees: FeeSchedule; initial?: Listing;
}) {
  const editing = !!initial;
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
  const [category, setCategory] = useState<string>(initial?.category ?? "tcg");
  const [price, setPrice] = useState(initial && initial.format === "fixed" ? dollars(initial.price_cents) : "");
  const [preset, setPreset] = useState(initial?.package_preset ?? "envelope");
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

  const cents = format === "auction" ? toCents(start) : toCents(price);
  const label = LABEL_ESTIMATE[preset] ?? 0;
  const net = cents != null ? cents - FEE(cents) - label : null;

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
      card_number: editing ? s("card_number") : opt("card_number"),
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
    <form onSubmit={submit} noValidate className="mt-6 space-y-8">
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
        <legend className="mb-2 text-[18px] font-bold">The card</legend>
        <Field label="Category" name="category" error={fields.category}>
          <select id="category" name="category" className={select} value={category} onChange={(e) => setCategory(e.target.value)}>
            <option value="tcg">Trading card game</option>
            <option value="sports">Sports card</option>
          </select>
        </Field>
        <Field label={category === "sports" ? "Sport" : "Game"} name="game_or_sport" error={fields.game_or_sport}>
          <select id="game_or_sport" name="game_or_sport" key={category} className={select} defaultValue={initial?.category === category ? initial.game_or_sport : ""}>
            <option value="" disabled>Choose…</option>
            {(category === "sports" ? catalog.sports : catalog.games).map((o) => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
        </Field>
        <Field label="Set" name="set_name" placeholder="Base Set, Topps Chrome…" defaultValue={initial?.set_name} error={fields.set_name} />
        <div className="grid grid-cols-2 gap-4">
          <Field label="Year" name="year" inputMode="numeric" placeholder="1999" defaultValue={initial?.year ?? ""} error={fields.year} />
          <Field label="Card number" name="card_number" placeholder="4/102" defaultValue={initial?.card_number ?? ""} error={fields.card_number} />
        </div>
        <Field
          className="sm:col-span-2"
          label="Title"
          name="title"
          maxLength={200}
          placeholder="Charizard Holo Base Set"
          defaultValue={initial?.title}
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
                <input type="checkbox" name="attributes" value={k} defaultChecked={initial?.attributes.includes(k)} /> {v}
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
            <option value="envelope">Stamped envelope (one raw card up to $20)</option>
            <option value="bubble">Bubble mailer</option>
            <option value="box">Small box</option>
          </select>
        </Field>
        <Field label="Weight (oz)" name="weight_oz" inputMode="decimal" defaultValue={String(initial?.weight_oz ?? 1)} error={fields.weight_oz} />
        {!editing && <Field label="Your SKU (optional)" name="external_id" hint="Your own ID, used by CSV uploads and FireCardManager." error={fields.external_id} />}

        <div className="rounded-lg bg-wash p-4 text-[14px] sm:col-span-2" aria-live="polite">
          <p className="font-semibold">Estimated payout</p>
          {cents != null ? (
            <dl className="num mt-2 grid grid-cols-[1fr_auto] gap-y-1">
              <dt>{format === "auction" ? "If it sells at the starting bid" : "Sale price"}</dt><dd className="text-right">{money(cents)}</dd>
              <dt>Marketplace fee</dt><dd className="text-right">−{money(FEE(cents))}</dd>
              <dt>Shipping label (estimate)</dt><dd className="text-right">{label ? `−${money(label)}` : "Your stamp"}</dd>
              <dt className="font-bold">You receive about</dt><dd className="text-right font-bold">{money(Math.max(0, net ?? 0))}</dd>
            </dl>
          ) : (
            <p className="mt-1 text-muted">Enter a {format === "auction" ? "starting bid" : "price"} to see what you&apos;ll receive.</p>
          )}
          <p className="mt-2 text-[12px] text-muted">Before Stripe&apos;s card processing fee, which comes out of your Stripe balance.</p>
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
