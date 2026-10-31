import { Suspense } from "react";
import { ForgotPassword } from "@/components/TokenAction";

export const metadata = { title: "Reset your password", robots: { index: false } };

export default function Page() {
  return (
    <div className="mx-auto max-w-[440px] pt-12">
      <h1 className="text-[26px] font-bold">Reset your password</h1>
      <p className="mt-2 text-muted">Enter the email you signed up with and we&apos;ll send a link to choose a new password.</p>
      <Suspense><ForgotPassword /></Suspense>
    </div>
  );
}
