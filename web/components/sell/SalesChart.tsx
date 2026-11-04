"use client";

import { useState } from "react";

type Day = { day: string; cents: number; orders: number };

const fmt = (c: number) => "$" + (c / 100).toLocaleString("en-US", { minimumFractionDigits: c % 100 ? 2 : 0, maximumFractionDigits: 2 });
const short = (d: string) => new Date(d + "T12:00:00").toLocaleDateString("en-US", { month: "short", day: "numeric" });

/** Sales per day for the last 31 days: thin bars, hover for the day's total. */
export default function SalesChart({ days }: { days: Day[] }) {
  const [hover, setHover] = useState<number | null>(null);
  const W = 620, H = 160, L = 44, B = 22, T = 8;
  const max = Math.max(...days.map((d) => d.cents), 0);
  // A round axis top: 1, 2 or 5 times a power of ten (in dollars).
  const top = (() => {
    if (max <= 0) return 1000;
    const dollars = max / 100, p = Math.pow(10, Math.floor(Math.log10(dollars)));
    const m = [1, 2, 5, 10].find((x) => x * p >= dollars) ?? 10;
    return m * p * 100;
  })();
  const plotW = W - L - 4, plotH = H - T - B;
  const step = plotW / days.length, bw = Math.max(2, step - 2);
  const y = (c: number) => T + plotH - (c / top) * plotH;
  const ticks = [0, top / 2, top];
  const h = hover != null ? days[hover] : null;

  return (
    <div className="relative">
      <svg viewBox={`0 0 ${W} ${H}`} className="block h-auto w-full" role="img"
        aria-label={`Sales by day for the last ${days.length} days`} onMouseLeave={() => setHover(null)}>
        {ticks.map((t) => (
          <g key={t}>
            <line x1={L} x2={W - 4} y1={y(t)} y2={y(t)} stroke="var(--color-line)" strokeWidth={t === 0 ? 1 : 0.6} strokeDasharray={t === 0 ? undefined : "2 3"} />
            <text x={L - 6} y={y(t) + 3.5} textAnchor="end" fontSize="10" fill="var(--color-muted)">{fmt(t)}</text>
          </g>
        ))}
        {days.map((d, i) => {
          const x = L + i * step + (step - bw) / 2;
          const hgt = Math.max(0, y(0) - y(d.cents));
          const r = Math.min(4, bw / 2, hgt);
          return (
            <g key={d.day}>
              {hgt > 0 && (
                <path fill={hover === i ? "var(--color-link-dark)" : "var(--color-link)"}
                  d={`M${x},${y(0)} V${y(d.cents) + r} Q${x},${y(d.cents)} ${x + r},${y(d.cents)} H${x + bw - r} Q${x + bw},${y(d.cents)} ${x + bw},${y(d.cents) + r} V${y(0)} Z`} />
              )}
              {/* hit target: the whole column */}
              <rect x={L + i * step} y={T} width={step} height={plotH} fill="transparent" onMouseEnter={() => setHover(i)} />
            </g>
          );
        })}
        {[0, 7, 14, 21, 28, days.length - 1].filter((i, k, a) => i < days.length && a.indexOf(i) === k).map((i) => (
          <text key={i} x={L + i * step + step / 2} y={H - 6} textAnchor="middle" fontSize="10" fill="var(--color-muted)">{short(days[i].day)}</text>
        ))}
      </svg>
      {h && hover != null && (
        <div className="pointer-events-none absolute top-0 rounded-md border border-line bg-paper px-2.5 py-1.5 text-[12px] shadow-sm"
          style={{ left: `min(calc(${((L + hover * step) / W) * 100}% ), calc(100% - 140px))` }}>
          <div className="font-semibold">{short(h.day)}</div>
          <div className="num">{fmt(h.cents)} · {h.orders} {h.orders === 1 ? "order" : "orders"}</div>
        </div>
      )}
      {max === 0 && <p className="mt-1 text-center text-[13px] text-muted">No sales in the last 31 days yet.</p>}
    </div>
  );
}
