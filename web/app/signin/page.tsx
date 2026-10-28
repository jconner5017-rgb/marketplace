import { Suspense } from "react";
import AuthForm from "@/components/AuthForm";

export const metadata = { title: "Sign in" };

export default function SignInPage() {
  return (
    <div className="mx-auto max-w-[400px] pt-12">
      <h1 className="mb-6 text-[26px] font-bold">Sign in</h1>
      <Suspense>
        <AuthForm mode="signin" />
      </Suspense>
    </div>
  );
}
