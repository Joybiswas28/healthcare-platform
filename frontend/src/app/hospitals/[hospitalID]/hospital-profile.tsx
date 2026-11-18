"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { ChamberCard } from "@/components/chamber-card";
import { Alert, Badge, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type PublicHospital } from "@/lib/api/client";
import { safeExternalUrl } from "@/lib/safe-next";
import { HospitalLogo } from "../hospital-results";

export function HospitalProfile() {
  const { hospitalID } = useParams<{ hospitalID: string }>();
  const [h, setH] = useState<PublicHospital | null>(null);
  const [error, setError] = useState("");
  const [department, setDepartment] = useState("");

  useEffect(() => {
    api.GET("/api/v1/hospitals/{hospitalID}", { params: { path: { hospitalID } } }).then(({ data, error, response }) => {
      if (data) setH(data);
      else setError(response.status === 404 ? "This hospital could not be found." : errorMessage(error));
    });
  }, [hospitalID]);

  if (error) return <Alert>{error}</Alert>;
  if (!h) return <Spinner />;

  const doctors = (h.doctors ?? []).filter((d) => !department || d.department === department);
  const departments = Array.from(new Set((h.doctors ?? []).map((d) => d.department).filter(Boolean))) as string[];
  const photos = h.photos ?? [];
  const website = safeExternalUrl(h.website);
  const mapLink =
    safeExternalUrl(h.map_url) ??
    (h.latitude != null && h.longitude != null
      ? `https://www.google.com/maps/search/?api=1&query=${h.latitude},${h.longitude}`
      : null);

  return (
    <div className="space-y-6">
      <Link href="/hospitals" className="text-sm text-teal-800 hover:underline">
        ← All hospitals
      </Link>

      <Card>
        <div className="flex flex-wrap items-start gap-4">
          <HospitalLogo hospital={h} size="h-20 w-20" />
          <div className="min-w-0 flex-1">
            <h1 className="text-2xl font-semibold text-slate-900">{h.name}</h1>
            <p className="text-slate-600">{[h.address, h.area, h.district].filter(Boolean).join(", ")}</p>
            <p className="mt-1 text-sm text-slate-600">
              {h.phone}
              {h.email ? ` · ${h.email}` : ""}
              {website && (
                <>
                  {" · "}
                  <a href={website} target="_blank" rel="noopener noreferrer nofollow" className="text-teal-800 hover:underline">
                    Website
                  </a>
                </>
              )}
              {mapLink && (
                <>
                  {" · "}
                  <a href={mapLink} target="_blank" rel="noopener noreferrer nofollow" className="text-teal-800 hover:underline">
                    Map
                  </a>
                </>
              )}
            </p>
            {h.visiting_hours && <p className="mt-1 text-sm text-slate-600">Visiting hours: {h.visiting_hours}</p>}
          </div>
        </div>
        {h.description && <p className="mt-4 whitespace-pre-line text-sm text-slate-700">{h.description}</p>}
        {(h.facilities ?? []).length > 0 && (
          <div className="mt-4 flex flex-wrap gap-2">
            {h.facilities!.map((f) => (
              <Badge key={f}>{f}</Badge>
            ))}
          </div>
        )}
      </Card>

      {photos.length > 0 && (
        <div className="flex gap-3 overflow-x-auto pb-2">
          {photos.map((p) => (
            // eslint-disable-next-line @next/next/no-img-element
            <img key={p.id} src={p.url} alt={`${h.name} photo`} className="h-40 shrink-0 rounded-lg border border-slate-200 object-cover" />
          ))}
        </div>
      )}

      <section className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-lg font-semibold text-slate-900">Doctors and sessions</h2>
          {departments.length > 1 && (
            <select
              value={department}
              onChange={(e) => setDepartment(e.target.value)}
              aria-label="Department"
              className="rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm"
            >
              <option value="">All departments</option>
              {departments.map((d) => (
                <option key={d}>{d}</option>
              ))}
            </select>
          )}
        </div>
        {doctors.length === 0 && <p className="text-sm text-slate-600">No doctors listed yet.</p>}
        {doctors.map((d) => (
          <div key={d.doctor_id} className="space-y-2">
            <div>
              <Link href={`/doctors/${d.doctor_id}`} className="font-semibold text-slate-900 hover:underline">
                {d.name}
              </Link>
              <p className="text-sm text-slate-600">
                {[d.designation, d.department, d.specialization, d.degrees].filter(Boolean).join(" · ")}
              </p>
            </div>
            {(d.chambers ?? [])
              .filter((c) => c.booking_enabled !== false)
              .map((c) => (
                <ChamberCard key={c.id} chamber={{ ...c, address: c.address ?? h.address }} title={`Chamber at ${h.name}`} />
              ))}
          </div>
        ))}
      </section>
    </div>
  );
}
