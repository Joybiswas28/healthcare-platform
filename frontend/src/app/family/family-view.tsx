"use client";

import { useEffect, useState, type FormEvent } from "react";
import { ForMember, forgetFamily, RELATIONS } from "@/components/family";
import { T } from "@/components/t";
import { Alert, Button, Card, Field, Select, Spinner } from "@/components/ui";
import { useT } from "@/components/use-lang";
import { api, errorMessage, type FamilyMember } from "@/lib/api/client";
import { todayInDhaka } from "@/lib/format";

type Gender = "" | "male" | "female" | "other";
type Relation = "spouse" | "child" | "parent" | "sibling" | "other";

export function FamilyView() {
  const t = useT();
  const [members, setMembers] = useState<FamilyMember[] | null>(null);
  const [max, setMax] = useState(10);
  const [error, setError] = useState("");
  // null: no form open; "": adding; an id: editing that member
  const [editing, setEditing] = useState<string | null>(null);

  async function load() {
    const { data, error } = await api.GET("/api/v1/family");
    if (!data) {
      setError(errorMessage(error));
      return;
    }
    setMembers(data.members ?? []);
    setMax(data.max ?? 10);
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load();
  }, []);

  async function remove(m: FamilyMember) {
    if (!m.id || !confirm(t("Remove {name}? Their past appointments and prescriptions stay.", { name: m.name ?? "" }))) return;
    const { error, response } = await api.DELETE("/api/v1/family/{memberID}", { params: { path: { memberID: m.id } } });
    if (!response.ok) {
      setError(errorMessage(error));
      return;
    }
    forgetFamily();
    setMembers((prev) => (prev ?? []).filter((x) => x.id !== m.id));
  }

  function saved(m: FamilyMember) {
    forgetFamily();
    setMembers((prev) => {
      const list = prev ?? [];
      return list.some((x) => x.id === m.id) ? list.map((x) => (x.id === m.id ? m : x)) : [...list, m];
    });
    setEditing(null);
  }

  if (!members) return error ? <Alert>{error}</Alert> : <Spinner />;

  return (
    <div className="space-y-3">
      {error && <Alert>{error}</Alert>}
      {members.length === 0 && editing === null && (
        <Card>
          <p className="text-sm text-slate-700">
            <T>No family members yet.</T>
          </p>
        </Card>
      )}
      {members.map((m) =>
        editing === m.id ? (
          <MemberForm key={m.id} member={m} onSaved={saved} onCancel={() => setEditing(null)} />
        ) : (
          <Card key={m.id} className="p-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <p className="font-semibold text-slate-900">{m.name}</p>
                <ForMember relation={m.relation} birthDate={m.birth_date} />
              </div>
              <div className="flex gap-2">
                <Button variant="secondary" onClick={() => setEditing(m.id ?? null)}>
                  Edit
                </Button>
                <Button variant="danger" onClick={() => remove(m)}>
                  Remove
                </Button>
              </div>
            </div>
          </Card>
        ),
      )}

      {editing === "" ? (
        <MemberForm onSaved={saved} onCancel={() => setEditing(null)} />
      ) : (
        editing === null &&
        (members.length < max ? (
          <Button onClick={() => setEditing("")}>+ Add family member</Button>
        ) : (
          <p className="text-sm text-slate-600">
            <T vars={{ n: max }}>{"You can keep up to {n} family members."}</T>
          </p>
        ))
      )}
    </div>
  );
}

function MemberForm({ member, onSaved, onCancel }: { member?: FamilyMember; onSaved: (m: FamilyMember) => void; onCancel: () => void }) {
  const t = useT();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const body = {
      name: String(f.get("name") ?? "").trim(),
      relation: String(f.get("relation")) as Relation,
      gender: String(f.get("gender") ?? "") as Gender,
      birth_date: String(f.get("birth_date") ?? ""),
    };
    setBusy(true);
    setError("");
    const res = member?.id
      ? await api.PUT("/api/v1/family/{memberID}", { params: { path: { memberID: member.id } }, body })
      : await api.POST("/api/v1/family", { body });
    setBusy(false);
    if (!res.data) {
      setError(errorMessage(res.error));
      return;
    }
    onSaved(res.data);
  }

  return (
    <Card>
      <form onSubmit={submit} className="space-y-3">
        <h2 className="font-semibold text-slate-900">
          <T>{member ? "Edit family member" : "Add family member"}</T>
        </h2>
        <Field label="Full name" name="name" required minLength={2} maxLength={100} defaultValue={member?.name} autoComplete="off" />
        <div className="grid gap-3 sm:grid-cols-3">
          <Select label="Relation" name="relation" defaultValue={member?.relation ?? "child"}>
            {Object.entries(RELATIONS).map(([v, label]) => (
              <option key={v} value={v}>
                {t(label)}
              </option>
            ))}
          </Select>
          <Select label="Gender" name="gender" defaultValue={member?.gender ?? ""}>
            <option value="">{t("Not given")}</option>
            <option value="male">{t("Male")}</option>
            <option value="female">{t("Female")}</option>
            <option value="other">{t("Other")}</option>
          </Select>
          <Field label="Date of birth" name="birth_date" type="date" max={todayInDhaka()} defaultValue={member?.birth_date ?? ""} />
        </div>
        <p className="text-xs text-slate-500">
          <T>Gender and age help the doctor; they are shown on bookings for this person.</T>
        </p>
        {error && <Alert>{error}</Alert>}
        <div className="flex flex-wrap gap-2">
          <Button type="submit" disabled={busy}>
            {busy ? "Saving…" : "Save"}
          </Button>
          <Button type="button" variant="secondary" onClick={onCancel} disabled={busy}>
            Cancel
          </Button>
        </div>
      </form>
    </Card>
  );
}
