import { apiGet } from "@/lib/api";
import { requireStaff, ROLE_LABEL } from "@/lib/staff";
import { money } from "@/lib/format";
import DailyBars from "@/components/admin/DailyBars";

type KPIs = {
  days: number; gmv_cents: number; fee_cents: number; net_fee_cents: number; orders: number; aov_cents: number;
  active_users: number; new_users: number; active_listings: number; views: number; conversion: number;
  series: { day: string; gmv_cents: number; orders: number; new_users: number; views: number }[];
};

export const dynamic = "force-dynamic";
export const metadata = { title: "Dashboard" };

type Stat = { key: string; label: string; value: number; money?: boolean };

export default async function AdminHome({ searchParams }: { searchParams: Promise<{ denied?: string; days?: string }> }) {
  const s = await requireStaff("/admin");
  const sp = await searchParams;
  const denied = sp.denied;
  const days = [7, 30, 90].includes(Number(sp.days)) ? Number(sp.days) : 30;
  const showMoney = s.permissions.includes("orders.view");
  const [r, kr] = await Promise.all([
    apiGet<{ stats: Stat[] }>("/v1/admin/dashboard"),
    showMoney ? apiGet<KPIs>(`/v1/admin/kpis?days=${days}`) : Promise.resolve({ status: 0, data: null }),
  ]);
  const stats = r.data?.stats ?? [];
  const k = kr.data;
  const n = (v: number) => v.toLocaleString("en-US");
  return (
    <div>
      <h1 className="text-[26px] font-bold">Dashboard</h1>
      <p className="text-muted">
        {ROLE_LABEL[s.staff_role]} · staff access until{" "}
        <span suppressHydrationWarning>{s.mfa_until ? new Date(s.mfa_until).toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit" }) : ""}</span>
      </p>
      {denied && (
        <p role="alert" className="mt-4 rounded-lg border border-slab/40 bg-slab/5 p-3 text-[14px] text-slab">
          Your staff role doesn&apos;t include that page.
        </p>
      )}
      {k && (
        <section className="mt-6">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h2 className="text-[18px] font-bold">Platform, last {k.days} days</h2>
            <nav className="flex gap-2 text-[13px]">
              {[7, 30, 90].map((d) => (
                <a key={d} href={`/admin?days=${d}`} className={`rounded-full border px-3 py-1 ${d === days ? "border-ink bg-ink text-white" : "border-line hover:border-ink"}`}>{d} days</a>
              ))}
            </nav>
          </div>
          <ul className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6">
            {[
              ["Gross sales (GMV)", money(k.gmv_cents)],
              ["Net fee revenue", money(k.net_fee_cents)],
              ["Orders", n(k.orders)],
              ["Average order", money(k.aov_cents)],
              ["Active members", n(k.active_users)],
              ["Conversion", k.views ? `${(k.conversion * 100).toFixed(2)}%` : "—"],
            ].map(([l, v]) => (
              <li key={l} className="rounded-lg border border-ink/20 bg-wash/40 p-4">
                <p className="text-[13px] text-muted">{l}</p>
                <p className="num mt-1 text-[24px] font-extrabold leading-none">{v}</p>
              </li>
            ))}
          </ul>
          <p className="mt-2 text-[12px] text-muted">
            GMV is paid orders before refunds. Net fee revenue subtracts fees on fully refunded orders. Conversion is orders ÷ listing page views ({n(k.views)} views).
          </p>
          <div className="mt-4 grid gap-4 lg:grid-cols-2">
            <DailyBars title="Gross sales per day" days={k.series.map((x) => x.day)} values={k.series.map((x) => x.gmv_cents)} format={money} />
            <DailyBars title="Orders per day" days={k.series.map((x) => x.day)} values={k.series.map((x) => x.orders)} format={n} />
            <DailyBars title="New members per day" days={k.series.map((x) => x.day)} values={k.series.map((x) => x.new_users)} format={n} />
            <DailyBars title="Listing views per day" days={k.series.map((x) => x.day)} values={k.series.map((x) => x.views)} format={n} />
          </div>
        </section>
      )}

      <h2 className="mt-8 text-[18px] font-bold">Right now</h2>
      <ul className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-4">
        {stats.map((st) => (
          <li key={st.key} className="rounded-lg border border-line p-4">
            <p className="text-[13px] text-muted">{st.label}</p>
            <p className="num mt-1 text-[26px] font-extrabold leading-none">{st.money ? money(st.value) : st.value.toLocaleString("en-US")}</p>
          </li>
        ))}
      </ul>
      {k && k.orders === 0 && <p className="mt-6 text-[13px] text-muted">Sales figures fill in once checkout is switched on.</p>}
    </div>
  );
}
