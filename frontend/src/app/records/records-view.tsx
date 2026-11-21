"use client";

import Link from "next/link";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { RELATIONS, useFamily } from "@/components/family";
import { fileHref, KIND_ICON, RECORD_KINDS } from "@/components/records";
import { T } from "@/components/t";
import { TextArea } from "@/components/textarea";
import { Alert, Badge, Button, Card, Field, Select, Spinner } from "@/components/ui";
import { useT } from "@/components/use-lang";
import { api, errorMessage, type FamilyMember, type HealthRecord } from "@/lib/api/client";
import { fileSize, prettyDate, todayInDhaka } from "@/lib/format";

const MAX_FILE_BYTES = 10 * 1024 * 1024;
type Kind = "lab_report" | "imaging" | "prescription" | "discharge" | "vaccination" | "other";

export function RecordsView({ me }: { me: string }) {
  const t = useT();
  const family = useFamily();
  const [records, setRecords] = useState<HealthRecord[] | null>(null);
  const [person, setPerson] = useState("");
  const [error, setError] = useState("");
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);

  useEffect(() => {
    api.GET("/api/v1/records").then(({ data, error }) => {
      if (data) setRecords(data.records ?? []);
      else setError(errorMessage(error));
    });
  }, []);

  async function remove(r: HealthRecord) {
    if (!r.id || !confirm(t("Delete “{title}”? Doctors you shared it with will no longer see it.", { title: r.title ?? "" }))) return;
    const { error, response } = await api.DELETE("/api/v1/records/{recordID}", { params: { path: { recordID: r.id } } });
    if (!response.ok) {
      setError(errorMessage(error));
      return;
    }
    setRecords((prev) => (prev ?? []).filter((x) => x.id !== r.id));
  }

  function saved(r: HealthRecord) {
    setRecords((prev) => {
      const list = prev ?? [];
      return list.some((x) => x.id === r.id) ? list.map((x) => (x.id === r.id ? r : x)) : [r, ...list];
    });
    setAdding(false);
    setEditing(null);
  }

  if (!records || !family) return error ? <Alert>{error}</Alert> : <Spinner />;

  const shown = records.filter((r) => (person === "" ? true : person === "self" ? !r.family_member_id : r.family_member_id === person));
  const people = [{ id: "", label: t("Everyone") }, { id: "self", label: me }, ...family.map((m) => ({ id: m.id ?? "", label: m.name ?? "" }))];

  return (
    <div className="space-y-4">
      {error && <Alert>{error}</Alert>}

      {adding ? (
        <RecordForm me={me} family={family} defaultPerson={person === "self" ? "" : person} onSaved={saved} onCancel={() => setAdding(false)} />
      ) : (
        <Button onClick={() => setAdding(true)}>+ Upload a record</Button>
      )}

      {people.length > 2 && (
        <div className="flex flex-wrap gap-2" role="group" aria-label={t("Whose records")}>
          {people.map((p) => (
            <button
              key={p.id}
              onClick={() => setPerson(p.id)}
              aria-pressed={person === p.id}
              className={`rounded-full border px-3 py-1 text-sm ${
                person === p.id ? "border-teal-700 bg-teal-700 text-white" : "border-slate-300 bg-surface hover:border-teal-600"
              }`}
            >
              {p.label}
            </button>
          ))}
        </div>
      )}

      {shown.length === 0 && (
        <Card>
          <p className="text-sm text-slate-700">
            <T>No records here yet. Upload a report, an X-ray or an old prescription (PDF, JPEG or PNG, up to 10 MB).</T>
          </p>
        </Card>
      )}

      <ul className="space-y-3">
        {shown.map((r) =>
          editing === r.id ? (
            <li key={r.id}>
              <RecordForm me={me} family={family} record={r} onSaved={saved} onCancel={() => setEditing(null)} />
            </li>
          ) : (
            <li key={r.id}>
              <Card className="p-4">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="font-semibold text-slate-900">
                      <span aria-hidden>{KIND_ICON[r.kind ?? "other"]}</span> {r.title}
                    </p>
                    <p className="text-sm text-slate-600">
                      {[t(RECORD_KINDS[r.kind ?? "other"] ?? ""), r.record_date ? prettyDate(r.record_date) : null].filter(Boolean).join(" · ")}
                    </p>
                    <p className="text-xs text-slate-500">
                      {r.person_name} · {r.file_name} · {fileSize(r.size_bytes)}
                    </p>
                    {r.note && <p className="mt-1 whitespace-pre-line text-sm text-slate-700">{r.note}</p>}
                  </div>
                  <a
                    href={fileHref(r.id)}
                    className="inline-flex items-center justify-center rounded-lg border border-slate-300 bg-surface px-4 py-2 text-sm font-medium text-slate-800 hover:border-teal-600"
                  >
                    <T>Download</T>
                  </a>
                </div>
                {(r.shared_with ?? []).length > 0 && (
                  <div className="mt-2 flex flex-wrap gap-1">
                    {(r.shared_with ?? []).map((s) => (
                      <Badge key={s.appointment_id} tone="teal">
                        <T vars={{ doctor: s.doctor_name ?? "", date: prettyDate(s.date) }}>{"Shared with {doctor} ({date})"}</T>
                      </Badge>
                    ))}
                  </div>
                )}
                <div className="mt-3 flex flex-wrap gap-2 border-t border-slate-100 pt-3">
                  <Button variant="secondary" onClick={() => setEditing(r.id ?? null)}>
                    Edit
                  </Button>
                  <Button variant="danger" onClick={() => remove(r)}>
                    Delete
                  </Button>
                </div>
              </Card>
            </li>
          ),
        )}
      </ul>

      <p className="text-xs text-slate-500">
        <T>To show a record to a doctor, open</T>{" "}
        <Link href="/appointments" className="text-teal-800 hover:underline">
          <T>My appointments</T>
        </Link>{" "}
        <T>or an active online consultation.</T>
      </p>
    </div>
  );
}

function RecordForm({
  me,
  family,
  record,
  defaultPerson = "",
  onSaved,
  onCancel,
}: {
  me: string;
  family: FamilyMember[];
  record?: HealthRecord;
  defaultPerson?: string;
  onSaved: (r: HealthRecord) => void;
  onCancel: () => void;
}) {
  const t = useT();
  const fileInput = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const meta = {
      title: String(f.get("title") ?? "").trim(),
      kind: String(f.get("kind") ?? "other") as Kind,
      record_date: String(f.get("record_date") ?? ""),
      note: String(f.get("note") ?? "").trim(),
      family_member_id: String(f.get("family_member_id") ?? ""),
    };
    setError("");

    if (record?.id) {
      setBusy(true);
      const { data, error } = await api.PUT("/api/v1/records/{recordID}", { params: { path: { recordID: record.id } }, body: meta });
      setBusy(false);
      if (data) onSaved(data);
      else setError(errorMessage(error));
      return;
    }

    const file = fileInput.current?.files?.[0];
    if (!file) {
      setError(t("Choose a file to upload."));
      return;
    }
    if (file.size > MAX_FILE_BYTES) {
      setError(t("Files can be at most 10 MB."));
      return;
    }
    // The API reads the text fields first, so the file goes last.
    const body = new FormData();
    for (const [k, v] of Object.entries(meta)) if (v) body.append(k, v);
    body.append("file", file);
    setBusy(true);
    const res = await fetch("/api/v1/records", { method: "POST", body });
    setBusy(false);
    const data = await res.json().catch(() => null);
    if (res.ok && data) onSaved(data as HealthRecord);
    else setError(errorMessage(data, t("Could not upload the file.")));
  }

  return (
    <Card>
      <form onSubmit={submit} className="space-y-3">
        <h2 className="font-semibold text-slate-900">
          <T>{record ? "Edit record" : "Upload a record"}</T>
        </h2>
        {!record && (
          <label className="block">
            <span className="mb-1 block text-sm font-medium text-slate-700">
              <T>File (PDF, JPEG or PNG, up to 10 MB)</T>
            </span>
            <input
              ref={fileInput}
              type="file"
              accept="application/pdf,image/jpeg,image/png"
              required
              className="block w-full text-sm text-slate-700 file:mr-3 file:rounded-lg file:border-0 file:bg-teal-50 file:px-3 file:py-2 file:text-teal-800"
            />
          </label>
        )}
        <Field label="Title" name="title" required minLength={2} maxLength={150} defaultValue={record?.title} placeholder={t("e.g. CBC report")} />
        <div className="grid gap-3 sm:grid-cols-3">
          <Select label="Type" name="kind" defaultValue={record?.kind ?? "lab_report"}>
            {Object.entries(RECORD_KINDS).map(([v, label]) => (
              <option key={v} value={v}>
                {t(label)}
              </option>
            ))}
          </Select>
          <Field label="Date of the report" name="record_date" type="date" max={todayInDhaka()} defaultValue={record?.record_date ?? ""} />
          <Select label="Whose record" name="family_member_id" defaultValue={record ? (record.family_member_id ?? "") : defaultPerson}>
            <option value="">{t("Myself ({name})", { name: me })}</option>
            {family.map((m) => (
              <option key={m.id} value={m.id}>
                {m.name} ({t(RELATIONS[m.relation ?? ""] ?? "")})
              </option>
            ))}
          </Select>
        </div>
        <TextArea label="Note (optional)" name="note" rows={2} maxLength={2000} defaultValue={record?.note ?? ""} />
        {error && <Alert>{error}</Alert>}
        <div className="flex flex-wrap gap-2">
          <Button type="submit" disabled={busy}>
            {busy ? (record ? "Saving…" : "Uploading…") : record ? "Save" : "Upload"}
          </Button>
          <Button type="button" variant="secondary" onClick={onCancel} disabled={busy}>
            Cancel
          </Button>
        </div>
      </form>
    </Card>
  );
}
