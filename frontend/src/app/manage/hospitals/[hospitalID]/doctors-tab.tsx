"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Alert, Badge, Button, Card, Field, Select, Spinner } from "@/components/ui";
import { api, errorMessage, type Chamber, type Hospital, type StaffDoctor } from "@/lib/api/client";
import { clock, taka, WEEKDAYS } from "@/lib/format";

const STATUS = {
  invited: { label: "Invitation sent", tone: "amber" },
  active: { label: "On staff", tone: "teal" },
  declined: { label: "Declined", tone: "slate" },
  removed: { label: "Removed", tone: "slate" },
  left: { label: "Left", tone: "slate" },
} as const;

const input =
  "rounded-lg border border-slate-300 bg-white px-2 py-1.5 text-sm outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-600/20";

export function DoctorsTab({ hospital }: { hospital: Hospital }) {
  const hospitalID = hospital.id ?? "";
  const [doctors, setDoctors] = useState<StaffDoctor[] | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [inviting, setInviting] = useState(false);

  const reload = useCallback(async () => {
    const { data, error } = await api.GET("/api/v1/manage/hospitals/{hospitalID}/doctors", {
      params: { path: { hospitalID } },
    });
    if (data) setDoctors(data.doctors ?? []);
    else setError(errorMessage(error));
  }, [hospitalID]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    reload();
  }, [reload]);

  async function invite(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const text = (k: string) => String(f.get(k) ?? "").trim() || undefined;
    setInviting(true);
    setError("");
    const { data, error } = await api.POST("/api/v1/manage/hospitals/{hospitalID}/doctors", {
      params: { path: { hospitalID } },
      body: { bmdc_reg_no: String(f.get("bmdc")).trim(), department: text("department"), designation: text("designation") },
    });
    setInviting(false);
    if (!data) {
      setError(errorMessage(error));
      return;
    }
    (e.target as HTMLFormElement).reset();
    setNotice(`Invitation sent to ${data.name}. They join once they accept it.`);
    reload();
  }

  if (!doctors) return error ? <Alert>{error}</Alert> : <Spinner />;

  const current = doctors.filter((d) => d.status === "active" || d.status === "invited");
  const past = doctors.filter((d) => d.status !== "active" && d.status !== "invited");

  return (
    <div className="space-y-6">
      <Card>
        <h2 className="font-semibold text-slate-900">Invite a doctor</h2>
        <p className="mb-3 text-sm text-slate-600">
          Use the doctor&apos;s BMDC registration number. Only verified doctors with an account can be invited.
        </p>
        <form onSubmit={invite} className="grid gap-3 sm:grid-cols-[1fr_1fr_1fr_auto] sm:items-end">
          <Field label="BMDC number" name="bmdc" required placeholder="A-12345" />
          <Field label="Department" name="department" maxLength={60} placeholder="Cardiology" />
          <Field label="Designation" name="designation" maxLength={80} placeholder="Consultant" />
          <Button type="submit" disabled={inviting || hospital.verification_status !== "approved"}>
            {inviting ? "Inviting…" : "Invite"}
          </Button>
        </form>
      </Card>

      {notice && <Alert kind="success">{notice}</Alert>}
      {error && <Alert>{error}</Alert>}

      {current.length === 0 && <p className="text-sm text-slate-600">No doctors yet.</p>}
      {current.map((d) => (
        <DoctorCard
          key={d.doctor_id}
          hospitalID={hospitalID}
          doctor={d}
          onChange={reload}
          onError={setError}
          onNotice={setNotice}
        />
      ))}

      {past.length > 0 && (
        <details className="text-sm">
          <summary className="cursor-pointer text-slate-600">Former and declined ({past.length})</summary>
          <ul className="mt-2 space-y-1">
            {past.map((d) => (
              <li key={d.doctor_id} className="text-slate-600">
                {d.name} · {STATUS[d.status ?? "removed"].label}
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  );
}

function DoctorCard({
  hospitalID,
  doctor: d,
  onChange,
  onError,
  onNotice,
}: {
  hospitalID: string;
  doctor: StaffDoctor;
  onChange: () => void;
  onError: (m: string) => void;
  onNotice: (m: string) => void;
}) {
  const doctorID = d.doctor_id ?? "";
  const [editing, setEditing] = useState(false);
  const [opening, setOpening] = useState(false);
  const s = STATUS[d.status ?? "invited"];

  async function saveRole(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const { error } = await api.PUT("/api/v1/manage/hospitals/{hospitalID}/doctors/{doctorID}", {
      params: { path: { hospitalID, doctorID } },
      body: { department: String(f.get("department")).trim(), designation: String(f.get("designation")).trim() },
    });
    if (error) onError(errorMessage(error));
    else {
      setEditing(false);
      onChange();
    }
  }

  async function remove() {
    const what = d.status === "invited" ? "Withdraw the invitation" : `Remove ${d.name} from the hospital`;
    if (!confirm(`${what}? Booking stops for their chambers here.`)) return;
    const { data, error } = await api.DELETE("/api/v1/manage/hospitals/{hospitalID}/doctors/{doctorID}", {
      params: { path: { hospitalID, doctorID } },
    });
    if (!data) {
      onError(errorMessage(error));
      return;
    }
    if (data.upcoming_appointments) {
      onNotice(
        `${d.name} removed. ${data.upcoming_appointments} upcoming booking(s) remain — cancel or reschedule them in the Bookings tab.`,
      );
    }
    onChange();
  }

  async function openChamber(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const { error } = await api.POST("/api/v1/manage/hospitals/{hospitalID}/doctors/{doctorID}/chambers", {
      params: { path: { hospitalID, doctorID } },
      body: {
        fee_paisa: Math.round(Number(f.get("fee")) * 100),
        room: String(f.get("room")).trim() || undefined,
        booking_enabled: true,
      },
    });
    if (error) onError(errorMessage(error));
    else {
      setOpening(false);
      onChange();
    }
  }

  return (
    <Card>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <p className="font-semibold text-slate-900">{d.name}</p>
            <Badge tone={s.tone}>{s.label}</Badge>
          </div>
          <p className="text-sm text-slate-600">
            {[d.designation, d.department].filter(Boolean).join(", ") || "No department set"}
            {" · "}
            {[d.specialization, d.degrees].filter(Boolean).join(", ")}
          </p>
          <p className="text-xs text-slate-500">BMDC {d.bmdc_reg_no}</p>
        </div>
        <div className="flex gap-2">
          <Button variant="secondary" onClick={() => setEditing((x) => !x)}>
            Edit role
          </Button>
          <Button variant="danger" onClick={remove}>
            {d.status === "invited" ? "Withdraw" : "Remove"}
          </Button>
        </div>
      </div>

      {editing && (
        <form onSubmit={saveRole} className="mt-3 grid gap-2 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
          <Field label="Department" name="department" defaultValue={d.department ?? ""} maxLength={60} />
          <Field label="Designation" name="designation" defaultValue={d.designation ?? ""} maxLength={80} />
          <Button type="submit">Save</Button>
        </form>
      )}

      {d.status === "active" && (
        <div className="mt-4 space-y-3 border-t border-slate-100 pt-4">
          {(d.chambers ?? []).map((c) => (
            <ChamberEditor key={c.id} hospitalID={hospitalID} chamber={c} onChange={onChange} onError={onError} />
          ))}
          {opening ? (
            <form onSubmit={openChamber} className="grid gap-2 sm:grid-cols-[10rem_1fr_auto_auto] sm:items-end">
              <Field label="Fee (৳)" name="fee" type="number" min={0} step={1} required />
              <Field label="Room" name="room" placeholder="Room 412, 4th floor" />
              <Button type="submit">Open chamber</Button>
              <Button type="button" variant="secondary" onClick={() => setOpening(false)}>
                Cancel
              </Button>
            </form>
          ) : (
            (d.chambers ?? []).length === 0 && (
              <Button variant="secondary" onClick={() => setOpening(true)}>
                + Open a chamber for {d.name}
              </Button>
            )
          )}
        </div>
      )}
      {d.status === "invited" && (
        <p className="mt-3 text-sm text-slate-500">Chambers can be set up once the doctor accepts the invitation.</p>
      )}
    </Card>
  );
}

function ChamberEditor({
  hospitalID,
  chamber: c,
  onChange,
  onError,
}: {
  hospitalID: string;
  chamber: Chamber;
  onChange: () => void;
  onError: (m: string) => void;
}) {
  const chamberID = c.id ?? "";
  const [editing, setEditing] = useState(false);
  const [adding, setAdding] = useState(false);

  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const { error } = await api.PUT("/api/v1/manage/hospitals/{hospitalID}/chambers/{chamberID}", {
      params: { path: { hospitalID, chamberID } },
      body: {
        fee_paisa: Math.round(Number(f.get("fee")) * 100),
        room: String(f.get("room")).trim() || undefined,
        booking_enabled: f.get("booking") === "on",
      },
    });
    if (error) onError(errorMessage(error));
    else {
      setEditing(false);
      onChange();
    }
  }

  async function addSession(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const { error } = await api.POST("/api/v1/manage/hospitals/{hospitalID}/chambers/{chamberID}/schedules", {
      params: { path: { hospitalID, chamberID } },
      body: {
        weekday: Number(f.get("weekday")),
        start_time: String(f.get("start")),
        end_time: String(f.get("end")),
        max_patients: Number(f.get("max")),
      },
    });
    if (error) onError(errorMessage(error));
    else {
      setAdding(false);
      onChange();
    }
  }

  async function removeSession(scheduleID: string) {
    if (!confirm("Remove this weekly session?")) return;
    const { error } = await api.DELETE("/api/v1/manage/hospitals/{hospitalID}/schedules/{scheduleID}", {
      params: { path: { hospitalID, scheduleID } },
    });
    if (error) onError(errorMessage(error));
    else onChange();
  }

  return (
    <div className="rounded-lg border border-slate-200 p-3">
      <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
        <span>
          <span className="font-medium text-slate-900">{c.room || "Chamber"}</span> · Fee {taka(c.fee_paisa)}
          {c.booking_enabled === false && (
            <>
              {" "}
              <Badge tone="amber">Booking off</Badge>
            </>
          )}
        </span>
        <button onClick={() => setEditing((x) => !x)} className="text-teal-800 hover:underline">
          Edit chamber
        </button>
      </div>

      {editing && (
        <form onSubmit={save} className="mt-3 grid gap-2 sm:grid-cols-[8rem_1fr_auto_auto] sm:items-end">
          <Field label="Fee (৳)" name="fee" type="number" min={0} step={1} defaultValue={(c.fee_paisa ?? 0) / 100} required />
          <Field label="Room" name="room" defaultValue={c.room ?? ""} />
          <label className="flex items-center gap-2 pb-2 text-sm text-slate-700">
            <input type="checkbox" name="booking" defaultChecked={c.booking_enabled !== false} className="accent-teal-700" />
            Booking open
          </label>
          <Button type="submit">Save</Button>
        </form>
      )}

      <ul className="mt-3 space-y-1 text-sm">
        {(c.schedules ?? []).map((s) => (
          <li key={s.id} className="flex items-center justify-between gap-2 rounded bg-slate-50 px-2 py-1">
            <span>
              {WEEKDAYS[s.weekday ?? 0]} · {clock(s.start_time)} – {clock(s.end_time)} · up to {s.max_patients}
            </span>
            <button onClick={() => s.id && removeSession(s.id)} className="text-xs text-red-700 hover:underline">
              Remove
            </button>
          </li>
        ))}
      </ul>

      {adding ? (
        <form onSubmit={addSession} className="mt-3 grid gap-2 sm:grid-cols-[1fr_auto_auto_6rem_auto_auto] sm:items-end">
          <Select label="Day" name="weekday" defaultValue="0">
            {WEEKDAYS.map((w, i) => (
              <option key={w} value={i}>
                {w}
              </option>
            ))}
          </Select>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-slate-700">From</span>
            <input type="time" name="start" required defaultValue="17:00" className={input} />
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium text-slate-700">To</span>
            <input type="time" name="end" required defaultValue="21:00" className={input} />
          </label>
          <Field label="Patients" name="max" type="number" min={1} max={500} defaultValue={20} required />
          <Button type="submit">Add</Button>
          <Button type="button" variant="secondary" onClick={() => setAdding(false)}>
            Cancel
          </Button>
        </form>
      ) : (
        <button onClick={() => setAdding(true)} className="mt-2 text-sm text-teal-800 hover:underline">
          + Add weekly session
        </button>
      )}
    </div>
  );
}
