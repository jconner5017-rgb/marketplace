export function StatusBadge({ status }: { status: string }) {
  const tone: Record<string, string> = {
    active: "bg-ship/10 text-ship", frozen: "bg-[#e6f0ff] text-link", suspended: "bg-[#fff4e0] text-[#8a5300]", banned: "bg-slab/10 text-slab",
    pending: "bg-wash text-muted", approved: "bg-ship/10 text-ship", rejected: "bg-slab/10 text-slab",
  };
  return <span className={`inline-block rounded-full px-2 py-0.5 text-[12px] font-semibold capitalize ${tone[status] ?? "bg-wash"}`}>{status}</span>;
}

export const ROLE: Record<string, string> = {
  super_admin: "Super Admin", moderator: "Moderator", support: "Support Agent", auditor: "Financial Auditor",
};

export function when(iso: string | null | undefined): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString("en-US", { month: "short", day: "numeric", year: "numeric", hour: "numeric", minute: "2-digit" });
}
