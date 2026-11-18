"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";
import { RequireAuth } from "@/components/require-auth";
import { TextArea } from "@/components/textarea";
import { Alert, Button, Card, Field, Spinner } from "@/components/ui";
import { api, errorMessage, type PublicDoctor } from "@/lib/api/client";
import { tokens } from "@/lib/format";

export function RequestForm() {
  const doctorID = useSearchParams().get("doctor") ?? "";
  return (
    <RequireAuth>
      {(user) =>
        user.role === "patient" ? (
          <Form doctorID={doctorID} />
        ) : (
          <p className="text-sm text-slate-600">Only patient accounts can request a consultation.</p>
        )
      }
    </RequireAuth>
  );
}

function Form({ doctorID }: { doctorID: string }) {
  const router = useRouter();
  const [doctor, setDoctor] = useState<PublicDoctor | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!doctorID) return;
    api.GET("/api/v1/doctors/{doctorID}", { params: { path: { doctorID } } }).then(({ data, error }) => {
      if (data) setDoctor(data);
      else setError(errorMessage(error, "This doctor could not be found."));
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
      },
    });
    setBusy(false);
    if (!data?.id) {
      setError(errorMessage(error, "Could not send the request."));
      return;
    }
    router.push(`/consultations/${data.id}`);
  }

  if (!doctorID) {
    return (
      <Alert kind="info">
        Choose a doctor first.{" "}
        <Link href="/doctors?online=true" className="font-medium underline">
          Find doctors who consult online
        </Link>
      </Alert>
    );
  }
  if (!doctor) return error ? <Alert>{error}</Alert> : <Spinner />;
  if (!doctor.consultation_enabled) {
    return <Alert>{doctor.name} is not taking online consultations right now.</Alert>;
  }

  return (
    <Card>
      <form onSubmit={onSubmit} className="space-y-4">
        <div className="rounded-lg bg-slate-50 p-3 text-sm text-slate-700">
          <p className="font-medium text-slate-900">{doctor.name}</p>
          <p>
            Fee: {tokens(doctor.consultation_fee_tokens)}. Nothing is charged until the doctor accepts and you confirm.
          </p>
        </div>
        <Field label="Subject" name="subject" minLength={5} maxLength={150} required placeholder="e.g. Fever for three days" />
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
