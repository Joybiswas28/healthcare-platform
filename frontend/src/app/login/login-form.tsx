"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState, type FormEvent } from "react";
import { useAuth } from "@/components/auth-provider";
import { T } from "@/components/t";
import { Alert, Button, Card, Field } from "@/components/ui";
import { api, errorMessage } from "@/lib/api/client";
import { DEMO_ACCOUNTS, SHOW_DEMO_ACCOUNTS } from "@/lib/demo-accounts";
import { safeNext } from "@/lib/safe-next";

export function LoginForm() {
  const router = useRouter();
  const params = useSearchParams();
  const { setUser } = useAuth();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState<string | null>(null);

  async function login(phone: string, password: string, who: string) {
    setBusy(who);
    setError("");
    const { data, error } = await api.POST("/api/v1/auth/login", { body: { phone, password } });
    setBusy(null);
    if (!data?.user) {
      setError(errorMessage(error, "Could not log in."));
      return;
    }
    setUser(data.user);
    router.push(safeNext(params.get("next")));
  }

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    login(String(form.get("phone")), String(form.get("password")), "form");
  }

  return (
    <div className="space-y-4">
      <Card>
        <form onSubmit={onSubmit} className="space-y-4">
          {params.get("reset") === "1" && <Alert kind="success">Password changed. Log in with your new password.</Alert>}
          <Field label="Mobile number" name="phone" type="tel" placeholder="01XXXXXXXXX" autoComplete="tel" required />
          <Field label="Password" name="password" type="password" autoComplete="current-password" required />
          {error && <Alert>{error}</Alert>}
          <Button type="submit" disabled={busy !== null} className="w-full">
            {busy === "form" ? "Logging in…" : "Log in"}
          </Button>
          <div className="flex justify-between text-sm">
            <Link href="/forgot-password" className="text-teal-800 hover:underline">
              <T>Forgot password?</T>
            </Link>
            <Link href="/register" className="text-teal-800 hover:underline">
              <T>Create an account</T>
            </Link>
          </div>
        </form>
      </Card>

      {SHOW_DEMO_ACCOUNTS && (
        <Card className="border-dashed">
          <h2 className="font-semibold text-slate-900">
            <T>Demo accounts</T>
          </h2>
          <p className="mb-3 text-xs text-slate-500">
            <T>Test data on this computer only; this box is hidden on a real deployment. Click to log in as:</T>
          </p>
          <ul className="divide-y divide-slate-100">
            {DEMO_ACCOUNTS.map((a) => (
              <li key={a.phone} className="flex items-center justify-between gap-3 py-2 text-sm">
                <span>
                  <span className="font-medium text-slate-900">
                    <T>{a.role}</T>
                  </span>
                  <span className="text-slate-600"> · {a.name}</span>
                  <span className="block font-mono text-xs text-slate-500">
                    {a.phone} / {a.password}
                  </span>
                </span>
                <Button
                  type="button"
                  variant="secondary"
                  disabled={busy !== null}
                  onClick={() => login(a.phone, a.password, a.phone)}
                >
                  {busy === a.phone ? "…" : "Log in"}
                </Button>
              </li>
            ))}
          </ul>
        </Card>
      )}
    </div>
  );
}
