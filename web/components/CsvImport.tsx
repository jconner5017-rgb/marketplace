"use client";

import Link from "next/link";
import { useState } from "react";
import { send } from "@/lib/client";

type Row = { row: number; status: string; listing_id?: string; errors?: Record<string, string> };
type Result = { created: number; updated: number; failed: number; rows: Row[] };

export default function CsvImport() {
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<Result | null>(null);

  async function upload(e: React.FormEvent) {
    e.preventDefault();
    if (!file) return;
    setBusy(true);
    setError("");
    setResult(null);
    const text = await file.text();
    const res = await send<Result>("POST", "/v1/seller/listings/import", text, "text/csv");
    setBusy(false);
    if (!res.ok) {
      setError(res.message.charAt(0).toUpperCase() + res.message.slice(1));
      return;
    }
    setResult(res.data);
  }

  const failed = result?.rows.filter((r) => r.status === "error") ?? [];
  return (
    <div className="mt-6">
      <form onSubmit={upload} className="flex flex-wrap items-center gap-3 rounded-lg border-2 border-dashed border-line p-6">
        <label className="flex-1">
          <span className="sr-only">CSV file</span>
          <input type="file" accept=".csv,text/csv" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
        </label>
        <button type="submit" disabled={!file || busy} className="rounded-full bg-link px-6 py-2 font-semibold text-white hover:bg-link-dark disabled:opacity-50">
          {busy ? "Uploading…" : "Upload and list"}
        </button>
      </form>

      {error && <p role="alert" className="mt-4 rounded-md border border-slab/40 bg-slab/5 px-3 py-2 text-slab">{error}</p>}

      {result && (
        <div className="mt-6" role="status">
          <p className="text-[17px]">
            <strong className="text-ship">{result.created} listed</strong>, <strong>{result.updated} updated</strong>
            {result.failed > 0 && <>, <strong className="text-slab">{result.failed} need fixing</strong></>}.
          </p>
          {failed.length > 0 && (
            <>
              <p className="mt-1 text-[14px] text-muted">Fix these rows in your file and upload it again. Rows that worked stay listed.</p>
              <table className="mt-3 w-full text-[14px]">
                <thead>
                  <tr className="border-b border-line text-left text-[13px] text-muted">
                    <th className="py-2 pr-4 font-semibold">Row</th>
                    <th className="py-2 font-semibold">What to fix</th>
                  </tr>
                </thead>
                <tbody>
                  {failed.map((r) => (
                    <tr key={r.row} className="border-b border-wash align-top">
                      <td className="num py-2 pr-4">{r.row}</td>
                      <td className="py-2">
                        <ul>
                          {Object.entries(r.errors ?? {}).map(([k, v]) => (
                            <li key={k}><code className="text-[13px]">{k}</code>: {v}</li>
                          ))}
                        </ul>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}
          <Link href="/sell" className="mt-4 inline-block text-link underline">Back to your listings</Link>
        </div>
      )}
    </div>
  );
}
