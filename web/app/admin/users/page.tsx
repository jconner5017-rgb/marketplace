import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { ROLE, StatusBadge, when } from "@/components/admin/Badges";

export const dynamic = "force-dynamic";
export const metadata = { title: "Members" };

type Row = {
  id: string; email: string; username: string; status: string; staff_role: string | null; created_at: string;
  is_seller: boolean; tier: number | null; review_status: string | null; active_listings: number; last_login: string | null;
};

export default async function UsersPage({ searchParams }: { searchParams: Promise<Record<string, string | undefined>> }) {
  await requireStaff("/admin/users", "users.view");
  const sp = await searchParams;
  const page = Math.max(1, Number(sp.page) || 1);
  const qs = new URLSearchParams({ limit: "50", offset: String((page - 1) * 50) });
  for (const k of ["q", "status", "staff", "sellers"]) if (sp[k]) qs.set(k, sp[k]!);
  const r = await apiGet<{ users: Row[]; total: number }>("/v1/admin/users?" + qs.toString());
  const rows = r.data?.users ?? [];
  const total = r.data?.total ?? 0;
  const link = (p: number) => {
    const n = new URLSearchParams(Object.entries(sp).filter(([, v]) => v) as [string, string][]);
    n.set("page", String(p));
    return `/admin/users?${n}`;
  };

  return (
    <div>
      <h1 className="text-[26px] font-bold">Members</h1>
      <form className="mt-4 flex flex-wrap items-end gap-3 text-[14px]" action="/admin/users">
        <label className="flex-1 min-w-[220px]">
          <span className="block text-[13px] font-semibold">Search</span>
          <input name="q" defaultValue={sp.q} placeholder="Email, username or member ID" className="mt-1 w-full rounded-md border border-line px-3 py-2" />
        </label>
        <label>
          <span className="block text-[13px] font-semibold">Status</span>
          <select name="status" defaultValue={sp.status ?? ""} className="mt-1 rounded-md border border-line px-2 py-2">
            <option value="">Any</option>
            {["active", "frozen", "suspended", "banned"].map((s) => <option key={s} value={s}>{s}</option>)}
          </select>
        </label>
        <label className="flex items-center gap-1.5 pb-2"><input type="checkbox" name="sellers" value="1" defaultChecked={sp.sellers === "1"} /> Sellers</label>
        <label className="flex items-center gap-1.5 pb-2"><input type="checkbox" name="staff" value="1" defaultChecked={sp.staff === "1"} /> Staff</label>
        <button className="rounded-full bg-link px-5 py-2 font-semibold text-white hover:bg-link-dark">Search</button>
      </form>
      <p className="mt-4 text-[13px] text-muted">{total.toLocaleString("en-US")} {total === 1 ? "member" : "members"}</p>

      {/* Cards on phones, a table on wider screens. */}
      <ul className="mt-2 space-y-2 md:hidden">
        {rows.map((m) => (
          <li key={m.id}>
            <Link href={`/admin/users/${m.id}`} className="block rounded-lg border border-line p-3 text-[14px] hover:border-ink">
              <div className="flex items-center justify-between gap-2">
                <span className="font-semibold">{m.username}</span>
                <StatusBadge status={m.status} />
              </div>
              <p className="text-muted">{m.email}</p>
              <p className="text-[12px] text-muted">
                {m.staff_role ? ROLE[m.staff_role] + " · " : ""}{m.is_seller ? `Seller (level ${m.tier}, ${m.review_status}) · ${m.active_listings} listings · ` : ""}joined {when(m.created_at)}
              </p>
            </Link>
          </li>
        ))}
      </ul>
      <div className="mt-2 hidden overflow-x-auto md:block">
        <table className="w-full text-[14px]">
          <thead>
            <tr className="border-b border-line text-left text-[13px] text-muted">
              <th className="py-2 pr-3 font-semibold">Member</th>
              <th className="py-2 pr-3 font-semibold">Status</th>
              <th className="py-2 pr-3 font-semibold">Staff</th>
              <th className="py-2 pr-3 font-semibold">Seller</th>
              <th className="py-2 pr-3 text-right font-semibold">Listings</th>
              <th className="py-2 pr-3 font-semibold">Last sign-in</th>
              <th className="py-2 font-semibold">Joined</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((m) => (
              <tr key={m.id} className="border-b border-wash">
                <td className="py-2 pr-3">
                  <Link href={`/admin/users/${m.id}`} className="font-semibold text-link hover:underline">{m.username}</Link>
                  <div className="text-[12px] text-muted">{m.email}</div>
                </td>
                <td className="py-2 pr-3"><StatusBadge status={m.status} /></td>
                <td className="py-2 pr-3">{m.staff_role ? ROLE[m.staff_role] : ""}</td>
                <td className="py-2 pr-3">{m.is_seller ? <>Level {m.tier} · <StatusBadge status={m.review_status ?? "pending"} /></> : ""}</td>
                <td className="num py-2 pr-3 text-right">{m.active_listings}</td>
                <td className="py-2 pr-3 text-[13px]" suppressHydrationWarning>{when(m.last_login)}</td>
                <td className="py-2 text-[13px]" suppressHydrationWarning>{when(m.created_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {rows.length === 0 && <p className="py-8 text-center text-muted">No members match.</p>}
      <div className="mt-4 flex gap-4 text-[14px]">
        {page > 1 && <Link href={link(page - 1)} className="text-link underline">Previous</Link>}
        {page * 50 < total && <Link href={link(page + 1)} className="text-link underline">Next</Link>}
      </div>
    </div>
  );
}
