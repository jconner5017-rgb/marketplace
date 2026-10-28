import { notFound } from "next/navigation";
import { requireUser } from "@/lib/auth";
import AnnounceForm from "@/components/AnnounceForm";

export const dynamic = "force-dynamic";
export const metadata = { title: "Post an announcement" };

export default async function AnnouncePage() {
  const me = await requireUser("/admin/announce");
  if (me.role !== "admin") notFound();
  return (
    <div className="mx-auto max-w-[640px] pt-8">
      <h1 className="text-[26px] font-bold">Post an announcement</h1>
      <p className="mt-1 text-muted">Shows in every member&apos;s notification bell and on the Announcements page.</p>
      <AnnounceForm />
    </div>
  );
}
