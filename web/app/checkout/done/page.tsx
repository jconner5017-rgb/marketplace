import Link from "next/link";
import { apiPost } from "@/lib/api";
import { requireUser } from "@/lib/auth";

export const dynamic = "force-dynamic";
export const metadata = { title: "Checkout", robots: { index: false } };

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** Where Stripe sends the buyer back. Confirms the payment with Stripe. */
export default async function CheckoutDone({ searchParams }: {
  searchParams: Promise<{ order?: string; session_id?: string; cancelled?: string; try?: string }>;
}) {
  const sp = await searchParams;
  const order = sp.order && UUID.test(sp.order) ? sp.order : "";
  await requireUser(`/checkout/done?order=${order}`);

  if (!order || sp.cancelled) {
    // Close the Stripe page and release the cards now. If it turns out the
    // buyer did pay, fall through to the confirmation below.
    const c = order ? await apiPost<{ status: string }>(`/v1/orders/${order}/cancel`) : null;
    if (c?.data?.status !== "paid") return (
      <div className="mx-auto max-w-[560px] py-16 text-center">
        <h1 className="text-[26px] font-bold">Checkout cancelled</h1>
        <p className="mt-2 text-muted">You weren&apos;t charged. Your cards are still waiting in your cart or Purchases if you want to try again.</p>
        <div className="mt-6 flex justify-center gap-3">
          <Link href="/cart" className="rounded-full bg-link px-5 py-2 font-semibold text-white">Back to cart</Link>
          <Link href="/purchases" className="rounded-full border border-link px-5 py-2 font-semibold text-link">Purchases</Link>
        </div>
      </div>
    );
  }

  const sid = (sp.session_id ?? "").slice(0, 200);
  const r = await apiPost<{ status: string }>(`/v1/orders/${order}/confirm?session_id=${encodeURIComponent(sid)}`);
  const status = r.data?.status;
  const tries = Math.min(Number(sp.try) || 0, 10);

  if (status === "paid" || status === "shipped" || status === "delivered") {
    return (
      <div className="mx-auto max-w-[560px] py-16 text-center">
        <p className="text-[40px]" aria-hidden>✓</p>
        <h1 className="text-[26px] font-bold">Thanks, you&apos;re all set</h1>
        <p className="mt-2 text-muted">Payment received. The seller has been told to ship, and you&apos;ll get a notification with tracking.</p>
        <div className="mt-6 flex justify-center gap-3">
          <Link href={`/purchases/${order}`} className="rounded-full bg-link px-5 py-2 font-semibold text-white">View order</Link>
          <Link href="/cart" className="rounded-full border border-link px-5 py-2 font-semibold text-link">Back to cart</Link>
        </div>
      </div>
    );
  }

  if (status === "pending_payment" && tries < 10) {
    return (
      <div className="mx-auto max-w-[560px] py-16 text-center">
        {/* Check again in a few seconds while Stripe finishes. */}
        <meta httpEquiv="refresh" content={`3;url=/checkout/done?order=${order}&session_id=${encodeURIComponent(sid)}&try=${tries + 1}`} />
        <h1 className="text-[26px] font-bold">Confirming your payment…</h1>
        <p className="mt-2 text-muted">This usually takes a few seconds. Please don&apos;t pay again.</p>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-[560px] py-16 text-center">
      <h1 className="text-[26px] font-bold">{status === "cancelled" ? "This checkout expired" : "We're still checking on your payment"}</h1>
      <p className="mt-2 text-muted">
        {status === "cancelled"
          ? "You weren't charged. Head back to your cart to try again."
          : "If you were charged, the order will appear in Purchases shortly. Please don't pay again."}
      </p>
      <div className="mt-6 flex justify-center gap-3">
        <Link href="/purchases" className="rounded-full bg-link px-5 py-2 font-semibold text-white">Purchases</Link>
        <Link href="/cart" className="rounded-full border border-link px-5 py-2 font-semibold text-link">Cart</Link>
      </div>
    </div>
  );
}
