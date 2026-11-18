// Links typed in by users (a hospital's website, a map link) are shown only if
// they are plain web addresses, never javascript: or data: URLs.
export function safeExternalUrl(raw: string | null | undefined): string | null {
  if (!raw) return null;
  const withScheme = /^[a-z][a-z0-9+.-]*:/i.test(raw) ? raw : `https://${raw}`;
  try {
    const u = new URL(withScheme);
    return u.protocol === "https:" || u.protocol === "http:" ? u.toString() : null;
  } catch {
    return null;
  }
}

// Where to go after logging in. Only same-site paths are allowed, so a crafted
// link like /login?next=https://evil.example can't send people elsewhere.
export function safeNext(next: string | null | undefined, fallback = "/dashboard"): string {
  if (!next || !next.startsWith("/") || next.startsWith("//") || next.startsWith("/\\")) return fallback;
  return next;
}
