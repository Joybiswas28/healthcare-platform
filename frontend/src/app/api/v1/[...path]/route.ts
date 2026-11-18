import { NextRequest, NextResponse } from "next/server";
import { API_URL, SESSION_COOKIE, isSameOrigin, sessionCookieOptions } from "@/lib/session";

// Backend-for-frontend proxy: /api/v1/* here is forwarded to the same path on the
// Go API. Paths match api/openapi.yaml exactly, so the generated client works
// against this route unchanged.

// Responses that hand out a token. The proxy keeps the token in the cookie and
// removes it from the JSON the browser receives.
const TOKEN_ROUTES = new Set(["auth/login", "auth/register"]);

// Request headers worth passing on. Everything else (cookies, the browser's own
// Authorization header, hop-by-hop headers) is dropped.
const FORWARD_REQUEST = ["accept", "content-type", "accept-language", "if-none-match", "last-event-id"];
// content-security-policy and x-content-type-options matter for shared files:
// the backend serves them sandboxed so an uploaded HTML/SVG can't run script.
const FORWARD_RESPONSE = [
  "content-type",
  "content-length",
  "content-disposition",
  "content-security-policy",
  "x-content-type-options",
  "retry-after",
  "cache-control",
  "etag",
  "x-accel-buffering", // event streams: tell any proxy in front not to buffer
];

const SAFE_METHODS = new Set(["GET", "HEAD", "OPTIONS"]);

async function handle(req: NextRequest, ctx: RouteContext<"/api/v1/[...path]">) {
  const { path } = await ctx.params;
  const subpath = path.map(encodeURIComponent).join("/");

  if (!SAFE_METHODS.has(req.method) && !isSameOrigin(req)) {
    return NextResponse.json({ error: "cross-site request blocked" }, { status: 403 });
  }

  const headers = new Headers();
  for (const name of FORWARD_REQUEST) {
    const value = req.headers.get(name);
    if (value) headers.set(name, value);
  }
  const token = req.cookies.get(SESSION_COOKIE)?.value;
  if (token) headers.set("authorization", `Bearer ${token}`);

  let upstream: Response;
  try {
    upstream = await fetch(`${API_URL}/api/v1/${subpath}${req.nextUrl.search}`, {
      method: req.method,
      headers,
      body: SAFE_METHODS.has(req.method) ? undefined : await req.arrayBuffer(),
      redirect: "manual",
      cache: "no-store",
      // When the browser goes away, stop the upstream request too (matters for event streams).
      signal: req.signal,
    });
  } catch {
    return NextResponse.json({ error: "the server is unreachable, try again shortly" }, { status: 502 });
  }

  const outHeaders = new Headers();
  for (const name of FORWARD_RESPONSE) {
    const value = upstream.headers.get(name);
    if (value) outHeaders.set(name, value);
  }

  if (TOKEN_ROUTES.has(subpath) && req.method === "POST" && upstream.ok) {
    const data = (await upstream.json()) as { access_token?: string; expires_at?: string };
    const { access_token, ...rest } = data;
    const res = NextResponse.json(rest, { status: upstream.status });
    if (access_token) {
      res.cookies.set(SESSION_COOKIE, access_token, sessionCookieOptions(data.expires_at));
    }
    return res;
  }

  const res = new NextResponse(upstream.body, { status: upstream.status, headers: outHeaders });
  // An expired or revoked token: drop it so the UI shows the logged-out state.
  if (upstream.status === 401 && token) {
    res.cookies.delete(SESSION_COOKIE);
  }
  return res;
}

export const GET = handle;
export const HEAD = handle;
export const POST = handle;
export const PUT = handle;
export const PATCH = handle;
export const DELETE = handle;
