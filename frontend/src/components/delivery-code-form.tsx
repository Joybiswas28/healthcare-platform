"use client";

import { useState, type FormEvent } from "react";
import { Alert, Button } from "./ui";
import { T } from "@/components/t";

// The 6-digit code the patient reads out at handover. Five wrong codes lock the order.
export function DeliveryCodeForm({
  label,
  onSubmit,
}: {
  label: string;
  onSubmit: (code: string) => Promise<string>;
}) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const code = String(new FormData(e.currentTarget).get("code")).trim();
    setBusy(true);
    setError("");
    const problem = await onSubmit(code);
    setBusy(false);
    if (problem) setError(problem);
  }

  return (
    <form onSubmit={submit} className="space-y-2">
      <label className="block text-sm">
        <span className="mb-1 block font-medium text-slate-700"><T>Patient&apos;s 6-digit delivery code</T></span>
        <input
          name="code"
          inputMode="numeric"
          pattern="[0-9]{6}"
          maxLength={6}
          required
          autoComplete="off"
          className="w-40 rounded-lg border border-slate-300 px-3 py-2 font-mono text-lg tracking-[0.3em] outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-600/20"
        />
      </label>
      <p className="text-xs text-slate-500"><T>Ask the patient for it after handing over the medicines and collecting the cash.</T></p>
      {error && <Alert>{error}</Alert>}
      <Button type="submit" disabled={busy}>
        {busy ? "Checking…" : label}
      </Button>
    </form>
  );
}
