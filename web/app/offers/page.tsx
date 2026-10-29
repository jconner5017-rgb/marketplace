import Link from "next/link";
import BuyerRecord from "@/components/feedback/BuyerRecord";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import type { Offer } from "@/lib/types";
import { ago, money, timeLeft } from "@/lib/format";
import OfferRespond from "@/components/OfferRespond";

export const dynamic = "force-dynamic";
export const metadata = { title: "Offers", robots: { index: false } };

const STATUS: Record<Offer["status"], string> = {
  pending: "Waiting for a reply", accepted: "Accepted", declined: "Declined", countered: "Countered", expired: "Expired",
};

export default async function OffersPage({ searchParams }: { searchParams: Promise<{ side?: string }> }) {
  const side = (await searchParams).side === "selling" ? "selling" : "buying";
  await requireUser(`/offers?side=${side}`);
  const r = await apiGet<{ offers: Offer[] }>(`/v1/offers?side=${side}`);
  const offers = r.data?.offers ?? [];
  const waiting = offers.filter((o) => o.can_respond);
  const rest = offers.filter((o) => !o.can_respond);

  const tab = (v: string, label: string) => (
    <Link href={`/offers?side=${v}`} aria-current={side === v ? "page" : undefined}
      className={`border-b-2 px-1 pb-2 font-semibold ${side === v ? "border-ink" : "border-transparent text-muted hover:text-ink"}`}>
      {label}
    </Link>
  );

  return (
    <div className="pt-6">
      <h1 className="text-[26px] font-bold">Offers</h1>
      <nav className="mt-3 flex gap-6 border-b border-line text-[15px]" aria-label="Offers">
        {tab("buying", "Offers you made")}
        {tab("selling", "Offers on your listings")}
      </nav>

      {offers.length === 0 && (
        <p className="py-12 text-center text-muted">
          {side === "buying" ? "You haven't made any offers yet. Look for “Make offer” on Buy It Now listings." : "No offers on your listings yet. Turn on “Accept offers” when you list a card."}
        </p>
      )}
      {waiting.length > 0 && (
        <section className="mt-6">
          <h2 className="text-[18px] font-bold">Waiting for you ({waiting.length})</h2>
          <ul>{waiting.map((o) => <OfferRow key={o.id} o={o} side={side} />)}</ul>
        </section>
      )}
      {rest.length > 0 && (
        <section className="mt-6">
          <h2 className="text-[18px] font-bold">{waiting.length ? "Everything else" : "Your offers"}</h2>
          <ul>{rest.map((o) => <OfferRow key={o.id} o={o} side={side} />)}</ul>
        </section>
      )}
    </div>
  );
}

function OfferRow({ o, side }: { o: Offer; side: string }) {
  const who = side === "buying" ? `Seller: ${o.seller}` : `Buyer: ${o.buyer}`;
  const byYou = (side === "buying") === (o.from_role === "buyer");
  return (
    <li className="flex gap-4 border-b border-line py-4">
      <Link href={`/item/${o.listing_id}`} className="shrink-0">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img src={o.listing_photo} alt="" className="h-[98px] w-[70px] rounded bg-wash object-contain" />
      </Link>
      <div className="min-w-0 flex-1 text-[14px]">
        <Link href={`/item/${o.listing_id}`} className="title-tight text-[16px] hover:underline">{o.listing_title}</Link>
        <p className="text-muted">{who} · listed at {money(o.price_cents)}</p>
        <p className="mt-1">
          <span className="font-semibold">{byYou ? "Your" : o.from_role === "seller" ? "Seller's" : "Buyer's"} {o.from_role === "seller" ? "counteroffer" : "offer"}:</span>{" "}
          <span className="num font-bold">{money(o.amount_cents)}</span>
          {o.quantity > 1 && <span className="text-muted"> each × {o.quantity}</span>}
        </p>
        {o.message && <p className="mt-1 italic text-muted">“{o.message}”</p>}
        <p className={`mt-1 ${o.status === "accepted" ? "font-semibold text-ship" : o.status === "pending" ? "" : "text-muted"}`}>
          {STATUS[o.status]}
          {o.decided_by === "auto" && " automatically"}
          {o.status === "pending" && <span className="text-muted"> · <span suppressHydrationWarning>{timeLeft(o.expires_at)}</span> left</span>}
          <span className="text-muted"> · <span suppressHydrationWarning>{ago(o.created_at)}</span></span>
        </p>
        {o.status === "accepted" && side === "buying" && (
          <Link href="/purchases" className="mt-1 inline-block font-semibold text-link underline">Pay within 48 hours</Link>
        )}
        {side === "selling" && <div className="mt-1"><BuyerRecord username={o.buyer} /></div>}
        {o.can_respond && <OfferRespond o={o} />}
      </div>
    </li>
  );
}
