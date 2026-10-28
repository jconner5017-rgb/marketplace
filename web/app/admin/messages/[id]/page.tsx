import Link from "next/link";
import { requireStaff } from "@/lib/staff";
import ThreadLog from "@/components/admin/ThreadLog";

export const dynamic = "force-dynamic";
export const metadata = { title: "Conversation" };

export default async function ThreadPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  await requireStaff(`/admin/messages/${id}`, "messages.view");
  return (
    <div className="max-w-[760px]">
      <Link href="/admin/messages" className="text-[13px] text-link hover:underline">← Message logs</Link>
      <h1 className="mt-2 text-[26px] font-bold">Conversation</h1>
      <ThreadLog id={id} />
    </div>
  );
}
