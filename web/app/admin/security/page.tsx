import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import { when } from "@/components/admin/Badges";
import ActionForm from "@/components/admin/ActionForm";
import DeleteButton from "@/components/admin/DeleteButton";

export const dynamic = "force-dynamic";
export const metadata = { title: "Network allowlist" };

type Rule = { id: number; cidr: string; label: string; created_by: string | null; created_at: string };

export default async function SecurityPage() {
  await requireStaff("/admin/security", "staff.manage");
  const r = await apiGet<{ rules: Rule[]; your_ip: string; enforced: boolean; switched_off: boolean }>("/v1/admin/ip-rules");
  const d = r.data ?? { rules: [], your_ip: "", enforced: false, switched_off: false };
  return (
    <div className="max-w-[760px]">
      <h1 className="text-[26px] font-bold">Network allowlist</h1>
      <p className="mt-1 text-muted">
        {d.switched_off
          ? "The allowlist is switched off on the server (ADMIN_IP_ALLOWLIST=off), so any network can reach the back office."
          : d.enforced
            ? "The back office only opens from the networks below. Everyone else gets a \"not available\" page, and the attempt raises a fraud alert."
            : "No networks are listed, so staff can reach the back office from anywhere (the authenticator code is still required). Add your office or home network to lock it down."}
      </p>
      <p className="mt-3 rounded-lg bg-wash p-3 text-[14px]">
        You&apos;re connecting from <span className="font-mono font-semibold">{d.your_ip}</span>.
        {d.rules.length === 0 && " The first network you add must include this address, so you can't lock yourself out."}
      </p>

      <div className="mt-5 overflow-x-auto">
        <table className="w-full min-w-[520px] text-[14px]">
          <thead><tr className="border-b border-line text-left text-[13px] text-muted">
            <th className="py-2 pr-3">Network</th><th className="py-2 pr-3">Label</th><th className="py-2 pr-3">Added</th><th className="py-2" />
          </tr></thead>
          <tbody>
            {d.rules.map((x) => (
              <tr key={x.id} className="border-b border-wash">
                <td className="py-2 pr-3 font-mono">{x.cidr}</td>
                <td className="py-2 pr-3">{x.label || "—"}</td>
                <td className="py-2 pr-3 text-muted"><span suppressHydrationWarning>{when(x.created_at)}</span>{x.created_by ? ` · ${x.created_by}` : ""}</td>
                <td className="py-2 text-right"><DeleteButton endpoint={`/v1/admin/ip-rules/${x.id}`} label="Remove" /></td>
              </tr>
            ))}
          </tbody>
        </table>
        {d.rules.length === 0 && <p className="py-4 text-[14px] text-muted">No networks yet.</p>}
      </div>

      <section className="mt-6 rounded-lg border border-line p-4">
        <h2 className="font-bold">Add a network</h2>
        <div className="mt-2">
          <ActionForm endpoint="/v1/admin/ip-rules" submit="Add network" done="Added."
            fields={[
              { name: "cidr", label: "IP address or range", placeholder: d.your_ip || "203.0.113.0/24", initial: d.rules.length === 0 ? d.your_ip : "",
                hint: "A single address, or a range like 203.0.113.0/24. Home internet addresses can change; check here if you get locked out." },
              { name: "label", label: "Label", placeholder: "Office, John's home" },
            ]} />
        </div>
      </section>
      <p className="mt-4 text-[13px] text-muted">
        Locked out? Set ADMIN_IP_ALLOWLIST=off on the API service in Render, sign in, fix the list, then remove the setting.
      </p>
    </div>
  );
}
