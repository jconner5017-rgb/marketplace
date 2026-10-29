import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import { money } from "@/lib/format";

export const dynamic = "force-dynamic";
export const metadata = { title: "Sales reports", robots: { index: false } };

type Month = {
  month: string; orders: number; cards: number; gross_cents: number; discount_cents: number; tax_cents: number;
  fee_cents: number; label_cents: number; refunded_cents: number; net_cents: number;
};

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const ymd = (d: Date) => d.toISOString().slice(0, 10);

export default async function ReportsPage({ searchParams }: { searchParams: Promise<{ year?: string }> }) {
  await requireUser("/sell/reports");
  const thisYear = new Date().getFullYear();
  const y = Number((await searchParams).year) || thisYear;
  const r = await apiGet<{ year: number; months: Month[]; total: Month }>(`/v1/seller/reports/summary?year=${y}`);
  const d = r.data;
  const today = new Date();
  const monthAgo = new Date(Date.now() - 29 * 86400e3);
  const years = Array.from({ length: thisYear - 2025 + 1 }, (_, i) => thisYear - i);
  const cols: [keyof Month, string][] = [
    ["orders", "Orders"], ["gross_cents", "Gross sales"], ["discount_cents", "Discounts"], ["tax_cents", "Sales tax"],
    ["fee_cents", "Our fees"], ["label_cents", "Labels"], ["refunded_cents", "Refunds"], ["net_cents", "Net"],
  ];

  return (
    <div className="pt-6">
      <Link href="/sell" className="text-[14px] text-link hover:underline">← Seller hub</Link>
      <h1 className="mt-2 text-[26px] font-bold">Sales reports</h1>
      <p className="max-w-[640px] text-muted">
        For your bookkeeping and taxes. Each order lists gross sales, discounts, sales tax, our fee, label costs, refunds and the
        buyer&apos;s ship-to state, in case you have to work out tax by destination.
      </p>

      <section className="mt-6 max-w-[640px] rounded-lg border border-line p-4">
        <h2 className="font-bold">Download orders (CSV)</h2>
        <form action="/api/v1/seller/reports/sales.csv" method="get" className="mt-3 flex flex-wrap items-end gap-3 text-[14px]">
          <label className="font-semibold">From
            <input type="date" name="from" defaultValue={ymd(monthAgo)} required className="mt-1 block rounded-md border border-line px-2 py-1.5 font-normal" />
          </label>
          <label className="font-semibold">To
            <input type="date" name="to" defaultValue={ymd(today)} required className="mt-1 block rounded-md border border-line px-2 py-1.5 font-normal" />
          </label>
          <button type="submit" className="rounded-full bg-link px-5 py-2 font-semibold text-white hover:bg-link-dark">Download CSV</button>
        </form>
        <p className="mt-2 text-[13px] text-muted">Up to 400 days at a time. Dates are US Eastern.</p>
      </section>

      <section className="mt-8">
        <div className="flex flex-wrap items-baseline justify-between gap-3">
          <h2 className="text-[20px] font-bold">{y} summary</h2>
          <nav aria-label="Year" className="flex gap-1 text-[14px]">
            {years.map((v) => (
              <Link key={v} href={`/sell/reports?year=${v}`} aria-current={v === y ? "page" : undefined}
                className={`rounded-full border px-3 py-1 ${v === y ? "border-ink bg-ink text-white" : "border-line hover:border-ink"}`}>{v}</Link>
            ))}
          </nav>
        </div>
        {!d ? <p className="py-6 text-muted">Couldn&apos;t load the summary.</p> : (
          <div className="mt-3 overflow-x-auto">
            <table className="num w-full min-w-[720px] text-[14px]">
              <thead>
                <tr className="border-b border-line text-right text-[13px] text-muted">
                  <th className="py-2 pr-3 text-left font-semibold">Month</th>
                  {cols.map(([, l]) => <th key={l} className="py-2 pr-3 font-semibold">{l}</th>)}
                </tr>
              </thead>
              <tbody>
                {d.months.map((m, i) => (
                  <tr key={m.month} className={`border-b border-wash text-right ${m.orders === 0 ? "text-muted" : ""}`}>
                    <td className="py-1.5 pr-3 text-left">{MONTHS[i]}</td>
                    {cols.map(([k]) => <td key={k} className="py-1.5 pr-3">{k === "orders" ? m[k] : money(Number(m[k]))}</td>)}
                  </tr>
                ))}
                <tr className="border-t-2 border-ink text-right font-bold">
                  <td className="py-2 pr-3 text-left">Year</td>
                  {cols.map(([k]) => <td key={k} className="py-2 pr-3">{k === "orders" ? d.total[k] : money(Number(d.total[k]))}</td>)}
                </tr>
              </tbody>
            </table>
            <p className="mt-2 text-[13px] text-muted">
              Net is before Stripe&apos;s card processing fee, which Stripe reports in your own dashboard. Sales tax is passed
              through to you to remit. Stripe sends Form 1099-K if you meet the IRS or your state&apos;s threshold.
            </p>
          </div>
        )}
      </section>
    </div>
  );
}
