"use client";

import { createContext, useCallback, useContext, useEffect, useState } from "react";
import type { ReactNode } from "react";
import { api, type User } from "@/lib/api/client";

type AuthState = {
  user: User | null;
  // True until the first /me call answers, so pages don't flash "logged out".
  loading: boolean;
  refresh: () => Promise<User | null>;
  setUser: (user: User | null) => void;
  logout: () => Promise<void>;
};

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    const { data } = await api.GET("/api/v1/me");
    const next = data ?? null;
    setUser(next);
    setLoading(false);
    return next;
  }, []);

  useEffect(() => {
    let cancelled = false;
    api.GET("/api/v1/me").then(({ data }) => {
      if (cancelled) return;
      setUser(data ?? null);
      setLoading(false);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const logout = useCallback(async () => {
    await fetch("/api/session/logout", { method: "POST" });
    setUser(null);
  }, []);

  return (
    <AuthContext value={{ user, loading, refresh, setUser, logout }}>{children}</AuthContext>
  );
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside <AuthProvider>");
  return ctx;
}
