"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { send } from "@/lib/client";
import { ago } from "@/lib/format";
import type { Bell } from "@/lib/types";
import { BellIcon } from "./Icons";

const KIND: Record<string, string> = {
  announcement: "From Slabs & Singles",
  order: "Order",
  auction: "Auction",
  offer: "Offer",
  saved_search: "Saved search",
  feedback: "Feedback",
  case: "Case",
  message: "Message",
};

export default function BellMenu({ unread }: { unread: number }) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [bell, setBell] = useState<Bell | null>(null);
  const [count, setCount] = useState(unread);
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => setCount(unread), [unread]);

  useEffect(() => {
    if (!open) return;
    send<Bell>("GET", "/v1/notifications").then((r) => {
      if (r.ok) {
        setBell(r.data);
        setCount(r.data.unread);
      }
    });
    const close = (e: MouseEvent) => {
      if (box.current && !box.current.contains(e.target as Node)) setOpen(false);
    };
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", esc);
    };
  }, [open]);

  // Mark one notice read as it's opened. keepalive lets the request finish
  // even though the page is navigating away.
  function readOne(id: string) {
    const was = bell?.notices.find((n) => n.id === id);
    if (!was || was.read) return;
    setCount((c) => Math.max(0, c - 1));
    setBell((b) => (b ? { unread: Math.max(0, b.unread - 1), notices: b.notices.map((n) => (n.id === id ? { ...n, read: true } : n)) } : b));
    fetch(`/api/v1/notifications/${encodeURIComponent(id)}/read`, {
      method: "POST", headers: { "content-type": "application/json" }, body: "{}", keepalive: true,
    }).then(() => router.refresh()).catch(() => {});
  }

  async function readAll() {
    await send("POST", "/v1/notifications/read-all", {});
    setCount(0);
    setBell((b) => (b ? { unread: 0, notices: b.notices.map((n) => ({ ...n, read: true })) } : b));
    router.refresh();
  }

  return (
    <div ref={box} className="relative">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        aria-label={count > 0 ? `Notifications, ${count} unread` : "Notifications"}
        className="relative flex items-center hover:text-link"
      >
        <BellIcon />
        {count > 0 && (
          <span className="num absolute -right-2 -top-1.5 min-w-[18px] rounded-full bg-slab px-1 text-center text-[11px] font-bold leading-[18px] text-white">
            {count > 99 ? "99+" : count}
          </span>
        )}
      </button>
      {open && (
        <div className="absolute right-0 top-full z-30 mt-3 w-[min(360px,90vw)] rounded-lg border border-line bg-paper shadow-lg">
          <div className="flex items-center justify-between border-b border-line px-4 py-2.5">
            <h2 className="text-[15px] font-bold">Notifications</h2>
            {count > 0 && (
              <button type="button" onClick={readAll} className="text-[13px] text-link hover:underline">Mark all read</button>
            )}
          </div>
          {!bell ? (
            <p className="px-4 py-6 text-center text-[13px] text-muted">Loading…</p>
          ) : bell.notices.length === 0 ? (
            <p className="px-4 py-6 text-center text-[13px] text-muted">
              Nothing yet. Messages from buyers and sellers, and news from us, show up here.
            </p>
          ) : (
            <ul className="max-h-[420px] overflow-y-auto">
              {bell.notices.map((n) => {
                const inner = (
                  <>
                    <span className="flex items-start gap-2">
                      {!n.read && <span className="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-link" aria-label="Unread" />}
                      <span className="min-w-0">
                        <span className="block text-[12px] font-semibold text-muted">{KIND[n.kind] ?? "Notice"}</span>
                        <span className="block text-[14px] font-semibold leading-snug">{n.title}</span>
                        {n.body && <span className="mt-0.5 line-clamp-2 block text-[13px] text-muted">{n.body}</span>}
                        <span className="mt-0.5 block text-[12px] text-muted">{ago(n.created_at)}</span>
                      </span>
                    </span>
                  </>
                );
                return (
                  <li key={n.id} className={`border-b border-wash last:border-0 ${n.read ? "" : "bg-link/5"}`}>
                    {n.link ? (
                      <Link href={n.link} onClick={() => { readOne(n.id); setOpen(false); }} className="block px-4 py-3 hover:bg-wash">{inner}</Link>
                    ) : (
                      <button type="button" onClick={() => readOne(n.id)} className="block w-full px-4 py-3 text-left hover:bg-wash">{inner}</button>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
          <Link href="/messages" onClick={() => setOpen(false)} className="block border-t border-line px-4 py-2.5 text-center text-[14px] font-semibold text-link hover:bg-wash">
            Go to messages
          </Link>
        </div>
      )}
    </div>
  );
}
