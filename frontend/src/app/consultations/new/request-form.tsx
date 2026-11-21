"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";
import { PersonPicker, useFamily } from "@/components/family";
import { RequireAuth } from "@/components/require-auth";
import { TextArea } from "@/components/textarea";
import { LoadFailure, loadFailure } from "@/components/load-failure";
import { Alert, Button, Card, Field, Spinner } from "@/components/ui";
import { useT } from "@/components/use-lang";
import { api, errorMessage, type PublicDoctor } from "@/lib/api/client";
import { tokens } from "@/lib/format";
import { T } from "@/components/t";

export function RequestForm() {
  const doctorID = useSearchParams().get("doctor") ?? "";
  return (
    <RequireAuth>
      {(user) =>
        user.role === "patient" ? (
          <Form doctorID={doctorID} me={user.name ?? ""} />
        ) : (
          <p className="text-sm text-slate-600"><T>Only patient accounts can request a consultation.</T></p>
        )
      }
    </RequireAuth>
  );
}

function Form({ doctorID, me }: { doctorID: string; me: string }) {
  const t = useT();
  const family = useFamily();
  const [forMember, setForMember] = useState("");
  const router = useRouter();
  const [doctor, setDoctor] = useState<PublicDoctor | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!doctorID) return;
    api.GET("/api/v1/doctors/{doctorID}", { params: { path: { doctorID } } }).then(({ data, error, response }) => {
      if (data) setDoctor(data);
      else setError(loadFailure(response.status, error, "doctor"));
    });
  }, [doctorID]);

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy(true);
    setError("");
    const { data, error } = await api.POST("/api/v1/consultations", {
      body: {
        doctor_id: doctorID,
        subject: String(form.get("subject")).trim(),
        description: String(form.get("description")).trim(),
        family_member_id: forMember || undefined,
      },
    });
    setBusy(false);
    if (!data?.id) {
      setError(errorMessage(error, t("Could not send the request.")));
      return;
    }
    router.push(`/consultations/${data.id}`);
  }

  if (!doctorID) {
    return (
      <Alert kind="info">
        <T>Choose a doctor first.</T>{" "}
        <Link href="/doctors?online=true" className="font-medium underline">
          <T>Find doctors who consult online</T>
        </Link>
      </Alert>
    );
  }
  if (!doctor)
    return error ? <LoadFailure failure={error} backHref="/doctors?online=true" backLabel="Find a doctor" /> : <Spinner />;
  if (!doctor.consultation_enabled) {
    return <Alert>{t("{name} is not taking online consultations right now.", { name: doctor.name })}</Alert>;
  }

  return (
    <Card>
      <form onSubmit={onSubmit} className="space-y-4">
        <div className="rounded-lg bg-slate-50 p-3 text-sm text-slate-700">
          <p className="font-medium text-slate-900">{doctor.name}</p>
          <p>
            {t("Fee: {fee}. Nothing is charged until the doctor accepts and you confirm.", { fee: tokens(doctor.consultation_fee_tokens) })}
          </p>
        </div>
        {family && <PersonPicker members={family} me={me} value={forMember} onChange={setForMember} />}
        <Field label="Subject" name="subject" minLength={5} maxLength={150} required placeholder={t("e.g. Fever for three days")} />
        <TextArea
          label="Describe the problem"
          name="description"
          minLength={20}
          maxLength={5000}
          required
          hint="Symptoms, how long, medicines you take, anything the doctor should know. You can share reports after the chat starts."
        />
        {error && <Alert>{error}</Alert>}
        <Button type="submit" disabled={busy}>
          {busy ? "Sending…" : "Send request"}
        </Button>
      </form>
    </Card>
  );
}
