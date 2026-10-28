import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import type { Cart } from "@/lib/types";
import { GradeBadge, Photo } from "@/components/CardBits";
import { money } from "@/lib/format";
import CartLineActions from "@/components/CartLineActions";

export const dynamic = "force-dynamic";
export const metadata = { title: "Cart" };

export default async function CartPage() {
  await requireUser("/cart");
  const r = await apiGet<Cart>("/v1/cart");
  const cart = r.data ?? { groups: [], card_count: 0, total_cents: 0, problems: 0 };
  const sellers = cart.groups.filter((g) => g.card_count > 0).length;

  if (cart.groups.length === 0) {
    return (
      <div className="py-16 text-center">
        <h1 className="text-[26px] font-bold">Your cart is empty</h1>
        <p className="mt-1 text-muted">Cards you add show up here, grouped by seller.</p>
        <Link href="/search?sort=newest" className="mt-5 inline-block rounded-full bg-link px-5 py-2 font-semibold text-white">Browse cards</Link>
      </div>
    );
  }

  return (
    <div className="pt-6">
      <h1 className="text-[26px] font-bold">Shopping cart</h1>
      <div className="mt-4 grid gap-8 lg:grid-cols-[1fr_340px]">
        <div className="space-y-6">
          {cart.groups.map((g) => (
            <section key={g.seller_username} className="rounded-lg border border-line">
              <header className="flex flex-wrap items-baseline justify-between gap-2 border-b border-line bg-wash px-4 py-2.5 text-[14px]">
                <span>Seller <Link href={`/search?seller=${encodeURIComponent(g.seller_username)}`} className="font-semibold text-link hover:underline">{g.seller_username}</Link></span>
                <span className="text-muted">
                  {g.ship_method === "envelope" ? "Ships in a stamped envelope" : "Ships together in one package with tracking"} · Free
                </span>
              </header>
              <ul>
                {g.lines.map((ln) => (
                  <li key={ln.listing.id} className={`flex gap-4 border-b border-wash px-4 py-4 last:border-0 ${ln.available ? "" : "bg-slab/5"}`}>
                    <Link href={`/item/${ln.listing.id}`} className="shrink-0"><Photo l={ln.listing} className="h-[112px] w-[80px] rounded" /></Link>
                    <div className="min-w-0 flex-1">
                      <Link href={`/item/${ln.listing.id}`} className="title-tight text-[16px] hover:underline">{ln.listing.title}</Link>
                      <div className="mt-1.5 flex items-center gap-2"><GradeBadge l={ln.listing} /><span className="text-[13px] text-muted">{ln.listing.set_name}</span></div>
                      {ln.problem && <p className="mt-1.5 text-[13px] font-semibold text-slab">{ln.problem}</p>}
                      <CartLineActions id={ln.listing.id} quantity={ln.quantity} max={Math.max(ln.listing.quantity, 1)} available={ln.available} />
                    </div>
                    <p className="num shrink-0 text-right text-[17px] font-bold">{ln.available ? money(ln.line_cents) : "—"}</p>
                  </li>
                ))}
              </ul>
              {g.discount_cents > 0 && (
                <p className="num flex justify-between border-t border-line px-4 py-2 text-[14px] text-ship">
                  <span>Multi-card discount · {g.discount_label}</span><span>−{money(g.discount_cents)}</span>
                </p>
              )}
            </section>
          ))}
        </div>

        <aside className="h-fit rounded-lg border border-line p-5 lg:sticky lg:top-4">
          <dl className="num space-y-2 text-[15px]">
            <div className="flex justify-between"><dt>Cards ({cart.card_count})</dt><dd>{money(cart.groups.reduce((a, g) => a + g.subtotal_cents, 0))}</dd></div>
            {cart.groups.some((g) => g.discount_cents > 0) && (
              <div className="flex justify-between text-ship"><dt>Multi-card discounts</dt><dd>−{money(cart.groups.reduce((a, g) => a + g.discount_cents, 0))}</dd></div>
            )}
            <div className="flex justify-between"><dt>Shipping</dt><dd className="text-ship">Free</dd></div>
            <div className="flex justify-between border-t border-line pt-3 text-[18px] font-bold"><dt>Total</dt><dd>{money(cart.total_cents)}</dd></div>
          </dl>
          <button type="button" disabled className="mt-5 w-full rounded-full bg-link py-3 font-semibold text-white opacity-50" aria-describedby="checkout-note">
            Go to checkout
          </button>
          <p id="checkout-note" className="mt-2 text-center text-[13px] text-muted">
            Checkout opens soon.{sellers > 1 ? ` You'll see ${sellers} charges, one per seller.` : ""}
          </p>
          {cart.problems > 0 && <p className="mt-3 text-[13px] text-slab">Remove the unavailable {cart.problems === 1 ? "card" : "cards"} before checking out.</p>}
        </aside>
      </div>
    </div>
  );
}
