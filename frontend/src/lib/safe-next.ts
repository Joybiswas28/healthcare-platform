// Where to go after logging in. Only same-site paths are allowed, so a crafted
// link like /login?next=https://evil.example can't send people elsewhere.
export function safeNext(next: string | null | undefined, fallback = "/dashboard"): string {
  if (!next || !next.startsWith("/") || next.startsWith("//") || next.startsWith("/\\")) return fallback;
  return next;
}
