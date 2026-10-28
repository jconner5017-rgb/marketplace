"use client";

import { useState } from "react";
import ActionForm from "./ActionForm";

/** Add a card feature: the key becomes part of the URL. */
export default function AddFeatureForm() {
  const [key, setKey] = useState("");
  return (
    <details className="mt-3 max-w-[520px] rounded-lg border border-line p-4">
      <summary className="cursor-pointer font-semibold">Add a feature</summary>
      <label className="mt-3 block text-[13px] font-semibold">
        Key (lowercase, underscores)
        <input value={key} onChange={(e) => setKey(e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, "_"))} placeholder="gold_border"
          className="mt-1 w-full rounded-md border border-line px-3 py-2 font-normal" />
      </label>
      <div className="mt-3">
        <ActionForm
          endpoint={`/v1/admin/catalog/features/${key || "_"}`}
          method="PUT"
          fields={[{ name: "label", label: "Label", placeholder: "Gold border" }, { name: "sort", label: "Order", initial: "100" }]}
          extra={{ active: true }}
          transform={(v) => ({ ...v, sort: Number(v.sort) || 0 })}
          submit="Add feature"
          done="Added."
        />
      </div>
    </details>
  );
}
