import { Suspense } from "react";
import { ResetPassword } from "@/components/TokenAction";

export const metadata = { title: "Choose a new password", robots: { index: false } };

export default function Page() {
  return (
    <div className="mx-auto max-w-[440px] pt-12">
      <h1 className="text-[26px] font-bold">Choose a new password</h1>
      
      <Suspense><ResetPassword /></Suspense>
    </div>
  );
}
