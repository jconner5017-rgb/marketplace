import Link from "next/link";
import { getSeller } from "@/lib/api";
import { requireUser } from "@/lib/auth";

export const dynamic = "force-dynamic";
export const metadata = { title: "Back from Stripe" };

// Stripe sends sellers here after onboarding. Loading the seller refreshes
// their status from Stripe.
export default async function OnboardingReturn() {
  await requireUser("/seller/onboarding/return");
  const { seller } = await getSeller();
  const ready = seller?.charges_enabled;
  return (
    <div className="mx-auto max-w-[560px] pt-16 text-center">
      <h1 className="text-[26px] font-bold">{ready ? "You're ready to sell" : "Almost there"}</h1>
      <p className="mt-2 text-muted">
        {ready
          ? "Your Stripe account is connected. Sales are paid straight into it."
          : seller?.details_submitted
            ? "Stripe is reviewing your details. Listing unlocks as soon as they approve."
            : "Your Stripe setup isn't finished yet. You can continue it from the seller hub."}
      </p>
      <Link href={ready ? "/sell/new" : "/sell"} className="mt-6 inline-block rounded-full bg-link px-6 py-2.5 font-semibold text-white">
        {ready ? "List your first card" : "Go to the seller hub"}
      </Link>
    </div>
  );
}
