import { NextRequest, NextResponse } from "next/server";
import { SESSION_COOKIE, isSameOrigin } from "@/lib/session";

// Logging out only needs the cookie gone: the backend keeps no server-side
// session, and a password reset already revokes every issued token.
export async function POST(req: NextRequest) {
  if (!isSameOrigin(req)) {
    return NextResponse.json({ error: "cross-site request blocked" }, { status: 403 });
  }
  const res = new NextResponse(null, { status: 204 });
  res.cookies.delete(SESSION_COOKIE);
  return res;
}
