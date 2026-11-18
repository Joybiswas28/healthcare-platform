import "server-only";

// The backend's access token lives only in this httpOnly cookie. Browser code
// never sees it: every API call goes through the /api/v1 proxy route, which
// adds it as a Bearer header on the way to the Go backend.
export const SESSION_COOKIE = "hp_session";

export const API_URL = (process.env.API_URL ?? "http://127.0.0.1:8080").replace(/\/+$/, "");

// Cookies are SameSite=Lax, which already stops most cross-site form posts.
// Requiring a matching Origin on every state-changing call closes the rest.
export function isSameOrigin(req: Request): boolean {
  const origin = req.headers.get("origin");
  if (!origin) return false;
  const host = req.headers.get("x-forwarded-host") ?? req.headers.get("host");
  try {
    return new URL(origin).host === host;
  } catch {
    return false;
  }
}

export function sessionCookieOptions(expiresAt?: string) {
  const expires = expiresAt ? new Date(expiresAt) : undefined;
  return {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax" as const,
    path: "/",
    ...(expires && !Number.isNaN(expires.getTime()) ? { expires } : {}),
  };
}
