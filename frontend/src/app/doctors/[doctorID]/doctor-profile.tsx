"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { ChamberCard } from "@/components/chamber-card";
import { Alert, Badge, ButtonLink, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type PublicDoctor } from "@/lib/api/client";

export function DoctorProfile() {
  const { doctorID } = useParams<{ doctorID: string }>();
  const [doctor, setDoctor] = useState<PublicDoctor | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    api
      .GET("/api/v1/doctors/{doctorID}", { params: { path: { doctorID } } })
      .then(({ data, error, response }) => {
        if (cancelled) return;
        if (data) setDoctor(data);
        else setError(response.status === 404 ? "This doctor could not be found." : errorMessage(error));
      });
    return () => {
      cancelled = true;
    };
  }, [doctorID]);

  if (error) {
    return (
      <div className="space-y-4">
        <Alert>{error}</Alert>
        <Link href="/doctors" className="text-sm text-teal-800 hover:underline">
          ← Back to search
        </Link>
      </div>
    );
  }
  if (!doctor) return <Spinner />;

  const chambers = (doctor.chambers ?? []).filter((c) => c.booking_enabled !== false);

  return (
    <div className="space-y-6">
      <Link href="/doctors" className="text-sm text-teal-800 hover:underline">
        ← Back to search
      </Link>

      <Card>
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 className="text-2xl font-semibold text-slate-900">{doctor.name}</h1>
            <p className="text-slate-600">
              {[doctor.specialization, doctor.degrees].filter(Boolean).join(" · ") || "General practice"}
            </p>
            <div className="mt-2 flex flex-wrap gap-2 text-xs">
              {doctor.verification_status === "approved" && <Badge tone="teal">BMDC verified</Badge>}
              {doctor.bmdc_reg_no && <Badge>BMDC {doctor.bmdc_reg_no}</Badge>}
              {doctor.experience_years ? <Badge>{doctor.experience_years} years experience</Badge> : null}
              {doctor.consultation_enabled && (
                <Badge tone="teal">Online consultation · {doctor.consultation_fee_tokens} tokens</Badge>
              )}
            </div>
          </div>
          {doctor.consultation_enabled && (
            <ButtonLink href={`/consultations/new?doctor=${doctor.user_id}`}>Consult online</ButtonLink>
          )}
        </div>
        {doctor.bio && <p className="mt-4 whitespace-pre-line text-sm text-slate-700">{doctor.bio}</p>}
      </Card>

      <section>
        <h2 className="mb-3 text-lg font-semibold text-slate-900">Chambers and sessions</h2>
        {chambers.length === 0 && (
          <p className="text-sm text-slate-600">This doctor has no chambers open for booking right now.</p>
        )}
        <div className="grid gap-4">
          {chambers.map((c) => (
            <ChamberCard key={c.id} chamber={c} />
          ))}
        </div>
      </section>
    </div>
  );
}
