import type { Metadata } from "next";
import { LandingPage, landingMetadata } from "@/components/Landing";
import type { SP } from "@/components/SearchView";

export const dynamic = "force-dynamic";

type Props = { params: Promise<{ slug: string }>; searchParams: Promise<SP> };

export async function generateMetadata({ params, searchParams }: Props): Promise<Metadata> {
  return landingMetadata("sports", (await params).slug, await searchParams);
}

export default async function Page({ params, searchParams }: Props) {
  return <LandingPage kind="sports" slug={(await params).slug} sp={await searchParams} />;
}
