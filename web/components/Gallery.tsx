"use client";

import { useState } from "react";

export default function Gallery({ photos, title }: { photos: string[]; title: string }) {
  const [i, setI] = useState(0);
  if (photos.length === 0) {
    return <div className="flex aspect-[5/7] max-h-[640px] items-center justify-center rounded-lg bg-wash text-muted">No photo</div>;
  }
  return (
    <div className="flex flex-col-reverse gap-3 sm:flex-row">
      {photos.length > 1 && (
        <ul className="flex gap-2 sm:flex-col" aria-label="Photos">
          {photos.map((p, n) => (
            <li key={p}>
              <button
                type="button"
                onClick={() => setI(n)}
                aria-label={`Show photo ${n + 1} of ${photos.length}`}
                aria-current={n === i ? "true" : undefined}
                className={`block h-[84px] w-[60px] overflow-hidden rounded border-2 bg-wash ${n === i ? "border-ink" : "border-transparent hover:border-line"}`}
              >
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img src={p} alt="" className="h-full w-full object-contain" />
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="flex flex-1 items-center justify-center rounded-lg bg-wash p-4">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src={photos[i]}
          alt={`${title}, photo ${i + 1} of ${photos.length}`}
          className="max-h-[600px] w-auto max-w-full object-contain"
        />
      </div>
    </div>
  );
}
