"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { ScheduleEditor, type ScheduleBody } from "@/components/schedule-editor";
import { Alert, Badge, Button, Card, Field, Spinner } from "@/components/ui";
import { api, errorMessage, type Chamber, type Hospital, type StaffDoctor } from "@/lib/api/client";
import { taka } from "@/lib/format";
import { T } from "@/components/t";
import { useT } from "@/components/use-lang";

const STATUS = {
  invited: { label: "Invitation sent", tone: "amber" },
  active: { label: "On staff", tone: "teal" },
  declined: { label: "Declined", tone: "slate" },
  removed: { label: "Removed", tone: "slate" },
  left: { label: "Left", tone: "slate" },
} as const;

export function DoctorsTab({ hospital }: { hospital: Hospital }) {
  const hospitalID = hospital.id ?? "";
  const [doctors, setDoctors] = useState<StaffDoctor[] | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [inviting, setInviting] = useState(false);
  const t = useT();

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
    setNotice(t("Invitation sent to {name}. They join once they accept it.", { name: data.name }));
    reload();
  }

  if (!doctors) return error ? <Alert>{error}</Alert> : <Spinner />;

  const current = doctors.filter((d) => d.status === "active" || d.status === "invited");
  const past = doctors.filter((d) => d.status !== "active" && d.status !== "invited");

  return (
    <div className="space-y-6">
      <Card>
        <h2 className="font-semibold text-slate-900"><T>Invite a doctor</T></h2>
        <p className="mb-3 text-sm text-slate-600">
          <T>Use the doctor&apos;s BMDC registration number. Only verified doctors with an account can be invited.</T>
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

      {current.length === 0 && <p className="text-sm text-slate-600"><T>No doctors yet.</T></p>}
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
          <summary className="cursor-pointer text-slate-600">
            <T vars={{ n: past.length }}>{"Former and declined ({n})"}</T>
          </summary>
          <ul className="mt-2 space-y-1">
            {past.map((d) => (
              <li key={d.doctor_id} className="text-slate-600">
                {d.name} · <T>{STATUS[d.status ?? "removed"].label}</T>
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
  const t = useT();

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
    const question =
      d.status === "invited"
        ? t("Withdraw the invitation? Booking stops for their chambers here.")
        : t("Remove {name} from the hospital? Booking stops for their chambers here.", { name: d.name });
    if (!confirm(question)) return;
    const { data, error } = await api.DELETE("/api/v1/manage/hospitals/{hospitalID}/doctors/{doctorID}", {
      params: { path: { hospitalID, doctorID } },
    });
    if (!data) {
      onError(errorMessage(error));
      return;
    }
    if (data.upcoming_appointments) {
      onNotice(
        t("{name} removed. {n} upcoming booking(s) remain — cancel or reschedule them in the Bookings tab.", {
          name: d.name,
          n: data.upcoming_appointments,
        }),
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
            {[d.designation, d.department].filter(Boolean).join(", ") || <T>No department set</T>}
            {" · "}
            {[d.specialization, d.degrees].filter(Boolean).join(", ")}
          </p>
          <p className="text-xs text-slate-500">BMDC {d.bmdc_reg_no}</p>
        </div>
        <div className="flex gap-2">
          <Button variant="secondary" onClick={() => setEditing((x) => !x)}>
            <T>Edit role</T>
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
                <T>Cancel</T>
              </Button>
            </form>
          ) : (
            (d.chambers ?? []).length === 0 && (
              <Button variant="secondary" onClick={() => setOpening(true)}>
                + <T vars={{ name: d.name }}>{"Open a chamber for {name}"}</T>
              </Button>
            )
          )}
        </div>
      )}
      {d.status === "invited" && (
        <p className="mt-3 text-sm text-slate-500"><T>Chambers can be set up once the doctor accepts the invitation.</T></p>
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

  async function addSession(body: ScheduleBody) {
    const { error } = await api.POST("/api/v1/manage/hospitals/{hospitalID}/chambers/{chamberID}/schedules", {
      params: { path: { hospitalID, chamberID } },
      body,
    });
    if (error) return errorMessage(error);
    onChange();
    return "";
  }

  async function removeSession(scheduleID: string) {
    const { error } = await api.DELETE("/api/v1/manage/hospitals/{hospitalID}/schedules/{scheduleID}", {
      params: { path: { hospitalID, scheduleID } },
    });
    if (error) return errorMessage(error);
    onChange();
    return "";
  }

  return (
    <div className="rounded-lg border border-slate-200 p-3">
      <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
        <span>
          <span className="font-medium text-slate-900">{c.room || <T>Chamber</T>}</span> ·{" "}
          <T vars={{ fee: taka(c.fee_paisa) }}>{"Fee {fee}"}</T>
          {c.booking_enabled === false && (
            <>
              {" "}
              <Badge tone="amber">Booking off</Badge>
            </>
          )}
        </span>
        <button onClick={() => setEditing((x) => !x)} className="text-teal-800 hover:underline">
          <T>Edit chamber</T>
        </button>
      </div>

      {editing && (
        <form onSubmit={save} className="mt-3 grid gap-2 sm:grid-cols-[8rem_1fr_auto_auto] sm:items-end">
          <Field label="Fee (৳)" name="fee" type="number" min={0} step={1} defaultValue={(c.fee_paisa ?? 0) / 100} required />
          <Field label="Room" name="room" defaultValue={c.room ?? ""} />
          <label className="flex items-center gap-2 pb-2 text-sm text-slate-700">
            <input type="checkbox" name="booking" defaultChecked={c.booking_enabled !== false} className="accent-teal-700" />
            <T>Booking open</T>
          </label>
          <Button type="submit">Save</Button>
        </form>
      )}

      <div className="mt-3">
        <ScheduleEditor schedules={c.schedules ?? []} onAdd={addSession} onRemove={removeSession} />
      </div>
    </div>
  );
}
