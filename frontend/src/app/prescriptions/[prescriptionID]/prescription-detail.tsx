"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";
import { MedicineList } from "@/components/prescription-view";
import { RequireAuth } from "@/components/require-auth";
import { TextArea } from "@/components/textarea";
import { Alert, Badge, Button, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type Prescription, type User } from "@/lib/api/client";
import { dateTime, prettyDate } from "@/lib/format";

export function PrescriptionDetail() {
  return <RequireAuth>{(user) => <Detail user={user} />}</RequireAuth>;
}

function Detail({ user }: { user: User }) {
  const { prescriptionID } = useParams<{ prescriptionID: string }>();
  const [rx, setRx] = useState<Prescription | null>(null);
  const [error, setError] = useState("");
  const [revoking, setRevoking] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api
      .GET("/api/v1/prescriptions/{prescriptionID}", { params: { path: { prescriptionID } } })
      .then(({ data, error, response }) => {
        if (data) setRx(data);
        else setError(response.status === 404 ? "This prescription could not be found." : errorMessage(error));
      });
  }, [prescriptionID]);

  async function revoke(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const reason = String(new FormData(e.currentTarget).get("reason")).trim();
    setBusy(true);
    setError("");
    const { data, error } = await api.POST("/api/v1/prescriptions/{prescriptionID}/revoke", {
      params: { path: { prescriptionID } },
      body: { reason },
    });
    setBusy(false);
    if (data) {
      setRx(data);
      setRevoking(false);
    } else setError(errorMessage(error));
  }

  if (!rx) return error ? <Alert>{error}</Alert> : <Spinner />;

  const isDoctor = user.id === rx.doctor_id;
  const revoked = rx.status === "revoked";

  return (
    <div className="space-y-4">
      <Link href="/prescriptions" className="text-sm text-teal-800 hover:underline">
        ← All prescriptions
      </Link>

      {revoked && (
        <Alert>
          The doctor revoked this prescription on {dateTime(rx.revoked_at)}
          {rx.revoked_reason ? `: ${rx.revoked_reason}` : "."} Do not use it to buy medicine.
        </Alert>
      )}

      <Card className="space-y-5">
        <div className="flex flex-wrap items-start justify-between gap-4 border-b border-slate-100 pb-4">
          <div>
            <p className="text-lg font-semibold text-slate-900">{rx.doctor_name}</p>
            <p className="text-sm text-slate-600">{[rx.doctor_degrees, rx.doctor_specialization].filter(Boolean).join(" · ")}</p>
            <p className="text-xs text-slate-500">BMDC {rx.doctor_bmdc}</p>
          </div>
          <div className="text-right text-sm">
            {revoked ? <Badge tone="red">Revoked</Badge> : <Badge tone="teal">Valid</Badge>}
            <p className="mt-1 text-slate-600">{dateTime(rx.issued_at)}</p>
            <p className="font-mono text-xs text-slate-500">Code {rx.code}</p>
          </div>
        </div>

        <div className="text-sm text-slate-700">
          <span className="font-medium text-slate-900">{rx.patient_name}</span>
          {rx.patient_age ? ` · ${rx.patient_age}` : ""}
          {rx.patient_gender ? ` · ${rx.patient_gender}` : ""}
        </div>

        {rx.chief_complaints && <Section title="Chief complaints">{rx.chief_complaints}</Section>}
        {rx.diagnosis && <Section title="Diagnosis">{rx.diagnosis}</Section>}

        <div>
          <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-slate-500">Rx</h2>
          <MedicineList medicines={rx.medicines ?? []} />
        </div>

        {(rx.investigations ?? []).length > 0 && (
          <Section title="Investigations">
            <ul className="list-inside list-disc">
              {rx.investigations!.map((t, i) => (
                <li key={i}>{t}</li>
              ))}
            </ul>
          </Section>
        )}
        {rx.advice && <Section title="Advice">{rx.advice}</Section>}
        {rx.follow_up_date && <Section title="Follow-up">{prettyDate(rx.follow_up_date)}</Section>}
      </Card>

      {error && <Alert>{error}</Alert>}

      <div className="flex flex-wrap gap-2">
        {!revoked && (
          <a
            href={`/api/v1/prescriptions/${rx.id}/print`}
            target="_blank"
            rel="noopener"
            className="inline-flex items-center rounded-lg bg-teal-700 px-4 py-2 text-sm font-medium text-white hover:bg-teal-800"
          >
            Print / save as PDF
          </a>
        )}
        {rx.consultation_id && (
          <Link
            href={`/consultations/${rx.consultation_id}`}
            className="inline-flex items-center rounded-lg border border-slate-300 bg-white px-4 py-2 text-sm font-medium text-slate-800 hover:bg-slate-50"
          >
            Open consultation
          </Link>
        )}
        {isDoctor && !revoked && !revoking && (
          <Button variant="danger" onClick={() => setRevoking(true)}>
            Revoke
          </Button>
        )}
      </div>

      {revoking && (
        <Card>
          <form onSubmit={revoke} className="space-y-3">
            <p className="text-sm text-slate-600">
              Prescriptions are never edited. Revoke this one and write a new one if something needs to change.
            </p>
            <TextArea label="Reason" name="reason" rows={2} minLength={5} required autoFocus />
            <div className="flex gap-2">
              <Button type="submit" variant="danger" disabled={busy}>
                {busy ? "Revoking…" : "Revoke prescription"}
              </Button>
              <Button type="button" variant="secondary" onClick={() => setRevoking(false)}>
                Back
              </Button>
            </div>
          </form>
        </Card>
      )}
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <h2 className="mb-1 text-sm font-semibold uppercase tracking-wide text-slate-500">{title}</h2>
      <div className="whitespace-pre-line text-sm text-slate-800">{children}</div>
    </div>
  );
}
