"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useAuth } from "@/components/auth-provider";
import { Alert, Badge, Button, Card, Field, Spinner } from "@/components/ui";
import { api, errorMessage, type Hospital, type Member } from "@/lib/api/client";
import { T } from "@/components/t";
import { useT } from "@/components/use-lang";

export function MembersTab({ hospital }: { hospital: Hospital }) {
  const hospitalID = hospital.id ?? "";
  const isOwner = hospital.my_role === "owner";
  const { user } = useAuth();
  const t = useT();
  const [members, setMembers] = useState<Member[] | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const reload = useCallback(async () => {
    const { data, error } = await api.GET("/api/v1/manage/hospitals/{hospitalID}/members", { params: { path: { hospitalID } } });
    if (data) setMembers(data.members ?? []);
    else setError(errorMessage(error));
  }, [hospitalID]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    reload();
  }, [reload]);

  async function add(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    setBusy(true);
    setError("");
    const { error } = await api.POST("/api/v1/manage/hospitals/{hospitalID}/members", {
      params: { path: { hospitalID } },
      body: { phone: String(new FormData(form).get("phone")).trim() },
    });
    setBusy(false);
    if (error) setError(errorMessage(error));
    else {
      form.reset();
      reload();
    }
  }

  async function remove(m: Member) {
    if (!m.user_id || !confirm(t("Remove {name}? They lose access to this hospital.", { name: m.name }))) return;
    const { error } = await api.DELETE("/api/v1/manage/hospitals/{hospitalID}/members/{userID}", {
      params: { path: { hospitalID, userID: m.user_id } },
    });
    if (error) setError(errorMessage(error));
    else reload();
  }

  if (!members) return error ? <Alert>{error}</Alert> : <Spinner />;

  return (
    <div className="space-y-4">
      {isOwner && (
        <Card>
          <h2 className="font-semibold text-slate-900"><T>Add a staff account</T></h2>
          <p className="mb-3 text-sm text-slate-600">
            <T>The person first signs up as a &quot;Hospital administrator&quot;; then add their mobile number here.</T>
          </p>
          <form onSubmit={add} className="flex flex-wrap items-end gap-2">
            <div className="flex-1">
              <Field label="Mobile number" name="phone" type="tel" placeholder="01XXXXXXXXX" required />
            </div>
            <Button type="submit" disabled={busy}>
              {busy ? "Adding…" : "Add"}
            </Button>
          </form>
        </Card>
      )}
      {error && <Alert>{error}</Alert>}
      <Card className="p-0">
        <ul className="divide-y divide-slate-100">
          {members.map((m) => (
            <li key={m.user_id} className="flex items-center justify-between gap-3 px-5 py-3 text-sm">
              <span>
                <span className="font-medium text-slate-900">{m.name}</span>
                {m.user_id === user?.id && <span className="text-slate-500"> {t("(you)")}</span>}
                <span className="block text-slate-600">{m.phone}</span>
              </span>
              <span className="flex items-center gap-3">
                <Badge tone={m.role === "owner" ? "teal" : "slate"}>{m.role === "owner" ? "Owner" : "Staff"}</Badge>
                {isOwner && m.role !== "owner" && (
                  <button onClick={() => remove(m)} className="text-red-700 hover:underline">
                    <T>Remove</T>
                  </button>
                )}
              </span>
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
}
