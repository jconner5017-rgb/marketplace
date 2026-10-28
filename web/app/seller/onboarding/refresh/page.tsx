import { Suspense } from "react";
import OnboardButton from "@/components/OnboardButton";

export const metadata = { title: "Continue Stripe setup" };

// Stripe sends sellers here when an onboarding link expires.
export default function OnboardingRefresh() {
  return (
    <div className="mx-auto max-w-[560px] pt-16 text-center">
      <h1 className="text-[26px] font-bold">Your Stripe link expired</h1>
      <p className="mt-2 text-muted">Setup links only last a few minutes. Get a fresh one to carry on where you left off.</p>
      <div className="mt-6 flex justify-center">
        <Suspense><OnboardButton label="Continue Stripe setup" /></Suspense>
      </div>
    </div>
  );
}
