"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, errorMessage, type Appointment, type Queue, type Turn } from "@/lib/api/client";
import { clock } from "@/lib/format";
import { T } from "./t";
import { Alert, Button, Card } from "./ui";
import { useT } from "./use-lang";

// Live serial status ("now seeing serial 8") for today's chamber sessions.

// Re-run fn every `ms` while the tab is visible, and as soon as it becomes visible again.
function usePolling(fn: () => void, ms: number) {
  useEffect(() => {
    const timer = setInterval(() => document.visibilityState === "visible" && fn(), ms);
    const onVisible = () => document.visibilityState === "visible" && fn();
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [fn, ms]);
}

// Who runs the queue: the doctor, or a hospital (for its chambers).
export type QueueOwner = { kind: "doctor" } | { kind: "hospital"; hospitalID: string };

type Session = { scheduleID: string; label: string; start?: string; end?: string };

// One control per session that has bookings today, in the order they start.
export function TodaysQueues({ owner, appointments }: { owner: QueueOwner; appointments: Appointment[] }) {
  const sessions = new Map<string, Session>();
  for (const a of appointments) {
    if (!a.schedule_id || sessions.has(a.schedule_id)) continue;
    const label = owner.kind === "hospital" ? (a.doctor_name ?? "") : (a.chamber_name ?? "");
    sessions.set(a.schedule_id, { scheduleID: a.schedule_id, label, start: a.start_time, end: a.end_time });
  }
  const list = [...sessions.values()].sort((x, y) => (x.start ?? "").localeCompare(y.start ?? ""));
  if (list.length === 0) return null;
  return (
    <div className="space-y-3">
      {list.map((s) => (
        <QueueControl
          key={s.scheduleID}
          owner={owner}
          session={s}
          patients={appointments.filter((a) => a.schedule_id === s.scheduleID)}
        />
      ))}
    </div>
  );
}

function QueueControl({ owner, session, patients }: { owner: QueueOwner; session: Session; patients: Appointment[] }) {
  const t = useT();
  const [queue, setQueue] = useState<Queue | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [editing, setEditing] = useState(false);
  const scheduleID = session.scheduleID;
  // Plain strings, so the callbacks below stay the same across renders.
  const hospitalID = owner.kind === "hospital" ? owner.hospitalID : "";

  const load = useCallback(async () => {
    const res = !hospitalID
      ? await api.GET("/api/v1/doctor/schedules/{scheduleID}/queue", { params: { path: { scheduleID } } })
      : await api.GET("/api/v1/manage/hospitals/{hospitalID}/schedules/{scheduleID}/queue", {
          params: { path: { hospitalID, scheduleID } },
        });
    if (res.data) setQueue(res.data);
  }, [hospitalID, scheduleID]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load();
  }, [load]);
  // Another staff member may be calling patients too.
  usePolling(load, 30_000);

  async function next() {
    setBusy(true);
    setError("");
    const res = !hospitalID
      ? await api.POST("/api/v1/doctor/schedules/{scheduleID}/queue/next", { params: { path: { scheduleID } } })
      : await api.POST("/api/v1/manage/hospitals/{hospitalID}/schedules/{scheduleID}/queue/next", {
          params: { path: { hospitalID, scheduleID } },
        });
    setBusy(false);
    if (res.data) setQueue(res.data);
    else setError(errorMessage(res.error));
  }

  async function set(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const serial = Number(new FormData(e.currentTarget).get("serial"));
    if (!Number.isInteger(serial) || serial < 1) return;
    setBusy(true);
    setError("");
    const body = { now_serving: serial };
    const res = !hospitalID
      ? await api.PUT("/api/v1/doctor/schedules/{scheduleID}/queue", { params: { path: { scheduleID } }, body })
      : await api.PUT("/api/v1/manage/hospitals/{hospitalID}/schedules/{scheduleID}/queue", {
          params: { path: { hospitalID, scheduleID } },
          body,
        });
    setBusy(false);
    if (res.data) {
      setQueue(res.data);
      setEditing(false);
    } else setError(errorMessage(res.error));
  }

  const current = queue?.started ? patients.find((a) => a.serial_no === queue.now_serving) : undefined;
  const last = !!queue?.started && queue.waiting === 0;

  return (
    <Card className="border-teal-200 bg-teal-50/40">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p className="text-xs font-medium uppercase tracking-wide text-teal-800">
            <T>Live serial</T> · {session.label}, {clock(session.start)} – {clock(session.end)}
          </p>
          {queue === null ? (
            <p className="mt-1 text-sm text-slate-600"><T>Loading…</T></p>
          ) : queue.started ? (
            <>
              <p className="mt-1 text-2xl font-semibold text-slate-900">
                {t("Now seeing serial {n}", { n: queue.now_serving ?? 0 })}
              </p>
              {current && <p className="text-sm text-slate-700">{current.patient_name}</p>}
              <p className="text-sm text-slate-600">
                {t("{n} waiting", { n: queue.waiting ?? 0 })} · {t("about {m} min per patient", { m: queue.minutes_per_patient ?? 0 })}
              </p>
            </>
          ) : (
            <p className="mt-1 text-sm text-slate-700">
              {t("Not started. {n} waiting.", { n: queue.waiting ?? 0 })}{" "}
              <T>Patients see their place in line on their phone and get an SMS when their turn is near.</T>
            </p>
          )}
        </div>
        <div className="flex flex-wrap gap-2">
          <Button onClick={next} disabled={busy || queue === null || last}>
            {queue?.started ? "Next patient" : "Start the session"}
          </Button>
          <Button variant="secondary" onClick={() => setEditing(!editing)} disabled={queue === null}>
            Set serial
          </Button>
        </div>
      </div>
      {last && <p className="mt-2 text-sm text-slate-600"><T>No one else is waiting.</T></p>}
      {editing && (
        <form onSubmit={set} className="mt-3 flex flex-wrap items-end gap-2 border-t border-teal-100 pt-3">
          <label className="text-sm">
            <span className="mb-1 block font-medium text-slate-700"><T>Serial being seen now</T></span>
            <input
              name="serial"
              type="number"
              min={1}
              required
              defaultValue={queue?.now_serving || ""}
              className="w-28 rounded-lg border border-slate-300 bg-surface px-3 py-2 outline-none focus:border-teal-600"
            />
          </label>
          <Button type="submit" disabled={busy}>
            Save
          </Button>
        </form>
      )}
      {error && <div className="mt-3"><Alert>{error}</Alert></div>}
    </Card>
  );
}

// The patient's view: where their booking is in today's queue, refreshed every
// 20 seconds while the page is open.
export function LiveTurn({ appointmentID }: { appointmentID: string }) {
  const t = useT();
  const [turn, setTurn] = useState<Turn | null>(null);

  const load = useCallback(async () => {
    const { data } = await api.GET("/api/v1/appointments/{appointmentID}/queue", { params: { path: { appointmentID } } });
    if (data) setTurn(data);
  }, [appointmentID]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load();
  }, [load]);
  usePolling(load, 20_000);

  if (!turn || turn.status === "closed" || !turn.queue) return null;
  const q = turn.queue;

  if (turn.status === "your_turn") {
    return (
      <div className="mt-3 rounded-lg bg-teal-600 px-4 py-3 text-white" role="status">
        <p className="text-lg font-semibold"><T>It&apos;s your turn. Please go in.</T></p>
        <p className="text-sm text-teal-50">{t("Now seeing serial {n}", { n: q.now_serving ?? 0 })}</p>
      </div>
    );
  }
  if (turn.status === "passed") {
    return (
      <div className="mt-3 rounded-lg bg-amber-50 px-4 py-3 text-amber-900" role="status">
        <p className="font-semibold">{t("Now seeing serial {n}", { n: q.now_serving ?? 0 })}</p>
        <p className="text-sm"><T>Your serial has been called. If you are at the chamber, tell the reception.</T></p>
      </div>
    );
  }

  const ahead = turn.ahead ?? 0;
  return (
    <div className="mt-3 rounded-lg border border-teal-200 bg-teal-50 px-4 py-3" role="status">
      <p className="flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-teal-800">
        <span className="h-2 w-2 animate-pulse rounded-full bg-teal-600" aria-hidden /> <T>Live serial</T>
      </p>
      {q.started ? (
        <>
          <p className="mt-1 text-lg font-semibold text-slate-900">{t("Now seeing serial {n}", { n: q.now_serving ?? 0 })}</p>
          <p className="text-sm text-slate-700">
            {ahead === 0 ? t("You are next.") : t("{n} patients before you.", { n: ahead })}{" "}
            {turn.estimated_minutes != null && t("About {m} minutes.", { m: turn.estimated_minutes })}
          </p>
        </>
      ) : (
        <>
          <p className="mt-1 font-semibold text-slate-900"><T>The doctor has not started yet.</T></p>
          <p className="text-sm text-slate-700">
            {ahead === 0 ? t("You are first in line.") : t("{n} patients before you.", { n: ahead })}{" "}
            {t("The session starts at {time}.", { time: clock(q.start_time) })}
          </p>
        </>
      )}
      <p className="mt-1 text-xs text-slate-500"><T>We will also send you an SMS when your turn is near.</T></p>
    </div>
  );
}
