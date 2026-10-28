import "server-only";
import { redirect } from "next/navigation";
import { getMe } from "./api";
import type { User } from "./types";

/** Returns the signed-in user or sends the visitor to sign in. */
export async function requireUser(next: string): Promise<User> {
  const me = await getMe();
  if (!me) redirect(`/signin?next=${encodeURIComponent(next)}`);
  return me;
}
