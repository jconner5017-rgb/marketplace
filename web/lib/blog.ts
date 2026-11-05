// Blog posts come from the API (written in the back office). Same block
// format as the help center; {fee.*} placeholders are filled from the live
// fee schedule when shown.
import type { Block } from "./help";

export type Post = {
  id: number;
  slug: string;
  title: string;
  /** Meta description for search results (about 155 characters). */
  description: string;
  /** The search phrase the post is written for. */
  keyword: string;
  intro: string;
  body: Block[];
  status: "draft" | "published";
  published_at: string | null;
  updated_at: string;
  minutes: number;
};
