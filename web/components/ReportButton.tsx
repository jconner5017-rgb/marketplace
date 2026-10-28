"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";

const REASONS: [string, string][] = [
  ["counterfeit", "Counterfeit or fake slab"],
  ["misdescribed", "Not as described (condition, grade, photos)"],
  ["prohibited", "Prohibited item"],
  ["keyword_spam", "Misleading title or keyword spam"],
  ["off_platform", "Asked to pay or talk off the site"],
  ["shill_bidding", "Suspicious bidding"],
  ["abuse", "Harassment or abuse"],
  ["other", "Something else"],
];

/** A quiet "Report" link that opens a short form. */
export default function ReportButton({ type, id, label = "Report", signedIn, next }: {
  type: "listing" | "user" | "message" | "offer"; id: string; label?: string; signedIn: boolean; next: string;
}) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("counterfeit");
  const [details, setDetails] = useState("");
  const [state, setState] = useState<"" | "busy" | "done">("");
  const [error, setError] = useState("");

  if (state === "done") return <span role="status" className="text-[13px] text-ship">Thanks. Our team will review it.</span>;
  if (!open) {
    return (
      <button type="button" onClick={() => (signedIn ? setOpen(true) : router.push(`/signin?next=${encodeURIComponent(next)}`))}
        className="text-[13px] text-muted underline hover:text-ink">
        {label}
      </button>
    );
  }
  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setState("busy");
    setError("");
    const r = await send("POST", "/v1/reports", { target_type: type, target_id: id, reason, details });
    if (!r.ok) {
      setState("");
      return setError(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    }
    setState("done");
  }
  return (
    <form onSubmit={submit} className="mt-2 rounded-lg border border-line p-3 text-[14px]">
      <label className="block font-semibold">
        What&apos;s wrong?
        <select value={reason} onChange={(e) => setReason(e.target.value)} className="mt-1 w-full rounded-md border border-line px-2 py-1.5 font-normal">
          {REASONS.filter(([v]) => type === "listing" || v !== "keyword_spam").map(([v, l]) => <option key={v} value={v}>{l}</option>)}
        </select>
      </label>
      <label className="mt-2 block font-semibold">
        Details (optional)
        <textarea value={details} onChange={(e) => setDetails(e.target.value)} rows={2} maxLength={2000}
          className="mt-1 w-full rounded-md border border-line px-2 py-1.5 font-normal" />
      </label>
      {error && <p role="alert" className="mt-1 text-slab">{error}</p>}
      <div className="mt-2 flex gap-2">
        <button type="submit" disabled={state === "busy"} className="rounded-full bg-ink px-4 py-1.5 font-semibold text-white disabled:opacity-60">Send report</button>
        <button type="button" onClick={() => setOpen(false)} className="rounded-full px-3 py-1.5 font-semibold text-link hover:bg-wash">Cancel</button>
      </div>
      <p className="mt-2 text-[12px] text-muted">Reports are private. The member isn&apos;t told who reported them.</p>
    </form>
  );
}
