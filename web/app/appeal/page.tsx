import AppealForm from "@/components/AppealForm";

export const metadata = { title: "Appeal a suspension or ban", robots: { index: false } };

export default function AppealPage() {
  return (
    <div className="mx-auto max-w-[480px] pt-10">
      <h1 className="text-[26px] font-bold">Appeal a suspension or ban</h1>
      <p className="mt-2 text-muted">
        Sign in details confirm it&apos;s your account. A member of our team reviews every appeal, and you can appeal each decision once.
      </p>
      <AppealForm account />
    </div>
  );
}
