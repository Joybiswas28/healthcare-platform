"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { api, errorMessage, type HealthRecord } from "@/lib/api/client";
import { fileSize, prettyDate } from "@/lib/format";
import { T } from "./t";
import { Alert, Badge, Button } from "./ui";
import { useT } from "./use-lang";

export const RECORD_KINDS: Record<string, string> = {
  lab_report: "Lab report",
  imaging: "X-ray or scan",
  prescription: "Prescription",
  discharge: "Discharge summary",
  vaccination: "Vaccination",
  other: "Other",
};

export const KIND_ICON: Record<string, string> = {
  lab_report: "🧪",
  imaging: "🩻",
  prescription: "📄",
  discharge: "🏥",
  vaccination: "💉",
  other: "📁",
};

export function fileHref(id: string | undefined) {
  return `/api/v1/records/${id}/file`;
}

// One line about a record: icon, title, kind, date, size, whose.
export function RecordLine({ record: r, showPerson = true }: { record: HealthRecord; showPerson?: boolean }) {
  const t = useT();
  return (
    <div className="min-w-0">
      <a href={fileHref(r.id)} className="font-medium text-teal-800 hover:underline">
        <span aria-hidden>{KIND_ICON[r.kind ?? "other"]}</span> {r.title}
      </a>
      <p className="text-xs text-slate-500">
        {[t(RECORD_KINDS[r.kind ?? "other"] ?? ""), r.record_date ? prettyDate(r.record_date) : null, fileSize(r.size_bytes), showPerson ? r.person_name : null]
          .filter(Boolean)
          .join(" · ")}
      </p>
    </div>
  );
}

function useMyRecords() {
  const [records, setRecords] = useState<HealthRecord[] | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    api.GET("/api/v1/records").then(({ data, error }) => {
      if (data) setRecords(data.records ?? []);
      else setError(errorMessage(error));
    });
  }, []);
  return { records, setRecords, error };
}

function NoRecords() {
  return (
    <p className="text-sm text-slate-600">
      <T>You have no health records yet.</T>{" "}
      <Link href="/records" className="font-medium text-teal-800 hover:underline">
        <T>Upload reports</T>
      </Link>
    </p>
  );
}

// Patient: tick the records the doctor of this appointment may see.
export function ShareWithAppointment({ appointmentID }: { appointmentID: string }) {
  const { records, setRecords, error: loadError } = useMyRecords();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState("");

  async function toggle(r: HealthRecord, on: boolean) {
    if (!r.id) return;
    setBusy(r.id);
    setError("");
    const res = on
      ? await api.POST("/api/v1/records/{recordID}/share", { params: { path: { recordID: r.id } }, body: { appointment_id: appointmentID } })
      : await api.DELETE("/api/v1/records/{recordID}/shares/{appointmentID}", { params: { path: { recordID: r.id, appointmentID } } });
    setBusy(null);
    const updated = res.data && "record" in res.data ? res.data.record : (res.data as HealthRecord | undefined);
    if (!updated) {
      setError(errorMessage(res.error));
      return;
    }
    setRecords((prev) => (prev ?? []).map((x) => (x.id === updated.id ? updated : x)));
  }

  if (loadError) return <Alert>{loadError}</Alert>;
  if (!records) return null;
  if (records.length === 0) return <NoRecords />;
  return (
    <div className="space-y-2">
      <p className="text-sm text-slate-600">
        <T>Ticked records can be opened by this doctor until the appointment is cancelled or you untick them.</T>
      </p>
      <ul className="space-y-2">
        {records.map((r) => {
          const on = (r.shared_with ?? []).some((s) => s.appointment_id === appointmentID);
          return (
            <li key={r.id} className="flex items-start gap-3">
              <input
                type="checkbox"
                className="mt-1 h-4 w-4 accent-teal-700"
                checked={on}
                disabled={busy === r.id}
                onChange={(e) => toggle(r, e.target.checked)}
                aria-label={r.title}
              />
              <RecordLine record={r} />
            </li>
          );
        })}
      </ul>
      {error && <Alert>{error}</Alert>}
    </div>
  );
}

// Patient: send copies of records into an active consultation chat.
export function SendToConsultation({ consultationID, onSent }: { consultationID: string; onSent?: () => void }) {
  const { records, error: loadError } = useMyRecords();
  const [sent, setSent] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState("");

  async function send(r: HealthRecord) {
    if (!r.id) return;
    setBusy(r.id);
    setError("");
    const { data, error } = await api.POST("/api/v1/records/{recordID}/share", {
      params: { path: { recordID: r.id } },
      body: { consultation_id: consultationID },
    });
    setBusy(null);
    if (!data) {
      setError(errorMessage(error));
      return;
    }
    setSent((prev) => new Set(prev).add(r.id ?? ""));
    onSent?.();
  }

  if (loadError) return <Alert>{loadError}</Alert>;
  if (!records) return null;
  if (records.length === 0) return <NoRecords />;
  return (
    <div className="space-y-2">
      <ul className="space-y-2">
        {records.map((r) => (
          <li key={r.id} className="flex flex-wrap items-center justify-between gap-2">
            <RecordLine record={r} />
            {sent.has(r.id ?? "") ? (
              <Badge tone="teal">Sent</Badge>
            ) : (
              <Button variant="secondary" onClick={() => send(r)} disabled={busy === r.id}>
                {busy === r.id ? "Sending…" : "Send to chat"}
              </Button>
            )}
          </li>
        ))}
      </ul>
      {error && <Alert>{error}</Alert>}
    </div>
  );
}

// Doctor: records a patient shared with an appointment.
export function SharedRecords({ appointmentID, count }: { appointmentID: string; count: number }) {
  const [open, setOpen] = useState(false);
  const [records, setRecords] = useState<HealthRecord[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open || records) return;
    api
      .GET("/api/v1/doctor/appointments/{appointmentID}/records", { params: { path: { appointmentID } } })
      .then(({ data, error }) => (data ? setRecords(data.records ?? []) : setError(errorMessage(error))));
  }, [open, records, appointmentID]);

  if (!count) return null;
  return (
    <div className="mt-3 border-t border-slate-100 pt-3">
      <button onClick={() => setOpen(!open)} className="text-sm font-medium text-teal-800 hover:underline" aria-expanded={open}>
        📎 <T vars={{ n: count }}>{"Shared health records: {n}"}</T> {open ? "▴" : "▾"}
      </button>
      {open && (
        <div className="mt-2 space-y-2">
          {error && <Alert>{error}</Alert>}
          {records?.map((r) => (
            <div key={r.id}>
              <RecordLine record={r} />
              {r.note && <p className="text-xs text-slate-600">{r.note}</p>}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
