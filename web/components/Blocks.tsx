import Link from "next/link";
import type { Block } from "@/lib/help";

/** Renders help-center / blog blocks. */
export default function Blocks({ body }: { body: Block[] }) {
  return (
    <div className="space-y-4 text-[16px] leading-relaxed">
      {body.map((b, i) =>
        "h" in b ? <h2 key={i} className="pt-3 text-[21px] font-bold leading-snug">{b.h}</h2>
        : "list" in b ? <ul key={i} className="list-disc space-y-1.5 pl-6">{b.list.map((li, k) => <li key={k}>{li}</li>)}</ul>
        : "note" in b ? <p key={i} className="rounded-md bg-wash px-4 py-3 text-[14px]">{b.note}</p>
        : "links" in b ? (
          <ul key={i} className="space-y-2">
            {b.links.map((l) => (
              <li key={l.href + l.label}>
                {l.href.startsWith("/")
                  ? <Link href={l.href} className="font-semibold text-link hover:underline">{l.label}</Link>
                  : <a href={l.href} target="_blank" rel="noopener noreferrer" className="font-semibold text-link hover:underline">{l.label}</a>}
                {l.text ? <span>: {l.text}</span> : null}
              </li>
            ))}
          </ul>
        )
        : <p key={i}>{(b as { p: string }).p}</p>,
      )}
    </div>
  );
}
