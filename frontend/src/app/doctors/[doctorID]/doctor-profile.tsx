"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { ChamberCard } from "@/components/chamber-card";
import { LoadFailure, loadFailure } from "@/components/load-failure";
import { DoctorReviewCard, Stars } from "@/components/rating";
import { T } from "@/components/t";
import { Badge, ButtonLink, Card, Spinner } from "@/components/ui";
import { api, type PublicDoctor } from "@/lib/api/client";

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
        else setError(loadFailure(response.status, error, "doctor"));
      });
    return () => {
      cancelled = true;
    };
  }, [doctorID]);

  if (error) return <LoadFailure failure={error} backHref="/doctors" backLabel="Find a doctor" />;
  if (!doctor) return <Spinner />;

  const chambers = (doctor.chambers ?? []).filter((c) => c.booking_enabled !== false);

  return (
    <div className="space-y-6">
      <Link href="/doctors" className="text-sm text-teal-800 hover:underline">
        ← <T>Back to search</T>
      </Link>

      <Card>
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 className="text-2xl font-semibold text-slate-900">{doctor.name}</h1>
            <p className="text-slate-600">
              {[doctor.specialization, doctor.degrees].filter(Boolean).join(" · ") || <T>General practice</T>}
            </p>
            <p className="mt-1">
              <Stars rating={doctor.rating} count={doctor.review_count} />
            </p>
            <div className="mt-2 flex flex-wrap gap-2 text-xs">
              {doctor.verification_status === "approved" && <Badge tone="teal">BMDC verified</Badge>}
              {doctor.bmdc_reg_no && <Badge>
                  <T vars={{ no: doctor.bmdc_reg_no }}>{"BMDC {no}"}</T>
                </Badge>}
              {doctor.experience_years ? <Badge>
                  <T vars={{ n: doctor.experience_years }}>{"{n} years experience"}</T>
                </Badge> : null}
              {doctor.consultation_enabled && (
                <Badge tone="teal">
                  <T vars={{ n: doctor.consultation_fee_tokens }}>{"Online consultation · {n} tokens"}</T>
                </Badge>
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
        <h2 className="mb-3 text-lg font-semibold text-slate-900">
          <T>Chambers and sessions</T>
        </h2>
        {chambers.length === 0 && (
          <p className="text-sm text-slate-600"><T>This doctor has no chambers open for booking right now.</T></p>
        )}
        <div className="grid gap-4">
          {chambers.map((c) => (
            <ChamberCard key={c.id} chamber={c} />
          ))}
        </div>
      </section>

      <section>
        <h2 className="mb-1 text-lg font-semibold text-slate-900">
          <T>Patient reviews</T>
        </h2>
        <p className="mb-3 text-xs text-slate-500">
          <T>Only patients who visited or consulted this doctor can review. Their names are not shown.</T>
        </p>
        {(doctor.reviews ?? []).length === 0 && (
          <p className="text-sm text-slate-600">
            <T>No reviews yet.</T>
          </p>
        )}
        <ul className="space-y-2">
          {(doctor.reviews ?? []).map((r) => (
            <li key={r.id}>
              <DoctorReviewCard review={r} />
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
