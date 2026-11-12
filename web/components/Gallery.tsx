"use client";

import { useCallback, useEffect, useRef, useState, type TouchEvent } from "react";

/** Listing photos: thumbnails down the left, a large framed photo with
 *  previous/next arrows, and a full-screen view (expand button, or click the
 *  photo). Arrow keys and swiping move between photos. */
export default function Gallery({ photos, title }: { photos: string[]; title: string }) {
  const [i, setI] = useState(0);
  const [full, setFull] = useState(false);
  const touchX = useRef<number | null>(null);
  const n = photos.length;
  const go = useCallback((d: number) => setI((x) => (x + d + n) % n), [n]);

  useEffect(() => {
    if (!full) return;
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") setFull(false);
      if (e.key === "ArrowLeft") go(-1);
      if (e.key === "ArrowRight") go(1);
    };
    document.addEventListener("keydown", key);
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => { document.removeEventListener("keydown", key); document.body.style.overflow = overflow; };
  }, [full, go]);

  if (n === 0) {
    return <div className="flex aspect-[4/3] items-center justify-center rounded-2xl bg-wash text-muted">No photo</div>;
  }

  const swipe = {
    onTouchStart: (e: TouchEvent) => { touchX.current = e.touches[0].clientX; },
    onTouchEnd: (e: TouchEvent) => {
      if (touchX.current === null) return;
      const dx = e.changedTouches[0].clientX - touchX.current;
      touchX.current = null;
      if (Math.abs(dx) > 40) go(dx < 0 ? 1 : -1);
    },
  };
  const round = "flex h-11 w-11 items-center justify-center rounded-full bg-white text-ink shadow-md ring-1 ring-black/5 hover:bg-wash";
  const arrow = (d: -1 | 1) => (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d={d < 0 ? "M15 18l-6-6 6-6" : "M9 6l6 6-6 6"} />
    </svg>
  );

  return (
    <div className="flex flex-col-reverse gap-3 sm:flex-row">
      {n > 1 && (
        <ul className="flex gap-2 overflow-x-auto sm:flex-col sm:overflow-visible" aria-label="Photos">
          {photos.map((p, k) => (
            <li key={p + k} className="flex-none">
              <button
                type="button"
                onClick={() => setI(k)}
                aria-label={`Show photo ${k + 1} of ${n}`}
                aria-current={k === i ? "true" : undefined}
                className={`block h-[84px] w-[84px] overflow-hidden rounded-xl bg-wash p-1 ${k === i ? "ring-2 ring-ink" : "ring-1 ring-line hover:ring-ink/50"}`}
              >
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img src={p} alt="" className="h-full w-full rounded-lg object-contain" />
              </button>
            </li>
          ))}
        </ul>
      )}

      <div className="relative flex-1 overflow-hidden rounded-2xl bg-wash" {...swipe}>
        <button type="button" onClick={() => setFull(true)} aria-label="View photo full screen"
          className="flex aspect-[4/3] w-full cursor-zoom-in items-center justify-center p-3 lg:aspect-auto lg:h-[min(72vh,640px)]">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src={photos[i]} alt={`${title}, photo ${i + 1} of ${n}`} className="h-full w-full object-contain" />
        </button>
        <button type="button" onClick={() => setFull(true)} aria-label="Full screen" className={`${round} absolute right-4 top-4`}>
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7" />
          </svg>
        </button>
        {n > 1 && (
          <>
            <button type="button" onClick={() => go(-1)} aria-label="Previous photo" className={`${round} absolute left-4 top-1/2 -translate-y-1/2`}>{arrow(-1)}</button>
            <button type="button" onClick={() => go(1)} aria-label="Next photo" className={`${round} absolute right-4 top-1/2 -translate-y-1/2`}>{arrow(1)}</button>
            <span className="num absolute bottom-3 left-1/2 -translate-x-1/2 rounded-full bg-black/60 px-2.5 py-0.5 text-[12px] text-white">{i + 1} / {n}</span>
          </>
        )}
      </div>

      {full && (
        <div role="dialog" aria-modal="true" aria-label={`${title} photos`} className="fixed inset-0 z-50 flex flex-col bg-black/95" {...swipe}>
          <div className="flex items-center justify-between p-3 text-white">
            <span className="num text-[14px]">{i + 1} / {n}</span>
            <button type="button" onClick={() => setFull(false)} aria-label="Close" className="flex h-11 w-11 items-center justify-center rounded-full hover:bg-white/10">
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18" /></svg>
            </button>
          </div>
          <div className="relative flex min-h-0 flex-1 items-center justify-center px-4 pb-6" onClick={(e) => { if (e.target === e.currentTarget) setFull(false); }}>
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={photos[i]} alt={`${title}, photo ${i + 1} of ${n}`} className="max-h-full max-w-full object-contain" />
            {n > 1 && (
              <>
                <button type="button" onClick={() => go(-1)} aria-label="Previous photo" className={`${round} absolute left-4 top-1/2 -translate-y-1/2`}>{arrow(-1)}</button>
                <button type="button" onClick={() => go(1)} aria-label="Next photo" className={`${round} absolute right-4 top-1/2 -translate-y-1/2`}>{arrow(1)}</button>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
