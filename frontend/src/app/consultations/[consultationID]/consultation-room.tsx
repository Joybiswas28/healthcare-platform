"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { ConsultationBadge } from "@/components/consultation-status";
import { RequireAuth } from "@/components/require-auth";
import { LoadFailure, loadFailure } from "@/components/load-failure";
import { ForMember } from "@/components/family";
import { RatingForm, StarRow } from "@/components/rating";
import { SendToConsultation } from "@/components/records";
import { Alert, ButtonLink, Card, Spinner } from "@/components/ui";
import { PrescriptionList } from "@/app/prescriptions/prescription-list";
import { useT } from "@/components/use-lang";
import { api, errorMessage, type Consultation, type User } from "@/lib/api/client";
import { dateTime, tokens } from "@/lib/format";
import { Actions } from "./actions";
import { Chat } from "./chat";
import { VideoCall } from "./video-call";
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
            {c.family_member_id && (
              <p className="mt-1 flex flex-wrap items-center gap-2 text-sm text-slate-700">
                {!asDoctor && <T vars={{ name: c.patient_name ?? "" }}>{"For {name}"}</T>}
                <ForMember relation={c.relation} birthDate={c.patient_birth_date} accountName={c.account_name} showAccount={asDoctor} />
              </p>
            )}
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

      {!asDoctor && c.status === "completed" && <RateConsultation consultation={c} />}

      {c.status === "active" && c.id && (
        <VideoCall consultationID={c.id} otherName={(asDoctor ? c.patient_name : c.doctor_name) ?? ""} />
      )}

      {showChat && <Chat consultation={c} me={user} />}

      {!asDoctor && c.status === "active" && c.id && (
        <details className="rounded-xl border border-slate-200 bg-surface p-4 shadow-sm">
          <summary className="cursor-pointer text-sm font-medium text-slate-800">
            📁 <T>Send reports from my health records</T>
          </summary>
          <div className="mt-3">
            <SendToConsultation consultationID={c.id} />
          </div>
        </details>
      )}

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

// After a completed consultation the patient can rate the doctor once.
function RateConsultation({ consultation: c }: { consultation: Consultation }) {
  // undefined while loading, 0 when not rated yet
  const [mine, setMine] = useState<number | undefined>(undefined);
  const [justRated, setJustRated] = useState(false);

  useEffect(() => {
    api.GET("/api/v1/reviews/mine").then(({ data }) => {
      setMine((data?.reviews ?? []).find((r) => r.consultation_id === c.id)?.rating ?? 0);
    });
  }, [c.id]);

  async function send(rating: number, comment: string | undefined): Promise<string> {
    const { data, error } = await api.POST("/api/v1/consultations/{consultationID}/review", {
      params: { path: { consultationID: c.id ?? "" } },
      body: { rating, comment },
    });
    if (!data) return errorMessage(error);
    setMine(rating);
    setJustRated(true);
    return "";
  }

  if (mine === undefined) return null;
  if (mine > 0) {
    return justRated ? (
      <Alert kind="success">Thanks for rating the doctor.</Alert>
    ) : (
      <p className="flex items-center gap-2 text-sm text-slate-600">
        <T>You rated this consultation</T> <StarRow rating={mine} />
      </p>
    );
  }
  return (
    <Card>
      <RatingForm
        title="Rate the doctor"
        hint={<T vars={{ name: c.doctor_name ?? "" }}>{"How was your consultation with {name}? Your name is not shown with the review."}</T>}
        send={send}
      />
    </Card>
  );
}
