import { Suspense } from "react";
import { ConfirmEmail } from "@/components/TokenAction";

export const metadata = { title: "Confirm your email", robots: { index: false } };

export default function Page() {
  return (
    <div className="mx-auto max-w-[440px] pt-12">
      <h1 className="text-[26px] font-bold">Confirm your email</h1>
      <p className="mt-2 text-muted">One click and you can buy, bid, make offers and sell.</p>
      <Suspense><ConfirmEmail /></Suspense>
    </div>
  );
}
