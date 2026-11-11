import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import TestSales, { type TestSale } from "@/components/admin/TestSales";

export const dynamic = "force-dynamic";
export const metadata = { title: "Test sale" };

export default async function TestSalePage() {
  await requireStaff("/admin/test-sale", "staff.manage");
  const r = await apiGet<{ test_sales: TestSale[] }>("/v1/admin/test-sales");
  return (
    <div className="max-w-[680px]">
      <h1 className="text-[26px] font-bold">Test sale</h1>
      <p className="mt-1 text-muted">
        Puts a sold order in <b>your own</b> seller orders, so you can try the seller side for real: open it from the Seller hub (on
        your phone too), print the tracked envelope and mark it shipped. Nobody pays, no fees are charged, and no listing is used.
      </p>
      <ul className="mt-3 list-disc space-y-1 pl-5 text-[14px] text-muted">
        <li>It ships by envelope and is addressed to your own ship-from address, so an envelope that does get mailed comes back to you.</li>
        <li>The buyer is an internal test account; no emails go out to it.</li>
        <li>Delete the test sale here when you&apos;re done. Real orders can&apos;t be deleted from this page.</li>
      </ul>
      <TestSales initial={r.data?.test_sales ?? []} />
    </div>
  );
}
