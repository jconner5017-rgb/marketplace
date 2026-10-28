"use client";

import { useRouter } from "next/navigation";

export default function SignOutButton() {
  const router = useRouter();
  async function signOut() {
    await fetch("/api/v1/auth/logout", { method: "POST", headers: { "content-type": "application/json" }, body: "{}" });
    router.push("/");
    router.refresh();
  }
  return (
    <button type="button" onClick={signOut} className="ml-1 text-link underline">
      (Sign out)
    </button>
  );
}
