"use client";

import { usePathname, useRouter } from "next/navigation";
import { useEffect, type ReactNode } from "react";
import type { User } from "@/lib/api/client";
import { useAuth } from "./auth-provider";
import { Spinner } from "./ui";

// Sends logged-out visitors to /login and back here afterwards. This only
// shapes the UI: the backend checks the token on every request regardless.
export function RequireAuth({ children }: { children: (user: User) => ReactNode }) {
  const { user, loading } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    if (!loading && !user) router.replace(`/login?next=${encodeURIComponent(pathname)}`);
  }, [loading, user, router, pathname]);

  if (!user) return <Spinner />;
  return <>{children(user)}</>;
}
