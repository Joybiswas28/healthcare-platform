"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { ScheduleEditor, type ScheduleBody } from "@/components/schedule-editor";
import { Alert, Badge, Button, Card, Field, Spinner } from "@/components/ui";
import { api, errorMessage, type Chamber } from "@/lib/api/client";
import { clock, taka, WEEKDAYS } from "@/lib/format";
import { T } from "@/components/t";
import { useT } from "@/components/use-lang";

export function MyChambers() {
  const [chambers, setChambers] = useState<Chamber[] | null>(null);
  const [error, setError] = useState("");
  const [adding, setAdding] = useState(false);

  const reload = useCallback(async () => {
    const { data, error } = await api.GET("/api/v1/doctor/chambers");
    if (data) setChambers(data.chambers ?? []);
    else setError(errorMessage(error));
  }, []);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    reload();
  }, [reload]);

  if (!chambers) return error ? <Alert>{error}</Alert> : <Spinner />;

  const own = chambers.filter((c) => !c.hospital_id);
  const hospital = chambers.filter((c) => c.hospital_id);

  return (
    <div className="space-y-6">
      <p className="text-sm text-slate-600">
        <T>Patients book a serial in one of your weekly sessions. Sessions may not overlap, across all your chambers and hospitals.</T>
      </p>

      {adding ? (
        <Card>
          <h2 className="mb-3 font-semibold text-slate-900"><T>Add a private chamber</T></h2>
          <ChamberForm
            submitLabel="Add chamber"
            onSave={async (body) => {
              const { error } = await api.POST("/api/v1/doctor/chambers", { body });
              if (error) return errorMessage(error);
              setAdding(false);
              reload();
              return "";
            }}
            onCancel={() => setAdding(false)}
          />
        </Card>
      ) : (
        <Button onClick={() => setAdding(true)}>+ Add a private chamber</Button>
      )}

      {own.length === 0 && !adding && <p className="text-sm text-slate-600"><T>No private chambers yet.</T></p>}
      {own.map((c) => (
        <OwnChamber key={c.id} chamber={c} onChange={reload} />
      ))}

      {hospital.length > 0 && (
        <section className="space-y-3">
          <h2 className="text-lg font-semibold text-slate-900"><T>At hospitals</T></h2>
          <p className="text-sm text-slate-600"><T>The hospital sets the fee, room and sessions for these.</T></p>
          {hospital.map((c) => (
            <Card key={c.id}>
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <p className="font-semibold text-slate-900">{c.hospital_name}</p>
                  <p className="text-sm text-slate-600">{[c.room, c.address, c.district].filter(Boolean).join(" · ")}</p>
                </div>
                <span className="text-sm text-slate-700">
                  <T vars={{ fee: taka(c.fee_paisa) }}>{"Fee {fee}"}</T>
                </span>
              </div>
              <ul className="mt-3 space-y-1 text-sm text-slate-700">
                {(c.schedules ?? []).map((s) => (
                  <li key={s.id}>
                    <T>{WEEKDAYS[s.weekday ?? 0]}</T> · {clock(s.start_time)} – {clock(s.end_time)} ·{" "}
                    <T vars={{ n: s.max_patients ?? 0 }}>{"up to {n} patients"}</T>
                  </li>
                ))}
              </ul>
            </Card>
          ))}
        </section>
      )}
    </div>
  );
}

type ChamberBody = {
  name: string;
  address: string;
  district: string;
  area?: string;
  phone?: string;
  fee_paisa: number;
  booking_enabled: boolean;
};

function ChamberForm({
  initial,
  submitLabel,
  onSave,
  onCancel,
}: {
  initial?: Chamber;
  submitLabel: string;
  onSave: (body: ChamberBody) => Promise<string>;
  onCancel: () => void;
}) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const t = useT();

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const text = (k: string) => String(f.get(k) ?? "").trim();
    setBusy(true);
    setError("");
    const problem = await onSave({
      name: text("name"),
      address: text("address"),
      district: text("district"),
      area: text("area") || undefined,
      phone: text("phone") || undefined,
      fee_paisa: Math.round(Number(f.get("fee")) * 100),
      booking_enabled: f.get("booking") === "on",
    });
    setBusy(false);
    if (problem) setError(problem);
  }

  return (
    <form onSubmit={submit} className="space-y-3">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Chamber name" name="name" required defaultValue={initial?.name} placeholder={t("e.g. Popular Diagnostic, Dhanmondi")} />
        <Field label="Fee (৳)" name="fee" type="number" min={0} step={1} required defaultValue={initial ? (initial.fee_paisa ?? 0) / 100 : ""} />
        <Field label="Address" name="address" required defaultValue={initial?.address} />
        <Field label="Area" name="area" defaultValue={initial?.area ?? ""} />
        <Field label="District" name="district" required defaultValue={initial?.district} />
        <Field label="Chamber phone (optional)" name="phone" type="tel" defaultValue={initial?.phone ?? ""} />
      </div>
      <label className="flex items-center gap-2 text-sm text-slate-700">
        <input type="checkbox" name="booking" defaultChecked={initial?.booking_enabled !== false} className="accent-teal-700" />
        <T>Open for online booking</T>
      </label>
      {error && <Alert>{error}</Alert>}
      <div className="flex gap-2">
        <Button type="submit" disabled={busy}>
          {busy ? "Saving…" : submitLabel}
        </Button>
        <Button type="button" variant="secondary" onClick={onCancel}>
          <T>Cancel</T>
        </Button>
      </div>
    </form>
  );
}

function OwnChamber({ chamber: c, onChange }: { chamber: Chamber; onChange: () => void }) {
  const [editing, setEditing] = useState(false);
  const chamberID = c.id ?? "";

  return (
    <Card className="space-y-3">
      {editing ? (
        <ChamberForm
          initial={c}
          submitLabel="Save"
          onSave={async (body) => {
            const { error } = await api.PUT("/api/v1/doctor/chambers/{chamberID}", { params: { path: { chamberID } }, body });
            if (error) return errorMessage(error);
            setEditing(false);
            onChange();
            return "";
          }}
          onCancel={() => setEditing(false)}
        />
      ) : (
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <p className="font-semibold text-slate-900">{c.name}</p>
              {c.booking_enabled === false && <Badge tone="amber">Booking off</Badge>}
            </div>
            <p className="text-sm text-slate-600">
              {[c.address, c.area, c.district].filter(Boolean).join(", ")}
              {c.phone ? ` · ${c.phone}` : ""}
            </p>
          </div>
          <div className="flex items-center gap-3">
            <span className="text-sm text-slate-700">
                  <T vars={{ fee: taka(c.fee_paisa) }}>{"Fee {fee}"}</T>
                </span>
            <button onClick={() => setEditing(true)} className="text-sm text-teal-800 hover:underline">
              <T>Edit</T>
            </button>
          </div>
        </div>
      )}
      <div className="border-t border-slate-100 pt-3">
        <ScheduleEditor
          schedules={c.schedules ?? []}
          onAdd={async (body: ScheduleBody) => {
            const { error } = await api.POST("/api/v1/doctor/chambers/{chamberID}/schedules", { params: { path: { chamberID } }, body });
            if (error) return errorMessage(error);
            onChange();
            return "";
          }}
          onRemove={async (scheduleID) => {
            const { error } = await api.DELETE("/api/v1/doctor/schedules/{scheduleID}", { params: { path: { scheduleID } } });
            if (error) return errorMessage(error);
            onChange();
            return "";
          }}
        />
      </div>
    </Card>
  );
}
