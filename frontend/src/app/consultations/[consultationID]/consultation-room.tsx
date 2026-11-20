"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { ConsultationBadge } from "@/components/consultation-status";
import { RequireAuth } from "@/components/require-auth";
import { LoadFailure, loadFailure } from "@/components/load-failure";
import { ButtonLink, Card, Spinner } from "@/components/ui";
import { PrescriptionList } from "@/app/prescriptions/prescription-list";
import { useT } from "@/components/use-lang";
import { api, type Consultation, type User } from "@/lib/api/client";
import { dateTime, tokens } from "@/lib/format";
import { Actions } from "./actions";
import { Chat } from "./chat";
import { T } from "@/components/t";

// While someone else's move is pending, look for changes this often.
const STATUS_POLL_MS = 5000;

export function ConsultationRoom() {
  return <RequireAuth>{(user) => <Room user={user} />}</RequireAuth>;
}

function Room({ user }: { user: User }) {
  const t = useT();
  const { consultationID } = useParams<{ consultationID: string }>();
  const [c, setC] = useState<Consultation | null>(null);
  const [error, setError] = useState("");

  const reload = useCallback(async () => {
    const { data, error, response } = await api.GET("/api/v1/consultations/{consultationID}", {
      params: { path: { consultationID } },
    });
    if (data) setC(data);
    else setError(loadFailure(response.status, error, "consultation"));
  }, [consultationID]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    reload();
  }, [reload]);

  // Requested/accepted consultations change on the other person's action.
  const waiting = c?.status === "requested" || c?.status === "accepted" || c?.status === "active";
  useEffect(() => {
    if (!waiting) return;
    const t = setInterval(reload, STATUS_POLL_MS);
    return () => clearInterval(t);
  }, [waiting, reload]);

  if (error) return <LoadFailure failure={error} backHref="/consultations" backLabel="Back to consultations" />;
  if (!c) return <Spinner />;

  const asDoctor = user.id === c.doctor_id;
  const showChat = c.status === "active" || c.status === "completed" || (c.status === "cancelled" && c.confirmed_at);

  return (
    <div className="space-y-4">
      <Link href="/consultations" className="text-sm text-teal-800 hover:underline">
        ← <T>All consultations</T>
      </Link>

      <Card>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h1 className="text-xl font-semibold text-slate-900">{c.subject}</h1>
            <p className="text-sm text-slate-600">
              {asDoctor ? t("Patient: {name}", { name: c.patient_name }) : t("Doctor: {name}", { name: c.doctor_name })} · {tokens(c.fee_tokens)}
            </p>
            <p className="text-xs text-slate-500">{t("Requested {date}", { date: dateTime(c.requested_at) })}</p>
          </div>
          <ConsultationBadge status={c.status} asDoctor={asDoctor} />
        </div>
        <p className="mt-4 whitespace-pre-line text-sm text-slate-700">{c.description}</p>
        {c.status === "rejected" && c.reject_reason && (
          <p className="mt-3 text-sm text-red-700">Reason: {c.reject_reason}</p>
        )}
        <Actions consultation={c} asDoctor={asDoctor} onChange={setC} />
      </Card>

      {showChat && <Chat consultation={c} me={user} />}

      {(c.status === "active" || c.status === "completed") && (
        <section className="space-y-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h2 className="text-lg font-semibold text-slate-900"><T>Prescriptions</T></h2>
            {asDoctor && (
              <ButtonLink href={`/prescriptions/new?consultation_id=${c.id}`}>Write prescription</ButtonLink>
            )}
          </div>
          <PrescriptionList asDoctor={asDoctor} filter={{ consultation_id: c.id }} compact />
          {!asDoctor && (
            <p className="text-xs text-slate-500"><T>Prescriptions the doctor writes for this consultation appear here.</T></p>
          )}
        </section>
      )}
    </div>
  );
}
