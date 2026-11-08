// Forwards /api/* from the browser to the Go API, so the browser talks to one
// origin and no CORS setup is needed. API_ORIGIN is read at runtime, like the
// server-side calls in src/lib/server-api.ts. Bodies are streamed, not
// buffered; the API enforces its own size limits.
import { apiOrigin } from "@/lib/server-api";

const forwardedRequestHeaders = ["accept", "content-type"];
const forwardedResponseHeaders = [
  "content-type",
  "content-disposition",
  "x-content-type-options",
  "content-security-policy",
  "cache-control",
];

async function proxy(req: Request, { params }: { params: Promise<{ path: string[] }> }): Promise<Response> {
  const { path } = await params;
  const target = `${apiOrigin}/api/${path.map(encodeURIComponent).join("/")}${new URL(req.url).search}`;

  const headers = new Headers();
  for (const name of forwardedRequestHeaders) {
    const value = req.headers.get(name);
    if (value) headers.set(name, value);
  }

  let res: Response;
  try {
    res = await fetch(target, {
      method: req.method,
      headers,
      body: req.method === "GET" || req.method === "HEAD" ? undefined : req.body,
      cache: "no-store",
      redirect: "manual",
      // Required by Node to stream a request body.
      duplex: "half",
    } as RequestInit);
  } catch {
    return Response.json(
      { error: { code: "api_unreachable", message: "The ReadVault API is not reachable." } },
      { status: 502 },
    );
  }

  const out = new Headers();
  for (const name of forwardedResponseHeaders) {
    const value = res.headers.get(name);
    if (value) out.set(name, value);
  }
  return new Response(res.body, { status: res.status, headers: out });
}

export const dynamic = "force-dynamic";
export { proxy as GET, proxy as POST, proxy as PATCH };
