"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";
import { useAuth } from "@/components/auth-provider";
import { Alert, Button, Card, Field } from "@/components/ui";
import { useT } from "@/components/use-lang";
import { api, errorMessage, type User } from "@/lib/api/client";
import { dateTime, ROLE_LABELS } from "@/lib/format";
import { T } from "@/components/t";

export function AccountSettings({ user }: { user: User }) {
  return (
    <div className="space-y-6">
      <ProfileCard user={user} />
      <PasswordCard />
      <Card>
        <h2 className="font-semibold text-slate-900"><T>Account</T></h2>
        <dl className="mt-3 grid grid-cols-[8rem_1fr] gap-y-2 text-sm">
          <dt className="text-slate-500"><T>Mobile number</T></dt>
          <dd className="text-slate-900">{user.phone}</dd>
          <dt className="text-slate-500"><T>Account type</T></dt>
          <dd className="text-slate-900">
            <T>{ROLE_LABELS[user.role ?? ""] ?? user.role ?? ""}</T>
          </dd>
          <dt className="text-slate-500"><T>Member since</T></dt>
          <dd className="text-slate-900">{dateTime(user.created_at)}</dd>
        </dl>
        <p className="mt-3 text-xs text-slate-500">
          <T>{"Your mobile number is how you log in, so it can't be changed here. Contact support if you need a new one."}</T>
        </p>
      </Card>
    </div>
  );
}

function ProfileCard({ user }: { user: User }) {
  const { setUser } = useAuth();
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    setBusy(true);
    setError("");
    setSaved(false);
    const { data, error } = await api.PUT("/api/v1/me", {
      body: { name: String(f.get("name")).trim(), email: String(f.get("email")).trim() },
    });
    setBusy(false);
    if (!data) {
      setError(errorMessage(error));
      return;
    }
    setUser(data); // the header shows the new name straight away
    setSaved(true);
  }

  return (
    <Card>
      <h2 className="mb-4 font-semibold text-slate-900"><T>Profile</T></h2>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Full name" name="name" defaultValue={user.name} minLength={2} maxLength={100} required autoComplete="name" />
        <Field
          label="Email (optional)"
          name="email"
          type="email"
          defaultValue={user.email ?? ""}
          autoComplete="email"
          hint="Leave empty to remove it."
        />
        {error && <Alert>{error}</Alert>}
        {saved && <Alert kind="success">Saved.</Alert>}
        <Button type="submit" disabled={busy}>
          {busy ? "Saving…" : "Save profile"}
        </Button>
      </form>
    </Card>
  );
}

function PasswordCard() {
  const t = useT();
  const { setUser } = useAuth();
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const f = new FormData(form);
    const next = String(f.get("new_password"));
    if (next !== String(f.get("confirm"))) {
      setError(t("The two new passwords don't match."));
      return;
    }
    setBusy(true);
    setError("");
    setDone(false);
    const { data, error, response } = await api.POST("/api/v1/me/password", {
      body: { current_password: String(f.get("current_password")), new_password: next },
    });
    setBusy(false);
    if (!data) {
      const wait = response.headers.get("retry-after");
      setError(response.status === 429 && wait ? t("Too many tries. Wait {wait} seconds and try again.", { wait }) : errorMessage(error));
      return;
    }
    // The proxy stored the fresh login in the cookie; this device stays signed in.
    if (data.user) setUser(data.user);
    form.reset();
    setDone(true);
  }

  return (
    <Card>
      <h2 className="font-semibold text-slate-900"><T>Change password</T></h2>
      <p className="mb-4 text-sm text-slate-600"><T>You&apos;ll stay logged in here; every other device is logged out.</T></p>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Current password" name="current_password" type="password" autoComplete="current-password" required />
        <Field
          label="New password"
          name="new_password"
          type="password"
          autoComplete="new-password"
          minLength={8}
          maxLength={72}
          required
          hint="At least 8 characters."
        />
        <Field label="Confirm new password" name="confirm" type="password" autoComplete="new-password" required />
        {error && <Alert>{error}</Alert>}
        {done && <Alert kind="success">Password changed. Other devices have been logged out.</Alert>}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Button type="submit" disabled={busy}>
            {busy ? "Changing…" : "Change password"}
          </Button>
          <Link href="/forgot-password" className="text-sm text-teal-800 hover:underline">
            <T>Forgot your current password?</T>
          </Link>
        </div>
      </form>
    </Card>
  );
}
