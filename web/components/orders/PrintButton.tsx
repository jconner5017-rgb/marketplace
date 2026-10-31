"use client";

export default function PrintButton({ label = "Print" }: { label?: string }) {
  return (
    <button type="button" onClick={() => window.print()}
      className="rounded-full bg-link px-5 py-2 text-[14px] font-semibold text-white hover:bg-link-dark">
      {label}
    </button>
  );
}
