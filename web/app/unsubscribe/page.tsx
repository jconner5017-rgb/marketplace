import { Suspense } from "react";
import { Unsubscribe } from "@/components/TokenAction";

export const metadata = { title: "Unsubscribe from announcements", robots: { index: false } };

export default function Page() {
  return (
    <div className="mx-auto max-w-[440px] pt-12">
      <h1 className="text-[26px] font-bold">Unsubscribe from announcements</h1>
      <p className="mt-2 text-muted">Stop getting Slabs &amp; Singles announcement emails. You&apos;ll still get emails about your account and orders.</p>
      <Suspense><Unsubscribe /></Suspense>
    </div>
  );
}
