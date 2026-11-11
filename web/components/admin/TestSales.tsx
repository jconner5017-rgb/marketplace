"use client";

import Link from "next/link";
import { useState } from "react";
import { send } from "@/lib/client";

export type TestSale = { id: string; number: number; status: string };

const STATUS: Record<string, string> = { paid: "To ship", shipped: "Shipped", delivered: "Delivered" };

export default function TestSales({ initial }: { initial: TestSale[] }) {
  const [list, setList] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  async function create() {
    setBusy(true);
    setMsg("");
    const r = await send<{ test_sale: TestSale }>("POST", "/v1/admin/test-sales");
    setBusy(false);
    if (!r.ok) return setMsg(r.message.charAt(0).toUpperCase() + r.message.slice(1) + ".");
    setList([r.data.test_sale, ...list]);
    setMsg(`Test sale #${r.data.test_sale.number} is in your Seller hub orders.`);
  }

  async function remove(t: TestSale) {
    setBusy(true);
    setMsg("");
    const r = await send("DELETE", `/v1/admin/test-sales/${t.id}`);
    setBusy(false);
    if (!r.ok) return setMsg(r.message);
    setList(list.filter((x) => x.id !== t.id));
    setMsg(`Test sale #${t.number} deleted.`);
  }

  return (
    <div className="mt-6">
      <button type="button" onClick={create} disabled={busy} className="rounded-full bg-link px-6 py-2.5 font-semibold text-white hover:bg-link-dark disabled:opacity-60">
        {busy ? "Working…" : "Create a test sale"}
      </button>
      {msg && <p role="status" className="mt-3 text-[14px]">{msg}</p>}
      {list.length > 0 && (
        <table className="mt-5 w-full text-[14px]">
          <thead>
            <tr className="border-b border-line text-left text-[13px] text-muted">
              <th className="py-2 pr-3 font-semibold">Order</th>
              <th className="py-2 pr-3 font-semibold">Status</th>
              <th className="py-2" />
            </tr>
          </thead>
          <tbody>
            {list.map((t) => (
              <tr key={t.id} className="border-b border-wash">
                <td className="py-2 pr-3">
                  <Link href={`/sell/orders/${t.id}`} className="font-semibold text-link hover:underline">Order #{t.number}</Link>
                </td>
                <td className="py-2 pr-3">{STATUS[t.status] ?? t.status}</td>
                <td className="py-2 text-right">
                  <button type="button" onClick={() => remove(t)} disabled={busy} className="text-slab hover:underline disabled:opacity-60">Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
