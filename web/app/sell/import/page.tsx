import { redirect } from "next/navigation";
import { getSeller } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import CsvImport from "@/components/CsvImport";

export const dynamic = "force-dynamic";
export const metadata = { title: "Upload a CSV" };

export default async function ImportPage() {
  await requireUser("/sell/import");
  const { seller } = await getSeller();
  if (!seller?.charges_enabled) redirect("/sell");
  return (
    <div className="mx-auto max-w-[860px] pt-6">
      <h1 className="text-[26px] font-bold">Upload a CSV</h1>
      <p className="mt-1 max-w-[70ch] text-muted">
        List up to 5,000 cards at once. Each row needs your own SKU in <code>external_id</code>, so uploading the same
        file again updates those cards instead of listing them twice.
      </p>
      <p className="mt-2">
        <a href="/api/v1/seller/listings/import/template" className="text-link underline">Download the template</a>
      </p>
      <CsvImport />
    </div>
  );
}
