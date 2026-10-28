/**
 * One measure per day as thin bars (single series, so no legend: the title
 * names it). Each bar has a hover title, and the same numbers are in a table.
 */
export default function DailyBars({ title, days, values, format }: {
  title: string; days: string[]; values: number[]; format: (n: number) => string;
}) {
  const W = 560, H = 120, pad = 2;
  const max = Math.max(1, ...values);
  const bw = W / Math.max(1, values.length);
  const total = values.reduce((a, b) => a + b, 0);
  const label = (d: string) => new Date(d + "T12:00:00").toLocaleDateString("en-US", { month: "short", day: "numeric" });
  return (
    <figure className="rounded-lg border border-line p-4">
      <figcaption className="flex items-baseline justify-between gap-2">
        <span className="text-[14px] font-semibold">{title}</span>
        <span className="num text-[13px] text-muted">{format(total)} total</span>
      </figcaption>
      <svg viewBox={`0 0 ${W} ${H + 18}`} role="img" aria-label={`${title}, ${format(total)} over ${values.length} days`} className="mt-2 w-full">
        <line x1="0" x2={W} y1={H} y2={H} stroke="var(--color-line, #e5e5e5)" strokeWidth="1" />
        {values.map((v, i) => {
          const h = v === 0 ? 0 : Math.max(3, (v / max) * (H - 4));
          const x = i * bw + pad / 2, w = Math.max(1, bw - pad);
          return (
            <g key={days[i]}>
              {/* hit target is the whole column, bigger than the bar */}
              <rect x={i * bw} y={0} width={bw} height={H} fill="transparent"><title>{`${label(days[i])}: ${format(v)}`}</title></rect>
              {h > 0 && (
                <path d={`M${x},${H} V${H - h + Math.min(4, h)} a${Math.min(4, w / 2)},${Math.min(4, w / 2)} 0 0 1 ${Math.min(4, w / 2)},-${Math.min(4, w / 2)} H${x + w - Math.min(4, w / 2)} a${Math.min(4, w / 2)},${Math.min(4, w / 2)} 0 0 1 ${Math.min(4, w / 2)},${Math.min(4, w / 2)} V${H} Z`}
                  className="fill-link" pointerEvents="none" />
              )}
            </g>
          );
        })}
        <text x="0" y={H + 14} fontSize="11" className="fill-muted">{days[0] && label(days[0])}</text>
        <text x={W} y={H + 14} fontSize="11" textAnchor="end" className="fill-muted">{days.length > 0 && label(days[days.length - 1])}</text>
      </svg>
      <details className="mt-1 text-[12px]">
        <summary className="cursor-pointer text-muted">Show as a table</summary>
        <table className="mt-1 w-full">
          <tbody>
            {days.map((d, i) => (
              <tr key={d} className="border-b border-wash"><td className="py-0.5">{label(d)}</td><td className="num py-0.5 text-right">{format(values[i])}</td></tr>
            ))}
          </tbody>
        </table>
      </details>
    </figure>
  );
}
