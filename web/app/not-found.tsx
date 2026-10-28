import Link from "next/link";

export default function NotFound() {
  return (
    <div className="py-20 text-center">
      <h1 className="text-[24px] font-bold">This page isn&apos;t here</h1>
      <p className="mt-2 text-muted">The listing may have sold or been removed.</p>
      <Link href="/search?sort=newest" className="mt-5 inline-block rounded-full bg-link px-5 py-2 font-semibold text-white">
        Browse new listings
      </Link>
    </div>
  );
}
