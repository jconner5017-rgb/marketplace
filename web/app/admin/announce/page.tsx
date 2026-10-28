import { requireStaff } from "@/lib/staff";
import AnnounceForm from "@/components/AnnounceForm";

export const dynamic = "force-dynamic";
export const metadata = { title: "Post an announcement" };

export default async function AnnouncePage() {
  await requireStaff("/admin/announce", "announce");
  return (
    <div className="max-w-[640px]">
      <h1 className="text-[26px] font-bold">Post an announcement</h1>
      <p className="mt-1 text-muted">Shows in every member&apos;s notification bell and on the Announcements page.</p>
      <AnnounceForm />
    </div>
  );
}
