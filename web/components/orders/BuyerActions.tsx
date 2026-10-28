"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { send } from "@/lib/client";
import type { Order } from "@/lib/types";

const KINDS = [
  { value: "inr", label: "It hasn't arrived" },
  { value: "snad", label: "It's not as described" },
  { value: "other", label: "Something else" },
];

function msg(m: string) {
  return m.charAt(0).toUpperCase() + m.slice(1) + ".";
}

/** Delivered, feedback and report-a-problem controls on an order. */
export default function BuyerActions({ o }: { o: Order }) {
  const router = useRouter();
  const [open, setOpen] = useState<"" | "feedback" | "problem">("");
  const [rating, setRating] = useState<1 | 0 | -1>(1);
  const [comment, setComment] = useState("");
  const [kind, setKind] = useState(o.status === "paid" ? "other" : "inr");
  const [summary, setSummary] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState("");

  const buying = o.side === "buying";
  const canFeedback = !o.my_feedback && (buying ? o.status === "shipped" || o.status === "delivered" : o.status !== "cancelled");
  const hasOpenCase = o.cases.some((c) => ["open", "waiting_seller", "waiting_buyer"].includes(c.status));
  const canReport = buying && !hasOpenCase && ["paid", "shipped", "delivered"].includes(o.status);

  async function post(path: string, body: unknown, after: string) {
    setBusy(true);
    setError("");
    const r = await send<{ case_number?: number }>("POST", `/v1/orders/${o.id}/${path}`, body);
    setBusy(false);
    if (!r.ok) return setError(msg(r.message));
    setOpen("");
    setDone(r.data?.case_number ? `Case #${r.data.case_number} is open. Our support team will follow up by notification.` : after);
    router.refresh();
  }

  const btn = "rounded-full px-4 py-2 text-[14px] font-semibold disabled:opacity-60";
  return (
    <div className="mt-4 space-y-3">
      <div className="flex flex-wrap gap-2">
        {buying && o.status === "shipped" && (
          <button type="button" disabled={busy} onClick={() => post("delivered", {}, "Thanks for letting us know.")}
            className={`${btn} bg-link text-white hover:bg-link-dark`}>I got it</button>
        )}
        {canFeedback && (
          <button type="button" onClick={() => setOpen(open === "feedback" ? "" : "feedback")} className={`${btn} border border-link text-link hover:bg-wash`}>
            Leave feedback
          </button>
        )}
        {canReport && (
          <button type="button" onClick={() => setOpen(open === "problem" ? "" : "problem")} className={`${btn} text-link hover:bg-wash`}>
            Report a problem
          </button>
        )}
      </div>

      {open === "feedback" && (
        <form onSubmit={(e) => { e.preventDefault(); post("feedback", { rating, comment }, "Feedback posted."); }}
          className="rounded-lg border border-line p-4 text-[14px]">
          <fieldset>
            <legend className="font-semibold">How did it go with {buying ? o.seller : o.buyer}?</legend>
            <div className="mt-2 flex flex-wrap gap-4">
              {([[1, "Positive"], [0, "Neutral"], [-1, "Negative"]] as const).map(([v, l]) => (
                <label key={v} className="flex items-center gap-1.5">
                  <input type="radio" name="rating" checked={rating === v} onChange={() => setRating(v)} /> {l}
                </label>
              ))}
            </div>
          </fieldset>
          <label className="mt-3 block">
            <span className="font-semibold">Comment{rating < 1 ? "" : " (optional)"}</span>
            <textarea value={comment} onChange={(e) => setComment(e.target.value)} maxLength={500} rows={2}
              className="mt-1 w-full rounded-md border border-line px-3 py-2 outline-none focus:border-ink" />
          </label>
          <button type="submit" disabled={busy} className={`${btn} mt-2 bg-link text-white hover:bg-link-dark`}>Post feedback</button>
        </form>
      )}

      {open === "problem" && (
        <form onSubmit={(e) => { e.preventDefault(); post("problem", { kind, summary }, ""); }}
          className="rounded-lg border border-line p-4 text-[14px]">
          <p className="text-muted">Message the seller first if you can; most problems get sorted that way. This opens a case with our support team.</p>
          <fieldset className="mt-3">
            <legend className="font-semibold">What&apos;s wrong?</legend>
            <div className="mt-2 flex flex-wrap gap-4">
              {KINDS.map((k) => (
                <label key={k.value} className="flex items-center gap-1.5">
                  <input type="radio" name="kind" checked={kind === k.value} onChange={() => setKind(k.value)} /> {k.label}
                </label>
              ))}
            </div>
          </fieldset>
          <label className="mt-3 block">
            <span className="font-semibold">Tell us what happened</span>
            <textarea value={summary} onChange={(e) => setSummary(e.target.value)} maxLength={2000} rows={4} required minLength={10}
              className="mt-1 w-full rounded-md border border-line px-3 py-2 outline-none focus:border-ink" />
          </label>
          <button type="submit" disabled={busy} className={`${btn} mt-2 bg-link text-white hover:bg-link-dark`}>Open a case</button>
        </form>
      )}
      {error && <p role="alert" className="text-[13px] text-slab">{error}</p>}
      {done && <p role="status" className="text-[14px] text-ship">{done}</p>}
    </div>
  );
}
