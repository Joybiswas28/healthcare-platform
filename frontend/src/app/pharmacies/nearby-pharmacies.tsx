"use client";

import Link from "next/link";
import { useState } from "react";
import { LocationPicker, validCoords } from "@/components/location-picker";
import { Alert, Badge, Button, ButtonLink, Card, Select } from "@/components/ui";
import { api, errorMessage, type Pharmacy } from "@/lib/api/client";
import { km, type Coords } from "@/lib/geo";

export function NearbyPharmacies() {
  const [coords, setCoords] = useState<Coords | null>(null);
  const [radius, setRadius] = useState(5);
  const [items, setItems] = useState<Pharmacy[] | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function search() {
    if (!validCoords(coords)) {
      setError("Share or enter your location first.");
      return;
    }
    setBusy(true);
    setError("");
    const { data, error } = await api.GET("/api/v1/pharmacies", {
      params: { query: { latitude: coords.latitude, longitude: coords.longitude, radius_km: radius, limit: 50 } },
    });
    setBusy(false);
    if (data) setItems(data.pharmacies ?? []);
    else setError(errorMessage(error));
  }

  return (
    <div className="space-y-6">
      <Card className="space-y-4">
        <LocationPicker value={coords} onChange={setCoords} label="Your location" />
        <div className="flex flex-wrap items-end gap-3">
          <div className="w-40">
            <Select label="Within" value={radius} onChange={(e) => setRadius(Number(e.target.value))}>
              {[2, 5, 10, 20].map((r) => (
                <option key={r} value={r}>
                  {r} km
                </option>
              ))}
            </Select>
          </div>
          <Button onClick={search} disabled={busy}>
            {busy ? "Searching…" : "Find pharmacies"}
          </Button>
          <ButtonLink href="/medicines/new" variant="secondary">
            Order medicine
          </ButtonLink>
        </div>
      </Card>

      {error && <Alert>{error}</Alert>}
      {items !== null && items.length === 0 && (
        <p className="text-sm text-slate-600">No verified pharmacies within {radius} km. Try a wider radius.</p>
      )}
      <ul className="space-y-3">
        {(items ?? []).map((p) => (
          <li key={p.id}>
            <Link
              href={`/pharmacies/${p.id}`}
              className="block rounded-xl border border-slate-200 bg-white p-4 shadow-sm transition hover:border-teal-600"
            >
              <PharmacySummary pharmacy={p} />
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}

export function Stars({ rating, count }: { rating?: number | null; count?: number }) {
  if (!count || rating == null) return <span className="text-xs text-slate-500">No reviews yet</span>;
  return (
    <span className="text-sm text-slate-700">
      <span className="text-amber-500" aria-hidden>
        ★
      </span>{" "}
      {rating.toFixed(1)} <span className="text-xs text-slate-500">({count})</span>
    </span>
  );
}

export function PharmacySummary({ pharmacy: p }: { pharmacy: Pharmacy }) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div>
        <p className="font-semibold text-slate-900">{p.name}</p>
        <p className="text-sm text-slate-600">{[p.address, p.area, p.district].filter(Boolean).join(", ")}</p>
        <div className="mt-1 flex flex-wrap items-center gap-2">
          <Stars rating={p.rating} count={p.review_count} />
          {p.offers_delivery ? <Badge tone="teal">Delivers within {p.delivery_radius_km} km</Badge> : <Badge>Pickup only</Badge>}
          {p.accepting_orders === false && <Badge tone="amber">Not taking orders now</Badge>}
        </div>
      </div>
      {p.distance_km != null && <span className="text-sm font-medium text-slate-700">{km(p.distance_km)}</span>}
    </div>
  );
}
