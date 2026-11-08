// Server-side calls to the Go API. Pages render with fresh data on every
// request, so nothing is cached here.
import { readJSON } from "./api";

/** Where the Go API listens, read at runtime. */
export const apiOrigin = process.env.API_ORIGIN ?? "http://127.0.0.1:8080";

export async function apiGet<T>(path: string): Promise<T> {
  const res = await fetch(`${apiOrigin}${path}`, { cache: "no-store", headers: { Accept: "application/json" } });
  return readJSON<T>(res);
}
