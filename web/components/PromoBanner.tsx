import Link from "next/link";

/** The homepage promo banner: the standard 970 × 250 billboard, centred, and
 *  shrinking in proportion on narrower screens. */
export default function PromoBanner({ image, link, alt }: { image: string; link: string; alt: string }) {
  /* eslint-disable-next-line @next/next/no-img-element */
  const img = <img src={image} alt={alt} width={970} height={250} className="block h-full w-full object-cover" />;
  const box = "mx-auto block aspect-[97/25] w-full max-w-[970px] overflow-hidden rounded-lg border border-line bg-wash";
  return (
    <section aria-label="Featured" className="mt-10">
      {!link ? (
        <div className={box}>{img}</div>
      ) : link.startsWith("/") ? (
        <Link href={link} className={box}>{img}</Link>
      ) : (
        <a href={link} className={box} target="_blank" rel="noopener">{img}</a>
      )}
    </section>
  );
}
