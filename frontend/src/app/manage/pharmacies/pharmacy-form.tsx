"use client";

import { useState, type FormEvent } from "react";
import { LocationPicker, validCoords } from "@/components/location-picker";
import { Alert, Button, Field } from "@/components/ui";
import type { Pharmacy, PharmacyInput } from "@/lib/api/client";
import type { Coords } from "@/lib/geo";

// Registration and editing share this form. `onSave` returns an error message, or "" on success.
export function PharmacyForm({
  initial,
  submitLabel,
  onSave,
}: {
  initial?: Pharmacy;
  submitLabel: string;
  onSave: (input: PharmacyInput) => Promise<string>;
}) {
  const [coords, setCoords] = useState<Coords | null>(
    initial?.latitude != null && initial?.longitude != null ? { latitude: initial.latitude, longitude: initial.longitude } : null,
  );
  const [delivers, setDelivers] = useState(initial?.offers_delivery ?? true);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!validCoords(coords)) {
      setError("Add the shop's location: patients and riders are matched by distance.");
      return;
    }
    const f = new FormData(e.currentTarget);
    const text = (k: string) => String(f.get(k) ?? "").trim();
    const input: PharmacyInput = {
      name: text("name"),
      drug_license_no: text("drug_license_no"),
      address: text("address"),
      district: text("district"),
      area: text("area") || undefined,
      phone: text("phone"),
      latitude: coords.latitude,
      longitude: coords.longitude,
      opening_hours: text("opening_hours") || undefined,
      offers_delivery: delivers,
      delivery_radius_km: Number(f.get("delivery_radius_km") || 5),
      accepting_orders: f.get("accepting_orders") === "on",
    };
    setBusy(true);
    setError("");
    setSaved(false);
    const problem = await onSave(input);
    setBusy(false);
    if (problem) setError(problem);
    else setSaved(true);
  }

  const p = initial;
  return (
    <form onSubmit={onSubmit} className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Pharmacy name" name="name" defaultValue={p?.name} required />
        <Field label="DGDA drug licence number" name="drug_license_no" defaultValue={p?.drug_license_no} required />
        <Field label="Phone" name="phone" type="tel" defaultValue={p?.phone} required />
        <Field label="Opening hours" name="opening_hours" defaultValue={p?.opening_hours ?? ""} placeholder="e.g. 8 AM – 11 PM" />
        <Field label="Address" name="address" defaultValue={p?.address} required />
        <Field label="Area" name="area" defaultValue={p?.area ?? ""} />
        <Field label="District" name="district" defaultValue={p?.district} required />
      </div>
      <LocationPicker value={coords} onChange={setCoords} label="Shop location (stand in the shop and use your location)" />
      <div className="flex flex-wrap items-end gap-4">
        <label className="flex items-center gap-2 pb-2 text-sm text-slate-700">
          <input type="checkbox" checked={delivers} onChange={(e) => setDelivers(e.target.checked)} className="accent-teal-700" />
          We deliver
        </label>
        {delivers && (
          <div className="w-40">
            <Field
              label="Delivery radius (km)"
              name="delivery_radius_km"
              type="number"
              min={0.5}
              max={20}
              step={0.5}
              defaultValue={p?.delivery_radius_km ?? 5}
            />
          </div>
        )}
        <label className="flex items-center gap-2 pb-2 text-sm text-slate-700">
          <input type="checkbox" name="accepting_orders" defaultChecked={p?.accepting_orders ?? true} className="accent-teal-700" />
          Taking orders now
        </label>
      </div>
      {error && <Alert>{error}</Alert>}
      {saved && <Alert kind="success">Saved.</Alert>}
      <Button type="submit" disabled={busy}>
        {busy ? "Saving…" : submitLabel}
      </Button>
    </form>
  );
}
