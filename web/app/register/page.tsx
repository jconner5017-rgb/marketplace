import Link from "next/link";
import { Suspense } from "react";
import AuthForm from "@/components/AuthForm";

export const metadata = { title: "Create an account" };

export default function RegisterPage() {
  return (
    <div className="mx-auto max-w-[400px] pt-12">
      <h1 className="text-[26px] font-bold">Create an account</h1>
      <p className="mb-6 mt-1 text-[14px] text-muted">One account for buying and selling. US addresses only.</p>
      <Suspense>
        <AuthForm mode="register" />
      </Suspense>
      <p className="mt-4 text-[13px] text-muted">
        By creating an account, you agree to the{" "}
        <Link href="/help/terms" className="text-link hover:underline">Terms of Service</Link>,{" "}
        <Link href="/help/user-agreement" className="text-link hover:underline">User Agreement</Link> and{" "}
        <Link href="/help/privacy" className="text-link hover:underline">Privacy Statement</Link>.
      </p>
    </div>
  );
}
