"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { T } from "@/components/t";
import { useT } from "@/components/use-lang";
import { Alert, Button, Card, Field } from "@/components/ui";
import { api, errorMessage } from "@/lib/api/client";

export function ResetForm() {
  const router = useRouter();
  const t = useT();
  const [phone, setPhone] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function onPhone(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const value = String(new FormData(e.currentTarget).get("phone")).trim();
    setBusy(true);
    setError("");
    const { error, response } = await api.POST("/api/v1/auth/otp", {
      body: { phone: value, purpose: "reset_password" },
    });
    setBusy(false);
    if (error) {
      const wait = response.headers.get("retry-after");
      setError(response.status === 429 && wait ? t("Please wait {wait} seconds and try again.", { wait }) : errorMessage(error));
      return;
    }
    setPhone(value);
  }

  async function onReset(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!phone) return;
    const form = new FormData(e.currentTarget);
    const password = String(form.get("password"));
    if (password !== String(form.get("confirm"))) {
      setError(t("The two passwords don't match."));
      return;
    }
    setBusy(true);
    setError("");
    const { error } = await api.POST("/api/v1/auth/password/reset", {
      body: { phone, otp_code: String(form.get("code")).trim(), new_password: password },
    });
    setBusy(false);
    if (error) {
      setError(errorMessage(error));
      return;
    }
    router.push("/login?reset=1");
  }

  if (phone) {
    return (
      <Card>
        <form onSubmit={onReset} className="space-y-4">
          {/* The backend answers the same way for unknown numbers, so this stays vague on purpose. */}
          <Alert kind="info"><T vars={{ phone }}>{"If {phone} has an account, a 6-digit code is on its way."}</T></Alert>
          <Field
            label="Verification code"
            name="code"
            inputMode="numeric"
            autoComplete="one-time-code"
            pattern="[0-9]{6}"
            maxLength={6}
            required
            autoFocus
          />
          <Field label="New password" name="password" type="password" autoComplete="new-password" minLength={8} maxLength={72} required />
          <Field label="Confirm new password" name="confirm" type="password" autoComplete="new-password" required />
          {error && <Alert>{error}</Alert>}
          <Button type="submit" disabled={busy} className="w-full">
            {busy ? "Saving…" : "Set new password"}
          </Button>
          <p className="text-xs text-slate-500"><T>You will be logged out on every other device.</T></p>
        </form>
      </Card>
    );
  }

  return (
    <Card>
      <form onSubmit={onPhone} className="space-y-4">
        <Field label="Mobile number" name="phone" type="tel" placeholder="01XXXXXXXXX" autoComplete="tel" required />
        {error && <Alert>{error}</Alert>}
        <Button type="submit" disabled={busy} className="w-full">
          {busy ? "Sending…" : "Send code"}
        </Button>
      </form>
    </Card>
  );
}
