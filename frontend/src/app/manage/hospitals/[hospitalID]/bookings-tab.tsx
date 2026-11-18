"use client";

import { useEffect, useState, type FormEvent } from "react";
import { TextArea } from "@/components/textarea";
import { Alert, Badge, Button, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type Appointment, type Hospital, type StaffDoctor } from "@/lib/api/client";
import { clock, prettyDate, todayInDhaka, upcomingDates, WEEKDAYS } from "@/lib/format";

const STATUS: Record<string, { label: string; tone: "slate" | "teal" | "amber" | "red" }> = {
  confirmed: { label: "Confirmed", tone: "teal" },
  completed: { label: "Seen", tone: "slate" },
  cancelled: { label: "Cancelled", tone: "red" },
  no_show: { label: "No-show", tone: "amber" },
  rescheduled: { label: "Rescheduled", tone: "amber" },
};

function shiftDate(iso: string, days: number): string {
  const d = new Date(`${iso}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

export function BookingsTab({ hospital }: { hospital: Hospital }) {
  const hospitalID = hospital.id ?? "";
  const today = todayInDhaka();
  const [date, setDate] = useState(today);
  const [doctorID, setDoctorID] = useState("");
  const [doctors, setDoctors] = useState<StaffDoctor[]>([]);
  const [items, setItems] = useState<Appointment[] | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [action, setAction] = useState<{ kind: "cancel" | "reschedule"; appointment: Appointment } | null>(null);

  useEffect(() => {
    api.GET("/api/v1/manage/hospitals/{hospitalID}/doctors", { params: { path: { hospitalID } } }).then(({ data }) => {
      setDoctors((data?.doctors ?? []).filter((d) => (d.chambers ?? []).length > 0));
    });
  }, [hospitalID]);

  useEffect(() => {
    let cancelled = false;
    api
      .GET("/api/v1/manage/hospitals/{hospitalID}/appointments", {
        params: { path: { hospitalID }, query: { date, doctor_id: doctorID || undefined } },
      })
      .then(({ data, error }) => {
        if (cancelled) return;
        if (data) setItems(data.appointments ?? []);
        else setError(errorMessage(error));
      });
    return () => {
      cancelled = true;
    };
  }, [hospitalID, date, doctorID]);

  function changeDate(next: string) {
    setItems(null);
    setError("");
    setDate(next);
  }

  function replace(updated: Appointment) {
    setItems((prev) => prev?.map((x) => (x.id === updated.id ? updated : x)) ?? null);
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="secondary" onClick={() => changeDate(shiftDate(date, -1))} aria-label="Previous day">
          ←
        </Button>
        <input
          type="date"
          value={date}
          onChange={(e) => e.target.value && changeDate(e.target.value)}
          aria-label="Date"
          className="rounded-lg border border-slate-300 px-3 py-2 text-sm"
        />
        <Button variant="secondary" onClick={() => changeDate(shiftDate(date, 1))} aria-label="Next day">
          →
        </Button>
        {date !== today && (
          <Button variant="secondary" onClick={() => changeDate(today)}>
            Today
          </Button>
        )}
        <select
          value={doctorID}
          onChange={(e) => {
            setItems(null);
            setDoctorID(e.target.value);
          }}
          aria-label="Doctor"
          className="rounded-lg border border-slate-300 bg-surface px-3 py-2 text-sm"
        >
          <option value="">All doctors</option>
          {doctors.map((d) => (
            <option key={d.doctor_id} value={d.doctor_id}>
              {d.name}
            </option>
          ))}
        </select>
        <span className="text-sm text-slate-600">{prettyDate(date)}</span>
      </div>

      {notice && <Alert kind="success">{notice}</Alert>}
      {error && <Alert>{error}</Alert>}

      {items === null ? (
        !error && <Spinner />
      ) : items.length === 0 ? (
        <p className="text-sm text-slate-600">No bookings on this day.</p>
      ) : (
        <div className="space-y-3">
          {items.map((a) => {
            const s = STATUS[a.status ?? ""] ?? { label: a.status ?? "", tone: "slate" as const };
            const open = a.status === "confirmed" && (a.date ?? "") >= today;
            return (
              <Card key={a.id}>
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="flex items-start gap-4">
                    <div className="grid h-11 w-11 shrink-0 place-items-center rounded-lg bg-slate-100 font-semibold text-slate-900">
                      {a.serial_no}
                    </div>
                    <div>
                      <p className="font-semibold text-slate-900">{a.patient_name}</p>
                      <p className="text-sm text-slate-600">
                        {a.patient_phone} · <span className="font-mono">{a.ticket_code}</span>
                      </p>
                      <p className="text-xs text-slate-500">
                        {a.doctor_name} · {clock(a.start_time)}
                        {a.room ? ` · ${a.room}` : ""}
                      </p>
                    </div>
                  </div>
                  <Badge tone={s.tone}>{s.label}</Badge>
                </div>
                {a.cancel_reason && <p className="mt-2 text-xs text-slate-500">Reason: {a.cancel_reason}</p>}
                {open && (
                  <div className="mt-3 flex gap-2">
                    <Button variant="secondary" onClick={() => setAction({ kind: "reschedule", appointment: a })}>
                      Reschedule
                    </Button>
                    <Button variant="danger" onClick={() => setAction({ kind: "cancel", appointment: a })}>
                      Cancel
                    </Button>
                  </div>
                )}
                {action && action.appointment.id === a.id && (
                  <div className="mt-3 border-t border-slate-100 pt-3">
                    {action.kind === "cancel" ? (
                      <CancelForm
                        hospitalID={hospitalID}
                        appointment={a}
                        onDone={(u) => {
                          replace(u);
                          setAction(null);
                          setNotice(`Booking cancelled. ${a.patient_name} will get an SMS.`);
                        }}
                        onClose={() => setAction(null)}
                      />
                    ) : (
                      <RescheduleForm
                        hospitalID={hospitalID}
                        appointment={a}
                        doctor={doctors.find((d) => d.doctor_id === a.doctor_id)}
                        onDone={(fresh) => {
                          replace({ ...a, status: "rescheduled", rescheduled_to: fresh.id });
                          setAction(null);
                          setNotice(
                            `Moved to ${prettyDate(fresh.date)}, serial ${fresh.serial_no}, new ticket ${fresh.ticket_code}. The patient gets an SMS.`,
                          );
                        }}
                        onClose={() => setAction(null)}
                      />
                    )}
                  </div>
                )}
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}

function CancelForm({
  hospitalID,
  appointment,
  onDone,
  onClose,
}: {
  hospitalID: string;
  appointment: Appointment;
  onDone: (a: Appointment) => void;
  onClose: () => void;
}) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    const { data, error } = await api.POST("/api/v1/manage/hospitals/{hospitalID}/appointments/{appointmentID}/cancel", {
      params: { path: { hospitalID, appointmentID: appointment.id ?? "" } },
      body: { reason: String(new FormData(e.currentTarget).get("reason")).trim() },
    });
    setBusy(false);
    if (data) onDone(data);
    else setError(errorMessage(error));
  }

  return (
    <form onSubmit={submit} className="space-y-2">
      <TextArea label="Reason (sent to the patient)" name="reason" rows={2} minLength={5} required autoFocus />
      {error && <Alert>{error}</Alert>}
      <div className="flex gap-2">
        <Button type="submit" variant="danger" disabled={busy}>
          {busy ? "Cancelling…" : "Cancel booking"}
        </Button>
        <Button type="button" variant="secondary" onClick={onClose}>
          Back
        </Button>
      </div>
    </form>
  );
}

function RescheduleForm({
  hospitalID,
  appointment,
  doctor,
  onDone,
  onClose,
}: {
  hospitalID: string;
  appointment: Appointment;
  doctor?: StaffDoctor;
  onDone: (fresh: Appointment) => void;
  onClose: () => void;
}) {
  const sessions = (doctor?.chambers ?? []).flatMap((c) => c.schedules ?? []);
  const [scheduleID, setScheduleID] = useState(sessions[0]?.id ?? "");
  const weekday = sessions.find((s) => s.id === scheduleID)?.weekday ?? 0;
  const dates = upcomingDates(weekday).filter((d) => d !== appointment.date || scheduleID !== appointment.schedule_id);
  const [date, setDate] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const target = date || dates[0];
    if (!scheduleID || !target) return;
    setBusy(true);
    const { data, error } = await api.POST(
      "/api/v1/manage/hospitals/{hospitalID}/appointments/{appointmentID}/reschedule",
      { params: { path: { hospitalID, appointmentID: appointment.id ?? "" } }, body: { schedule_id: scheduleID, date: target } },
    );
    setBusy(false);
    if (data) onDone(data);
    else setError(errorMessage(error));
  }

  if (sessions.length === 0) {
    return <Alert kind="info">This doctor has no other sessions at the hospital to move the booking to.</Alert>;
  }

  return (
    <form onSubmit={submit} className="flex flex-wrap items-end gap-2">
      <label className="text-sm">
        <span className="mb-1 block font-medium text-slate-700">Session</span>
        <select
          value={scheduleID}
          onChange={(e) => {
            setScheduleID(e.target.value);
            setDate("");
          }}
          className="rounded-lg border border-slate-300 bg-surface px-3 py-2"
        >
          {sessions.map((s) => (
            <option key={s.id} value={s.id}>
              {WEEKDAYS[s.weekday ?? 0]} {clock(s.start_time)}
            </option>
          ))}
        </select>
      </label>
      <label className="text-sm">
        <span className="mb-1 block font-medium text-slate-700">Date</span>
        <select value={date || dates[0] || ""} onChange={(e) => setDate(e.target.value)} className="rounded-lg border border-slate-300 bg-surface px-3 py-2">
          {dates.map((d) => (
            <option key={d} value={d}>
              {prettyDate(d)}
            </option>
          ))}
        </select>
      </label>
      <Button type="submit" disabled={busy || dates.length === 0}>
        {busy ? "Moving…" : "Move booking"}
      </Button>
      <Button type="button" variant="secondary" onClick={onClose}>
        Back
      </Button>
      {error && (
        <div className="w-full">
          <Alert>{error}</Alert>
        </div>
      )}
    </form>
  );
}
