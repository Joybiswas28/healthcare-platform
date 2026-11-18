"use client";

import { useState } from "react";
import { currentPosition, type Coords } from "@/lib/geo";
import { Alert, Button } from "./ui";

const input =
  "w-full rounded-lg border border-slate-300 bg-surface px-3 py-2 text-sm outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-600/20";

// Latitude/longitude with a "use my location" button. Controlled by the parent.
export function LocationPicker({
  value,
  onChange,
  label = "Location",
}: {
  value: Coords | null;
  onChange: (c: Coords | null) => void;
  label?: string;
}) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function locate() {
    setBusy(true);
    setError("");
    try {
      onChange(await currentPosition());
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
    setBusy(false);
  }

  function set(field: keyof Coords, raw: string) {
    const n = Number(raw);
    const next = { latitude: value?.latitude ?? NaN, longitude: value?.longitude ?? NaN, [field]: raw === "" ? NaN : n };
    onChange(Number.isFinite(next.latitude) || Number.isFinite(next.longitude) ? next : null);
  }

  return (
    <fieldset className="space-y-2">
      <legend className="mb-1 text-sm font-medium text-slate-700">{label}</legend>
      <div className="flex flex-wrap items-center gap-2">
        <Button type="button" variant="secondary" onClick={locate} disabled={busy}>
          {busy ? "Locating…" : "📍 Use my current location"}
        </Button>
        {value && Number.isFinite(value.latitude) && Number.isFinite(value.longitude) && (
          <span className="text-xs text-slate-500">
            {value.latitude}, {value.longitude}
          </span>
        )}
      </div>
      <details className="text-sm">
        <summary className="cursor-pointer text-slate-600">Enter coordinates by hand</summary>
        <div className="mt-2 grid grid-cols-2 gap-2">
          <input
            aria-label="Latitude"
            placeholder="Latitude, e.g. 23.7465"
            inputMode="decimal"
            value={value && Number.isFinite(value.latitude) ? value.latitude : ""}
            onChange={(e) => set("latitude", e.target.value)}
            className={input}
          />
          <input
            aria-label="Longitude"
            placeholder="Longitude, e.g. 90.3750"
            inputMode="decimal"
            value={value && Number.isFinite(value.longitude) ? value.longitude : ""}
            onChange={(e) => set("longitude", e.target.value)}
            className={input}
          />
        </div>
        <p className="mt-1 text-xs text-slate-500">Tip: long-press a spot in Google Maps to copy its coordinates.</p>
      </details>
      {error && <Alert>{error}</Alert>}
    </fieldset>
  );
}

export function validCoords(c: Coords | null): c is Coords {
  return !!c && Number.isFinite(c.latitude) && Number.isFinite(c.longitude);
}
