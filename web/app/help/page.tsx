import Link from "next/link";
import { ARTICLES, SECTIONS } from "@/lib/help";
import { fillFees, getFees } from "@/lib/catalog";

export const metadata = { title: "Help center" };
export const dynamic = "force-dynamic";

export default async function HelpCenter() {
  const fees = await getFees();
  return (
    <div className="pt-8">
      <h1 className="text-[28px] font-bold">Help center</h1>
      <p className="mt-1 text-muted">Answers for buyers and sellers.</p>
      <div className="mt-8 grid gap-x-10 gap-y-10 md:grid-cols-2">
        {SECTIONS.map((sec) => (
          <section key={sec}>
            <h2 className="border-b border-line pb-2 text-[18px] font-bold">{sec}</h2>
            <ul className="mt-3 space-y-4">
              {ARTICLES.filter((a) => a.section === sec).map((a) => (
                <li key={a.slug}>
                  <Link href={`/help/${a.slug}`} className="text-[15px] font-semibold text-link hover:underline">{a.title}</Link>
                  <p className="text-[14px] text-muted">{fillFees(a.summary, fees)}</p>
                </li>
              ))}
            </ul>
          </section>
        ))}
      </div>
    </div>
  );
}
