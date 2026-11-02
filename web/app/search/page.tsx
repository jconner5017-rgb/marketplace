import type { Metadata } from "next";
import { SearchView, searchMetadata, type SP } from "@/components/SearchView";

export const dynamic = "force-dynamic";

export async function generateMetadata({ searchParams }: { searchParams: Promise<SP> }): Promise<Metadata> {
  return searchMetadata(await searchParams);
}

export default async function SearchPage({ searchParams }: { searchParams: Promise<SP> }) {
  return <SearchView raw={await searchParams} />;
}
