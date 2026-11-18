"use client";

import { useState, type FormEvent } from "react";
import { TextArea } from "@/components/textarea";
import { Alert, Button, Field } from "@/components/ui";
import type { Hospital, HospitalInput } from "@/lib/api/client";

function list(value: FormDataEntryValue | null): string[] {
  return String(value ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
}

function optionalNumber(value: FormDataEntryValue | null): number | undefined {
  const s = String(value ?? "").trim();
  if (!s) return undefined;
  const n = Number(s);
  return Number.isFinite(n) ? n : undefined;
}

// Registration and editing share this form. `onSave` returns an error message, or "" on success.
export function HospitalForm({
  initial,
  submitLabel,
  onSave,
}: {
  initial?: Hospital;
  submitLabel: string;
  onSave: (input: HospitalInput) => Promise<string>;
}) {
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const text = (k: string) => String(f.get(k) ?? "").trim();
    const input: HospitalInput = {
      name: text("name"),
      registration_no: text("registration_no"),
      description: text("description"),
      address: text("address"),
      district: text("district"),
      area: text("area"),
      phone: text("phone"),
      email: text("email"),
      website: text("website"),
      departments: list(f.get("departments")),
      facilities: list(f.get("facilities")),
      visiting_hours: text("visiting_hours"),
      latitude: optionalNumber(f.get("latitude")),
      longitude: optionalNumber(f.get("longitude")),
    };
    setBusy(true);
    setError("");
    setSaved(false);
    const problem = await onSave(input);
    setBusy(false);
    if (problem) setError(problem);
    else setSaved(true);
  }

  const h = initial;
  return (
    <form onSubmit={onSubmit} className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Hospital name" name="name" defaultValue={h?.name} required />
        <Field label="DGHS licence number" name="registration_no" defaultValue={h?.registration_no} required />
        <Field label="Phone" name="phone" type="tel" defaultValue={h?.phone} required />
        <Field label="Email" name="email" type="email" defaultValue={h?.email ?? ""} />
        <Field label="Address" name="address" defaultValue={h?.address} required />
        <Field label="Area" name="area" defaultValue={h?.area ?? ""} placeholder="e.g. Panthapath" />
        <Field label="District" name="district" defaultValue={h?.district} required placeholder="e.g. Dhaka" />
        <Field label="Website" name="website" defaultValue={h?.website ?? ""} placeholder="https://…" />
      </div>
      <TextArea label="About the hospital" name="description" rows={3} defaultValue={h?.description ?? ""} />
      <Field
        label="Departments"
        name="departments"
        defaultValue={(h?.departments ?? []).join(", ")}
        hint="Separate with commas, e.g. Cardiology, Medicine, Pediatrics"
      />
      <Field
        label="Facilities"
        name="facilities"
        defaultValue={(h?.facilities ?? []).join(", ")}
        hint="Separate with commas, e.g. ICU, Emergency 24/7, Pharmacy"
      />
      <Field label="Visiting hours" name="visiting_hours" defaultValue={h?.visiting_hours ?? ""} placeholder="e.g. 4 PM – 7 PM" />
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Latitude (optional)" name="latitude" inputMode="decimal" defaultValue={h?.latitude ?? ""} placeholder="23.7509" />
        <Field label="Longitude (optional)" name="longitude" inputMode="decimal" defaultValue={h?.longitude ?? ""} placeholder="90.3935" />
      </div>
      {error && <Alert>{error}</Alert>}
      {saved && <Alert kind="success">Saved.</Alert>}
      <Button type="submit" disabled={busy}>
        {busy ? "Saving…" : submitLabel}
      </Button>
    </form>
  );
}
