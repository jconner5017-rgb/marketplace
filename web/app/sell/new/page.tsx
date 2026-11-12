import { getListing, getSeller } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import { redirect } from "next/navigation";
import NewListingForm from "@/components/NewListingForm";
import { getCatalog, getFees } from "@/lib/catalog";

export const dynamic = "force-dynamic";
export const metadata = { title: "List a card or lot" };

export default async function NewListingPage({ searchParams }: { searchParams: Promise<{ lot?: string; like?: string }> }) {
  const sp = await searchParams;
  const lot = sp.lot === "1";
  // "Sell one like this" from a listing page: copy that card's details (not its photos, price or condition).
  const like = !lot && sp.like && /^[0-9a-f-]{36}$/i.test(sp.like) ? sp.like : null;
  await requireUser(lot ? "/sell/new?lot=1" : like ? `/sell/new?like=${like}` : "/sell/new");
  const [{ seller }, catalog, fees, source] = await Promise.all([getSeller(), getCatalog(), getFees(), like ? getListing(like) : Promise.resolve(null)]);
  const prefill = source && !source.lot_count
    ? { category: source.category, game_or_sport: source.game_or_sport, set_name: source.set_name, year: source.year,
        card_number: source.card_number, title: source.title, attributes: source.attributes }
    : undefined;
  if (!seller?.charges_enabled) redirect("/sell");
  return (
    <div className="mx-auto max-w-[760px] pt-6">
      <h1 className="text-[26px] font-bold">{lot ? "List a lot" : "List a card"}</h1>
      <p className="mt-1 text-muted">Your price limit is ${(seller.tier_cap_cents / 100).toFixed(2)} per listing (a lot counts as one listing). Every listing ships free.</p>
      {prefill && (
        <p className="mt-3 rounded-lg bg-wash p-3 text-[14px]">Card details are copied from the listing you were viewing. Add your own photos, condition and price, and check the details match your card.</p>
      )}
      <NewListingForm capCents={seller.tier_cap_cents} catalog={catalog} fees={fees} startAsLot={lot} prefill={prefill} />
    </div>
  );
}
