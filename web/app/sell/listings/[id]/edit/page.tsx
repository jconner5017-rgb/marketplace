import Link from "next/link";
import { notFound, redirect } from "next/navigation";
import { apiGet, getSeller } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import { getCatalog, getFees } from "@/lib/catalog";
import type { Listing } from "@/lib/types";
import NewListingForm from "@/components/NewListingForm";

export const dynamic = "force-dynamic";
export const metadata = { title: "Edit listing", robots: { index: false } };

export default async function EditListingPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  await requireUser(`/sell/listings/${id}/edit`);
  const [{ seller }, r, catalog, fees] = await Promise.all([
    getSeller(),
    apiGet<{ listing: Listing }>(`/v1/seller/listings/${encodeURIComponent(id)}`),
    getCatalog(),
    getFees(),
  ]);
  if (!seller) redirect("/sell");
  if (r.status === 404) notFound();
  const l = r.data?.listing;
  if (!l) {
    return <p role="alert" className="mx-auto max-w-[760px] pt-10 text-center">We couldn&apos;t load this listing just now. Reload the page in a moment.</p>;
  }
  const ended = l.format === "auction" && l.auction?.state !== "live";
  return (
    <div className="mx-auto max-w-[760px] pt-6">
      <p className="text-[13px]"><Link href="/sell" className="text-link hover:underline">Seller hub</Link> › Edit listing</p>
      <h1 className="mt-1 text-[26px] font-bold">Edit listing</h1>
      <p className="mt-1 text-muted">
        <Link href={`/item/${l.id}`} className="text-link hover:underline">{l.title}</Link>
        {l.status === "hidden" ? " · hidden from buyers" : ""}
      </p>
      {ended ? (
        <p className="mt-6 rounded-lg bg-wash p-4 text-[14px]">This auction has ended, so it can&apos;t be changed. List the card again to sell it.</p>
      ) : (
        <NewListingForm capCents={seller.tier_cap_cents} catalog={catalog} fees={fees} initial={l} />
      )}
    </div>
  );
}
