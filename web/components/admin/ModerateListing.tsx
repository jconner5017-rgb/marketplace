"use client";

import { useState } from "react";
import ActionForm from "./ActionForm";

/** Take a listing down (or put it back), with a reason. */
export default function ModerateListing({ id, action }: { id: string; action: "remove" | "restore" }) {
  const [open, setOpen] = useState(false);
  if (!open) {
    return (
      <button type="button" onClick={() => setOpen(true)}
        className={`rounded-full border px-4 py-1.5 text-[13px] font-semibold ${action === "remove" ? "border-slab text-slab hover:bg-slab/5" : "border-link text-link hover:bg-wash"}`}>
        {action === "remove" ? "Take down" : "Restore"}
      </button>
    );
  }
  return (
    <ActionForm
      endpoint={`/v1/admin/listings/${id}/moderate`}
      fields={[{ name: "reason", label: action === "remove" ? "Reason (sent to the seller)" : "Reason", type: "text",
        placeholder: action === "remove" ? "Counterfeit slab label" : "Seller showed proof" }]}
      extra={{ action }}
      submit={action === "remove" ? "Take down" : "Restore"}
      danger={action === "remove"}
      done="Done."
      compact
    />
  );
}
