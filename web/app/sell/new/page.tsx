import { getSeller } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import { redirect } from "next/navigation";
import NewListingForm from "@/components/NewListingForm";
import { getCatalog, getFees } from "@/lib/catalog";

export const dynamic = "force-dynamic";
export const metadata = { title: "List a card" };

export default async function NewListingPage() {
  await requireUser("/sell/new");
  const [{ seller }, catalog, fees] = await Promise.all([getSeller(), getCatalog(), getFees()]);
  if (!seller?.charges_enabled) redirect("/sell");
  return (
    <div className="mx-auto max-w-[760px] pt-6">
      <h1 className="text-[26px] font-bold">List a card</h1>
      <p className="mt-1 text-muted">Your price limit is ${(seller.tier_cap_cents / 100).toFixed(2)} per card. Every listing ships free.</p>
      <NewListingForm capCents={seller.tier_cap_cents} catalog={catalog} fees={fees} />
    </div>
  );
}
