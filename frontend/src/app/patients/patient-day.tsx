"use client";

import { useEffect, useState, type FormEvent } from "react";
import { Alert, Badge, Button, ButtonLink, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type Appointment } from "@/lib/api/client";
import { clock, prettyDate, todayInDhaka } from "@/lib/format";

const STATUS: Record<string, { label: string; tone: "slate" | "teal" | "amber" | "red" }> = {
  confirmed: { label: "Waiting", tone: "teal" },
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

export function PatientDay() {
  const today = todayInDhaka();
  const [date, setDate] = useState(today);
  const [items, setItems] = useState<Appointment[] | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [ticket, setTicket] = useState<Appointment | null>(null);
  const [ticketError, setTicketError] = useState("");

  useEffect(() => {
    let cancelled = false;
    api.GET("/api/v1/doctor/appointments", { params: { query: { date } } }).then(({ data, error }) => {
      if (cancelled) return;
      if (data) setItems(data.appointments ?? []);
      else setError(errorMessage(error));
    });
    return () => {
      cancelled = true;
    };
  }, [date]);

  function changeDate(next: string) {
    setItems(null);
    setError("");
    setDate(next);
  }

  async function outcome(a: Appointment, kind: "complete" | "no-show") {
    if (!a.id) return;
    if (kind === "no-show" && !confirm(`Mark ${a.patient_name} as not showing up?`)) return;
    setBusy(a.id);
    setError("");
    const path =
      kind === "complete"
        ? "/api/v1/doctor/appointments/{appointmentID}/complete"
        : "/api/v1/doctor/appointments/{appointmentID}/no-show";
    const { data, error } = await api.POST(path, { params: { path: { appointmentID: a.id } } });
    setBusy(null);
    if (data) {
      setItems((prev) => prev?.map((x) => (x.id === data.id ? data : x)) ?? null);
      if (ticket?.id === data.id) setTicket(data);
    } else setError(errorMessage(error));
  }

  async function lookUp(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const code = String(new FormData(e.currentTarget).get("code")).trim().toUpperCase();
    setTicket(null);
    setTicketError("");
    if (!code) return;
    const { data, error, response } = await api.GET("/api/v1/doctor/tickets/{code}", { params: { path: { code } } });
    if (data) setTicket(data);
    else setTicketError(response.status === 404 ? "No booking with this ticket code for you." : errorMessage(error));
  }

  const past = date <= today;

  function row(a: Appointment) {
    const s = STATUS[a.status ?? ""] ?? { label: a.status ?? "", tone: "slate" as const };
    return (
      <Card key={a.id}>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex items-start gap-4">
            <div className="grid h-12 w-12 shrink-0 place-items-center rounded-lg bg-slate-100 text-lg font-semibold text-slate-900">
              {a.serial_no}
            </div>
            <div>
              <p className="font-semibold text-slate-900">{a.patient_name}</p>
              <p className="text-sm text-slate-600">
                {a.patient_phone} · <span className="font-mono">{a.ticket_code}</span>
              </p>
              <p className="text-xs text-slate-500">
                {prettyDate(a.date)} · {a.chamber_name}, {clock(a.start_time)}
              </p>
            </div>
          </div>
          <Badge tone={s.tone}>{s.label}</Badge>
        </div>
        <div className="mt-3 flex flex-wrap gap-2">
          {a.status === "confirmed" && a.date && a.date <= today && (
            <>
              <Button onClick={() => outcome(a, "complete")} disabled={busy === a.id}>
                Mark seen
              </Button>
              <Button variant="secondary" onClick={() => outcome(a, "no-show")} disabled={busy === a.id}>
                No-show
              </Button>
            </>
          )}
          {a.status !== "cancelled" && a.date && a.date <= today && (
            <ButtonLink href={`/prescriptions/new?appointment_id=${a.id}&date=${a.date}`} variant="secondary">
              Write prescription
            </ButtonLink>
          )}
        </div>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <Card>
        <form onSubmit={lookUp} className="flex flex-wrap items-end gap-2">
          <label className="flex-1">
            <span className="mb-1 block text-sm font-medium text-slate-700">Patient&apos;s ticket code</span>
            <input
              name="code"
              placeholder="e.g. K7Q2M4XA"
              className="w-full rounded-lg border border-slate-300 px-3 py-2 font-mono uppercase outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-600/20"
            />
          </label>
          <Button type="submit" variant="secondary">
            Look up
          </Button>
        </form>
        {ticketError && <div className="mt-3"><Alert>{ticketError}</Alert></div>}
        {ticket && <div className="mt-3">{row(ticket)}</div>}
      </Card>

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
        <span className="text-sm text-slate-600">{prettyDate(date)}</span>
      </div>

      {error && <Alert>{error}</Alert>}
      {items === null ? (
        !error && <Spinner />
      ) : items.length === 0 ? (
        <p className="text-sm text-slate-600">No bookings {past ? "on" : "yet for"} this day.</p>
      ) : (
        <div className="space-y-3">{items.map(row)}</div>
      )}
    </div>
  );
}
