import type { Metadata } from "next";
import { LandingPage, landingMetadata } from "@/components/Landing";
import type { SP } from "@/components/SearchView";

export const dynamic = "force-dynamic";

type Props = { searchParams: Promise<SP> };

export async function generateMetadata({ searchParams }: Props): Promise<Metadata> {
  return landingMetadata("tcg", undefined, await searchParams);
}

export default async function Page({ searchParams }: Props) {
  return <LandingPage kind="tcg" sp={await searchParams} />;
}
