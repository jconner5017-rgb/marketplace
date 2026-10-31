"use client";

import { useState } from "react";
import { send } from "@/lib/client";

export default function EmailPrefs({ announcements }: { announcements: boolean }) {
  const [on, setOn] = useState(announcements);
  const [note, setNote] = useState("");
  async function toggle(v: boolean) {
    setOn(v);
    setNote("");
    const r = await send("PUT", "/v1/me/email-prefs", { announcements: v });
    if (!r.ok) {
      setOn(!v);
      return setNote(r.message);
    }
    setNote("Saved.");
  }
  return (
    <div className="mt-5 rounded-lg border border-line p-4">
      <label className="flex items-start gap-3 text-[15px]">
        <input type="checkbox" checked={on} onChange={(e) => toggle(e.target.checked)} className="mt-1" />
        <span>
          <span className="font-semibold">Announcements</span>
          <span className="block text-[14px] text-muted">News from Slabs &amp; Singles: new features, sales events and policy changes.</span>
        </span>
      </label>
      {note && <p className="mt-2 text-[13px] text-muted" role="status">{note}</p>}
    </div>
  );
}
