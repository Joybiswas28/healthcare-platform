"use client";

import { useState, type FormEvent } from "react";
import type { Schedule } from "@/lib/api/client";
import { clock, WEEKDAYS } from "@/lib/format";
import { Alert, Button, Field, Select } from "./ui";

export type ScheduleBody = { weekday: number; start_time: string; end_time: string; max_patients: number };

const timeInput =
  "rounded-lg border border-slate-300 bg-white px-2 py-1.5 text-sm outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-600/20";

// Weekly sessions of one chamber with add and remove. The callbacks return an error message or "".
export function ScheduleEditor({
  schedules,
  onAdd,
  onRemove,
}: {
  schedules: Schedule[];
  onAdd: (body: ScheduleBody) => Promise<string>;
  onRemove: (scheduleID: string) => Promise<string>;
}) {
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function add(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const body = {
      weekday: Number(f.get("weekday")),
      start_time: String(f.get("start")),
      end_time: String(f.get("end")),
      max_patients: Number(f.get("max")),
    };
    if (body.end_time <= body.start_time) {
      setError("The session must end after it starts.");
      return;
    }
    setBusy(true);
    setError("");
    const problem = await onAdd(body);
    setBusy(false);
    if (problem) setError(problem);
    else setAdding(false);
  }

  async function remove(id: string) {
    if (!confirm("Remove this weekly session? Bookings already made stay valid.")) return;
    setError("");
    const problem = await onRemove(id);
    if (problem) setError(problem);
  }

  const sorted = [...schedules].sort((a, b) => (a.weekday ?? 0) - (b.weekday ?? 0) || (a.start_time ?? "").localeCompare(b.start_time ?? ""));

  return (
    <div className="space-y-2">
      {sorted.length === 0 && <p className="text-sm text-slate-500">No weekly sessions yet — patients can&apos;t book until you add one.</p>}
      <ul className="space-y-1 text-sm">
        {sorted.map((s) => (
          <li key={s.id} className="flex items-center justify-between gap-2 rounded bg-slate-50 px-2 py-1">
            <span>
              {WEEKDAYS[s.weekday ?? 0]} · {clock(s.start_time)} – {clock(s.end_time)} · up to {s.max_patients} patients
            </span>
            <button onClick={() => s.id && remove(s.id)} className="text-xs text-red-700 hover:underline">
              Remove
            </button>
          </li>
        ))}
      </ul>

      {adding ? (
        <form onSubmit={add} className="grid gap-2 sm:grid-cols-[1fr_auto_auto_6rem_auto_auto] sm:items-end">
          <Select label="Day" name="weekday" defaultValue="0">
            {WEEKDAYS.map((w, i) => (
              <option key={w} value={i}>
                {w}
              </option>
            ))}
          </Select>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-slate-700">From</span>
            <input type="time" name="start" required defaultValue="17:00" className={timeInput} />
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-slate-700">To</span>
            <input type="time" name="end" required defaultValue="21:00" className={timeInput} />
          </label>
          <Field label="Patients" name="max" type="number" min={1} max={500} defaultValue={20} required />
          <Button type="submit" disabled={busy}>
            Add
          </Button>
          <Button type="button" variant="secondary" onClick={() => setAdding(false)}>
            Cancel
          </Button>
        </form>
      ) : (
        <button onClick={() => setAdding(true)} className="text-sm text-teal-800 hover:underline">
          + Add weekly session
        </button>
      )}
      {error && <Alert>{error}</Alert>}
    </div>
  );
}
