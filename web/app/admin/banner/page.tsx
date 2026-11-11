import { apiGet } from "@/lib/api";
import type { SiteBanner } from "@/lib/types";
import { requireStaff } from "@/lib/staff";
import BannerForm, { type BannerSize } from "@/components/admin/BannerForm";

export const dynamic = "force-dynamic";
export const metadata = { title: "Homepage banner" };

export default async function BannerPage() {
  await requireStaff("/admin/banner", "announce");
  const r = await apiGet<{ banner: SiteBanner; size: BannerSize }>("/v1/admin/banners/home");
  if (!r.data) {
    return <p role="alert" className="text-muted">Couldn&apos;t load the banner. Try again in a moment.</p>;
  }
  return (
    <div className="max-w-[1000px]">
      <h1 className="text-[26px] font-bold">Homepage banner</h1>
      <p className="mt-1 text-muted">
        Shows on the homepage between <b>Newly listed</b> and the next row of cards. Upload an image, add a link, then switch it on.
      </p>
      <BannerForm initial={r.data.banner} size={r.data.size} />
    </div>
  );
}
