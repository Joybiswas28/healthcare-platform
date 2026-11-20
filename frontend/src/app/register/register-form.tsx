"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { useAuth } from "@/components/auth-provider";
import { T } from "@/components/t";
import { useT } from "@/components/use-lang";
import { Alert, Button, Card, Field, Select } from "@/components/ui";
import { api, errorMessage } from "@/lib/api/client";

type Role = "patient" | "doctor" | "hospital_admin" | "pharmacy" | "rider";

type Details = { name: string; phone: string; password: string; role: Role };

// Two steps: the details first (which sends an SMS code), then the code.
export function RegisterForm() {
  const router = useRouter();
  const { setUser } = useAuth();
  const t = useT();
  const [details, setDetails] = useState<Details | null>(null);
  const [error, setError] = useState("");
  const [info, setInfo] = useState("");
  const [busy, setBusy] = useState(false);

  async function sendCode(phone: string) {
    const { error, response } = await api.POST("/api/v1/auth/otp", { body: { phone, purpose: "register" } });
    if (response.status === 429) {
      const wait = response.headers.get("retry-after");
      return wait ? t("Please wait {wait} seconds before asking for another code.", { wait }) : errorMessage(error);
    }
    return error ? errorMessage(error) : "";
  }

  async function onDetails(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const next: Details = {
      name: String(form.get("name")).trim(),
      phone: String(form.get("phone")).trim(),
      password: String(form.get("password")),
      role: String(form.get("role")) as Role,
    };
    if (next.password !== String(form.get("confirm"))) {
      setError(t("The two passwords don't match."));
      return;
    }
    setBusy(true);
    setError("");
    const problem = await sendCode(next.phone);
    setBusy(false);
    if (problem) {
      setError(problem);
      return;
    }
    setDetails(next);
    setInfo(t("We sent a 6-digit code to {phone}.", { phone: next.phone }));
  }

  async function onCode(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!details) return;
    const code = String(new FormData(e.currentTarget).get("code")).trim();
    setBusy(true);
    setError("");
    const { data, error } = await api.POST("/api/v1/auth/register", { body: { ...details, otp_code: code } });
    setBusy(false);
    if (!data?.user) {
      setError(errorMessage(error, t("Could not create the account.")));
      return;
    }
    setUser(data.user);
    router.push("/dashboard");
  }

  async function resend() {
    if (!details) return;
    setBusy(true);
    setError("");
    const problem = await sendCode(details.phone);
    setBusy(false);
    if (problem) setError(problem);
    else setInfo(t("We sent a new code to {phone}.", { phone: details.phone }));
  }

  if (details) {
    return (
      <Card>
        <form onSubmit={onCode} className="space-y-4">
          {info && <Alert kind="info">{info}</Alert>}
          <Field
            label="Verification code"
            name="code"
            inputMode="numeric"
            autoComplete="one-time-code"
            pattern="[0-9]{6}"
            maxLength={6}
            required
            autoFocus
            hint="The code expires in 5 minutes."
          />
          {error && <Alert>{error}</Alert>}
          <Button type="submit" disabled={busy} className="w-full">
            {busy ? "Creating account…" : "Verify and create account"}
          </Button>
          <div className="flex justify-between text-sm">
            <button type="button" onClick={() => setDetails(null)} className="text-slate-600 hover:underline">
              ← Change details
            </button>
            <button type="button" onClick={resend} disabled={busy} className="text-teal-800 hover:underline">
              <T>Send the code again</T>
            </button>
          </div>
        </form>
      </Card>
    );
  }

  return (
    <Card>
      <form onSubmit={onDetails} className="space-y-4">
        <Field label="Full name" name="name" autoComplete="name" minLength={2} required />
        <Field
          label="Mobile number"
          name="phone"
          type="tel"
          placeholder="01XXXXXXXXX"
          autoComplete="tel"
          required
          hint="We'll send a verification code to this number."
        />
        <Select label="I am a" name="role" defaultValue="patient">
          <option value="patient">{t("Patient")}</option>
          <option value="doctor">{t("Doctor")}</option>
          <option value="hospital_admin">{t("Hospital administrator")}</option>
          <option value="pharmacy">{t("Pharmacy owner")}</option>
          <option value="rider">{t("Delivery rider")}</option>
        </Select>
        <Field
          label="Password"
          name="password"
          type="password"
          autoComplete="new-password"
          minLength={8}
          maxLength={72}
          required
          hint="At least 8 characters."
        />
        <Field label="Confirm password" name="confirm" type="password" autoComplete="new-password" required />
        {error && <Alert>{error}</Alert>}
        <Button type="submit" disabled={busy} className="w-full">
          {busy ? "Sending code…" : "Continue"}
        </Button>
        <p className="text-center text-sm text-slate-600">
          Already have an account?{" "}
          <Link href="/login" className="text-teal-800 hover:underline">
            <T>Log in</T>
          </Link>
        </p>
      </form>
    </Card>
  );
}
