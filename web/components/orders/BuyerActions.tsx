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

const SUBS = [
  ["item_rating", "Item matched the description"],
  ["shipping_rating", "Shipping speed and packaging"],
  ["communication_rating", "Communication"],
] as const;

function msg(m: string) {
  return m.charAt(0).toUpperCase() + m.slice(1) + ".";
}

function StarPicker({ name, label, value, onChange }: { name: string; label: string; value: number; onChange: (v: number) => void }) {
  return (
    <fieldset className="flex flex-wrap items-center justify-between gap-2">
      <legend className="sr-only">{label}</legend>
      <span aria-hidden>{label}</span>
      <span className="flex">
        {[1, 2, 3, 4, 5].map((n) => (
          <label key={n} className="cursor-pointer px-0.5 text-[22px] leading-none">
            <input type="radio" name={name} value={n} checked={value === n} onChange={() => onChange(n)} className="sr-only" />
            <span aria-hidden className={n <= value ? "text-[#b7791f]" : "text-line"}>★</span>
            <span className="sr-only">{n} out of 5</span>
          </label>
        ))}
      </span>
    </fieldset>
  );
}

/** Messages, delivery, feedback and report-a-problem controls on an order. */
export default function BuyerActions({ o }: { o: Order }) {
  const router = useRouter();
  const buying = o.side === "buying";
  const mine = o.my_feedback;
  const canRevise = buying && mine && (mine.state === "held" || mine.state === "mediation") && mine.rating < 1;
  const [open, setOpen] = useState<"" | "feedback" | "problem" | "message">("");
  const [rating, setRating] = useState<1 | 0 | -1>(1);
  const [subs, setSubs] = useState<Record<string, number>>({ item_rating: 0, shipping_rating: 0, communication_rating: 0 });
  const [comment, setComment] = useState("");
  const [kind, setKind] = useState(o.status === "paid" ? "other" : "inr");
  const [summary, setSummary] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [needContact, setNeedContact] = useState(false);
  const [done, setDone] = useState("");

  const other = buying ? o.seller : o.buyer;
  const shipped = ["shipped", "delivered", "refunded", "partially_refunded"].includes(o.status);
  const canFeedback = (!mine && (buying ? shipped : o.status !== "cancelled")) || canRevise;
  const hasOpenCase = o.cases.some((c) => ["open", "waiting_seller", "waiting_buyer"].includes(c.status));
  const canReport = buying && !hasOpenCase && ["paid", "shipped", "delivered"].includes(o.status);

  async function post<T>(path: string, body: unknown): Promise<{ ok: true; data: T } | { ok: false; status: number; message: string } > {
    setBusy(true);
    setError("");
    const r = await send<T>("POST", `/v1/orders/${o.id}/${path}`, body);
    setBusy(false);
    return r.ok ? { ok: true, data: r.data } : { ok: false, status: r.status, message: r.message };
  }

  async function leaveFeedback(e: React.FormEvent) {
    e.preventDefault();
    if (buying && Object.values(subs).some((v) => v < 1)) return setError("Rate all three: item, shipping and communication.");
    const r = await post<{ result: { note: string } }>("feedback", { rating, comment, ...(buying ? subs : {}) });
    if (!r.ok) {
      if (r.status === 409 && /message the seller/.test(r.message)) {
        setNeedContact(true);
        return;
      }
      return setError(msg(r.message));
    }
    setOpen("");
    setNeedContact(false);
    setDone(r.data.result.note || "Feedback saved.");
    router.refresh();
  }

  async function sendMessage(e: React.FormEvent) {
    e.preventDefault();
    const r = await post<{ thread_id: string }>("messages", { body: message });
    if (!r.ok) return setError(msg(r.message));
    setMessage("");
    setOpen("");
    setNeedContact(false);
    setDone(`Message sent to ${other}. You'll get a notification when they reply.`);
    router.refresh();
  }

  async function report(e: React.FormEvent) {
    e.preventDefault();
    const r = await post<{ case_number: number }>("problem", { kind, summary });
    if (!r.ok) return setError(msg(r.message));
    setOpen("");
    setNeedContact(false);
    setDone(`Case #${r.data.case_number} is open. Our support team will follow up by notification.`);
    router.refresh();
  }

  const btn = "rounded-full px-4 py-2 text-[14px] font-semibold disabled:opacity-60";
  const box = "rounded-lg border border-line p-4 text-[14px]";
  const ratings: [1 | 0 | -1, string][] = buying ? [[1, "Positive"], [0, "Neutral"], [-1, "Negative"]] : [[1, "Positive"], [0, "Neutral"]];

  return (
    <div className="mt-4 space-y-3">
      <div className="flex flex-wrap gap-2">
        {buying && o.status === "shipped" && (
          <button type="button" disabled={busy} onClick={async () => { const r = await post("delivered", {}); if (r.ok) { setDone("Thanks for letting us know."); router.refresh(); } else setError(msg(r.message)); }}
            className={`${btn} bg-link text-white hover:bg-link-dark`}>I got it</button>
        )}
        {canFeedback && (
          <button type="button" onClick={() => setOpen(open === "feedback" ? "" : "feedback")} className={`${btn} border border-link text-link hover:bg-wash`}>
            {canRevise ? "Change your feedback" : "Leave feedback"}
          </button>
        )}
        <button type="button" onClick={() => setOpen(open === "message" ? "" : "message")} className={`${btn} border border-link text-link hover:bg-wash`}>
          Message {other}
        </button>
        {canReport && (
          <button type="button" onClick={() => setOpen(open === "problem" ? "" : "problem")} className={`${btn} text-link hover:bg-wash`}>
            Report a problem
          </button>
        )}
      </div>

      {o.their_feedback_hidden && !mine && (
        <p className="text-[13px] text-muted">{other} has left you feedback. You&apos;ll see it once you leave yours, or when the feedback window closes.</p>
      )}
      {mine && mine.state === "held" && mine.held_until && (
        <p className="text-[13px]">Your negative locks on {new Date(mine.held_until).toLocaleDateString("en-US", { month: "short", day: "numeric" })}. If the seller puts things right, you can change it before then.</p>
      )}
      {mine && mine.state === "mediation" && (
        <p className="text-[13px]">Your feedback is with our support team. This is a newer seller&apos;s first negative, so we look at it before it shows.</p>
      )}

      {open === "feedback" && (
        <form onSubmit={leaveFeedback} className={`${box} space-y-3`}>
          <fieldset>
            <legend className="font-semibold">How did it go with {other}?</legend>
            <div className="mt-2 flex flex-wrap gap-4">
              {ratings.filter(([v]) => !canRevise || v > (mine?.rating ?? -1)).map(([v, l]) => (
                <label key={v} className="flex items-center gap-1.5">
                  <input type="radio" name="rating" checked={rating === v} onChange={() => setRating(v)} /> {l}
                </label>
              ))}
            </div>
            {!buying && <p className="mt-1 text-[12px] text-muted">Sellers can leave positive or neutral feedback. Problems with a buyer go to our support team instead.</p>}
          </fieldset>
          {buying && (
            <div className="space-y-1 rounded-md bg-wash p-3">
              {SUBS.map(([k, l]) => <StarPicker key={k} name={k} label={l} value={subs[k]} onChange={(v) => setSubs({ ...subs, [k]: v })} />)}
            </div>
          )}
          <label className="block">
            <span className="font-semibold">Comment{buying && rating < 1 ? "" : " (optional)"}</span>
            <textarea value={comment} onChange={(e) => setComment(e.target.value)} maxLength={500} rows={2}
              className="mt-1 w-full rounded-md border border-line px-3 py-2 outline-none focus:border-ink" />
          </label>
          <p className="text-[12px] text-muted">
            Your feedback stays hidden from {other} until they leave theirs (or the window closes), so neither side can hold a rating hostage.
            {buying && rating < 0 ? " Negatives wait 3 days before they lock, so the seller has a chance to fix things." : ""}
          </p>
          {needContact && (
            <div role="alert" className="rounded-md border border-ink p-3">
              <p className="font-semibold">Give the seller a chance to fix it first</p>
              <p className="mt-1">Before a negative, message {other} or report the problem. Most issues get sorted out quickly.</p>
              <div className="mt-2 flex flex-wrap gap-2">
                <button type="button" onClick={() => setOpen("message")} className={`${btn} bg-link text-white`}>Message {other}</button>
                {canReport && <button type="button" onClick={() => setOpen("problem")} className={`${btn} border border-link text-link`}>Report a problem</button>}
              </div>
            </div>
          )}
          <button type="submit" disabled={busy} className={`${btn} bg-link text-white hover:bg-link-dark`}>{canRevise ? "Update feedback" : "Post feedback"}</button>
        </form>
      )}

      {open === "message" && (
        <form onSubmit={sendMessage} className={box}>
          <label className="block">
            <span className="font-semibold">Message {other} about order #{o.number}</span>
            <textarea value={message} onChange={(e) => setMessage(e.target.value)} maxLength={2000} rows={3} required
              className="mt-1 w-full rounded-md border border-line px-3 py-2 outline-none focus:border-ink" />
          </label>
          <p className="mt-1 text-[12px] text-muted">Keep it on the site: contact details and other payment apps are blocked to protect you.</p>
          <button type="submit" disabled={busy} className={`${btn} mt-2 bg-link text-white hover:bg-link-dark`}>Send message</button>
        </form>
      )}

      {open === "problem" && (
        <form onSubmit={report} className={box}>
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
