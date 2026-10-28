import { apiGet } from "@/lib/api";
import { requireStaff, ROLE_LABEL } from "@/lib/staff";
import { money } from "@/lib/format";

export const dynamic = "force-dynamic";
export const metadata = { title: "Dashboard" };

type Stat = { key: string; label: string; value: number; money?: boolean };

export default async function AdminHome({ searchParams }: { searchParams: Promise<{ denied?: string }> }) {
  const s = await requireStaff("/admin");
  const denied = (await searchParams).denied;
  const r = await apiGet<{ stats: Stat[] }>("/v1/admin/dashboard");
  const stats = r.data?.stats ?? [];
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
      <ul className="mt-6 grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-4">
        {stats.map((st) => (
          <li key={st.key} className="rounded-lg border border-line p-4">
            <p className="text-[13px] text-muted">{st.label}</p>
            <p className="num mt-1 text-[26px] font-extrabold leading-none">{st.money ? money(st.value) : st.value.toLocaleString("en-US")}</p>
          </li>
        ))}
      </ul>
      <p className="mt-6 text-[13px] text-muted">
        Money figures (sales, fees, conversion) appear once checkout is live.
      </p>
    </div>
  );
}
