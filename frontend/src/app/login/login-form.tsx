"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState, type FormEvent } from "react";
import { useAuth } from "@/components/auth-provider";
import { Alert, Button, Card, Field } from "@/components/ui";
import { api, errorMessage } from "@/lib/api/client";
import { safeNext } from "@/lib/safe-next";

export function LoginForm() {
  const router = useRouter();
  const params = useSearchParams();
  const { setUser } = useAuth();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy(true);
    setError("");
    const { data, error } = await api.POST("/api/v1/auth/login", {
      body: { phone: String(form.get("phone")), password: String(form.get("password")) },
    });
    setBusy(false);
    if (!data?.user) {
      setError(errorMessage(error, "Could not log in."));
      return;
    }
    setUser(data.user);
    router.push(safeNext(params.get("next")));
  }

  return (
    <Card>
      <form onSubmit={onSubmit} className="space-y-4">
        {params.get("reset") === "1" && <Alert kind="success">Password changed. Log in with your new password.</Alert>}
        <Field label="Mobile number" name="phone" type="tel" placeholder="01XXXXXXXXX" autoComplete="tel" required />
        <Field label="Password" name="password" type="password" autoComplete="current-password" required />
        {error && <Alert>{error}</Alert>}
        <Button type="submit" disabled={busy} className="w-full">
          {busy ? "Logging in…" : "Log in"}
        </Button>
        <div className="flex justify-between text-sm">
          <Link href="/forgot-password" className="text-teal-800 hover:underline">
            Forgot password?
          </Link>
          <Link href="/register" className="text-teal-800 hover:underline">
            Create an account
          </Link>
        </div>
      </form>
    </Card>
  );
}
