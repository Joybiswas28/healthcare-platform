"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";
import { RequireAuth } from "@/components/require-auth";
import { TextArea } from "@/components/textarea";
import { Alert, Button, Card, Field, Select, Spinner } from "@/components/ui";
import { api, errorMessage, type Medicine, type PrescriptionInput } from "@/lib/api/client";
import { prettyDate } from "@/lib/format";

const MAX_MEDICINES = 30;
const MAX_INVESTIGATIONS = 20;

const EMPTY_MEDICINE: Medicine = { form: "Tab.", name: "", strength: "", dosage: "", duration: "", instructions: "" };

// Who the prescription is for. It comes from the URL: either ?consultation_id=
// (from a consultation) or ?appointment_id= (from the day's patient list).
type Source = { kind: "consultation" | "appointment"; id: string; date?: string };

export function PrescriptionForm() {
  const params = useSearchParams();
  const consultation = params.get("consultation_id");
  const appointment = params.get("appointment_id");
  const source: Source | null = consultation
    ? { kind: "consultation", id: consultation }
    : appointment
      ? { kind: "appointment", id: appointment, date: params.get("date") ?? undefined }
      : null;

  return (
    <RequireAuth>
      {(user) =>
        user.role !== "doctor" ? (
          <p className="text-sm text-slate-600">Only doctors can write prescriptions.</p>
        ) : !source ? (
          <Alert kind="info">
            Open a consultation or your patient list and choose &quot;Write prescription&quot; from there.
          </Alert>
        ) : (
          <Form source={source} />
        )
      }
    </RequireAuth>
  );
}

function Form({ source }: { source: Source }) {
  const router = useRouter();
  const [context, setContext] = useState<string | null>(null);
  const [medicines, setMedicines] = useState<Medicine[]>([{ ...EMPTY_MEDICINE }]);
  const [investigations, setInvestigations] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  // Show who this is for, so the doctor can't prescribe to the wrong patient by accident.
  useEffect(() => {
    if (source.kind === "consultation") {
      api
        .GET("/api/v1/consultations/{consultationID}", { params: { path: { consultationID: source.id } } })
        .then(({ data, error }) =>
          data ? setContext(`${data.patient_name} · Consultation: ${data.subject}`) : setError(errorMessage(error)),
        );
    } else {
      // The doctor's day list is the only appointment lookup the API offers.
      api
        .GET("/api/v1/doctor/appointments", { params: { query: { date: source.date || undefined } } })
        .then(({ data, error }) => {
          const a = data?.appointments?.find((x) => x.id === source.id);
          if (a) setContext(`${a.patient_name} · Serial ${a.serial_no} on ${prettyDate(a.date)} at ${a.chamber_name}`);
          else if (data) setContext("Patient from your appointment list");
          else setError(errorMessage(error));
        });
    }
  }, [source.kind, source.id, source.date]);

  function updateMedicine(i: number, field: keyof Medicine, value: string) {
    setMedicines((prev) => prev.map((m, j) => (j === i ? { ...m, [field]: value } : m)));
  }

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const text = (name: string) => String(form.get(name) ?? "").trim() || undefined;

    const meds = medicines
      .map((m) => Object.fromEntries(Object.entries(m).map(([k, v]) => [k, (v ?? "").trim()])) as Medicine)
      .filter((m) => m.name);
    if (!text("chief_complaints") && !text("diagnosis")) {
      setError("Write the chief complaints or the diagnosis (or both).");
      return;
    }
    const incomplete = meds.find((m) => !m.dosage || !m.duration);
    if (incomplete) {
      setError(`Add the dosage and duration for ${incomplete.name}.`);
      return;
    }

    const body: PrescriptionInput = {
      [source.kind === "consultation" ? "consultation_id" : "appointment_id"]: source.id,
      patient_age: text("patient_age"),
      patient_gender: text("patient_gender") as PrescriptionInput["patient_gender"],
      chief_complaints: text("chief_complaints"),
      diagnosis: text("diagnosis"),
      medicines: meds.map((m) => ({
        ...m,
        strength: m.strength || undefined,
        form: m.form || undefined,
        instructions: m.instructions || undefined,
      })),
      investigations: investigations.map((s) => s.trim()).filter(Boolean),
      advice: text("advice"),
      follow_up_date: text("follow_up_date"),
    };
    if (!confirm("Issue this prescription? It can't be edited afterwards, only revoked.")) return;

    setBusy(true);
    setError("");
    const { data, error } = await api.POST("/api/v1/prescriptions", { body });
    setBusy(false);
    if (data?.id) router.push(`/prescriptions/${data.id}`);
    else setError(errorMessage(error));
  }

  if (context === null && !error) return <Spinner />;

  const input =
    "w-full rounded-lg border border-slate-300 bg-surface px-2 py-1.5 text-sm outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-600/20";

  return (
    <form onSubmit={onSubmit} className="space-y-4">
      {context && <Alert kind="info">For: {context}</Alert>}

      <Card className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Patient's age" name="patient_age" placeholder="e.g. 34 years" maxLength={30} />
          <Select label="Gender" name="patient_gender" defaultValue="">
            <option value="">—</option>
            <option value="male">Male</option>
            <option value="female">Female</option>
            <option value="other">Other</option>
          </Select>
        </div>
        <TextArea label="Chief complaints" name="chief_complaints" rows={2} />
        <TextArea label="Diagnosis" name="diagnosis" rows={2} />
      </Card>

      <Card className="space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="font-semibold text-slate-900">Medicines (Rx)</h2>
          <Button
            type="button"
            variant="secondary"
            disabled={medicines.length >= MAX_MEDICINES}
            onClick={() => setMedicines((prev) => [...prev, { ...EMPTY_MEDICINE }])}
          >
            + Add medicine
          </Button>
        </div>
        {medicines.map((m, i) => (
          <fieldset key={i} className="rounded-lg border border-slate-200 p-3">
            <legend className="px-1 text-xs text-slate-500">Medicine {i + 1}</legend>
            <div className="grid gap-2 sm:grid-cols-[5rem_1fr_7rem]">
              <input aria-label="Form" placeholder="Tab." value={m.form ?? ""} onChange={(e) => updateMedicine(i, "form", e.target.value)} className={input} />
              <input aria-label="Name" placeholder="Name, e.g. Napa" value={m.name} onChange={(e) => updateMedicine(i, "name", e.target.value)} className={input} />
              <input aria-label="Strength" placeholder="500 mg" value={m.strength ?? ""} onChange={(e) => updateMedicine(i, "strength", e.target.value)} className={input} />
            </div>
            <div className="mt-2 grid gap-2 sm:grid-cols-[7rem_7rem_1fr_auto]">
              <input aria-label="Dosage" placeholder="1+0+1" value={m.dosage} onChange={(e) => updateMedicine(i, "dosage", e.target.value)} className={input} />
              <input aria-label="Duration" placeholder="5 days" value={m.duration} onChange={(e) => updateMedicine(i, "duration", e.target.value)} className={input} />
              <input
                aria-label="Instructions"
                placeholder="after meal"
                value={m.instructions ?? ""}
                onChange={(e) => updateMedicine(i, "instructions", e.target.value)}
                className={input}
              />
              <button
                type="button"
                onClick={() => setMedicines((prev) => prev.filter((_, j) => j !== i))}
                className="rounded-lg px-2 text-sm text-red-700 hover:bg-red-50"
                aria-label={`Remove medicine ${i + 1}`}
              >
                Remove
              </button>
            </div>
          </fieldset>
        ))}
      </Card>

      <Card className="space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="font-semibold text-slate-900">Investigations</h2>
          <Button
            type="button"
            variant="secondary"
            disabled={investigations.length >= MAX_INVESTIGATIONS}
            onClick={() => setInvestigations((prev) => [...prev, ""])}
          >
            + Add test
          </Button>
        </div>
        {investigations.length === 0 && <p className="text-sm text-slate-500">No tests.</p>}
        {investigations.map((t, i) => (
          <div key={i} className="flex gap-2">
            <input
              aria-label={`Test ${i + 1}`}
              placeholder="e.g. CBC"
              value={t}
              onChange={(e) => setInvestigations((prev) => prev.map((x, j) => (j === i ? e.target.value : x)))}
              className={input}
            />
            <button
              type="button"
              onClick={() => setInvestigations((prev) => prev.filter((_, j) => j !== i))}
              className="rounded-lg px-2 text-sm text-red-700 hover:bg-red-50"
            >
              Remove
            </button>
          </div>
        ))}
      </Card>

      <Card className="space-y-4">
        <TextArea label="Advice" name="advice" rows={3} maxLength={2000} />
        <Field label="Follow-up date (optional)" name="follow_up_date" type="date" />
      </Card>

      {error && <Alert>{error}</Alert>}
      <Button type="submit" disabled={busy}>
        {busy ? "Issuing…" : "Issue prescription"}
      </Button>
    </form>
  );
}
