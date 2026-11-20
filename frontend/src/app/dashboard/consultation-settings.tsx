"use client";

import { useEffect, useState, type FormEvent } from "react";
import { T } from "@/components/t";
import { Alert, Button, Card, Spinner } from "@/components/ui";
import { api, errorMessage } from "@/lib/api/client";

type Settings = { enabled: boolean; fee: number; verified: boolean };

export function ConsultationSettings() {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.GET("/api/v1/doctor/profile").then(({ data, error }) => {
      if (!data) {
        setError(errorMessage(error));
        return;
      }
      setSettings({
        enabled: !!data.consultation_enabled,
        fee: data.consultation_fee_tokens || 1,
        verified: data.verification_status === "approved",
      });
    });
  }, []);

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!settings) return;
    const form = new FormData(e.currentTarget);
    const next = { enabled: form.get("enabled") === "on", fee_tokens: Number(form.get("fee")) };
    setBusy(true);
    setError("");
    setSaved(false);
    const { data, error } = await api.PUT("/api/v1/doctor/consultation-settings", { body: next });
    setBusy(false);
    if (!data) {
      setError(errorMessage(error));
      return;
    }
    setSettings({ ...settings, enabled: !!data.consultation_enabled, fee: data.consultation_fee_tokens || next.fee_tokens });
    setSaved(true);
  }

  return (
    <Card>
      <h2 className="font-semibold text-slate-900">
        <T>Online consultations</T>
      </h2>
      {!settings ? (
        error ? <Alert>{error}</Alert> : <Spinner />
      ) : (
        <form onSubmit={onSubmit} className="mt-3 space-y-3">
          {!settings.verified && (
            <Alert kind="info">Patients can find you once an admin verifies your BMDC registration.</Alert>
          )}
          <label className="flex items-center gap-2 text-sm text-slate-700">
            <input type="checkbox" name="enabled" defaultChecked={settings.enabled} className="accent-teal-700" />
            <T>Accept online consultations</T>
          </label>
          <label className="flex items-center gap-2 text-sm text-slate-700">
            Fee
            <input
              type="number"
              name="fee"
              min={1}
              max={20}
              defaultValue={settings.fee}
              className="w-20 rounded-lg border border-slate-300 px-2 py-1"
            />
            tokens (1 token = ৳100)
          </label>
          {error && <Alert>{error}</Alert>}
          {saved && <Alert kind="success">Saved.</Alert>}
          <Button type="submit" disabled={busy}>
            {busy ? "Saving…" : "Save"}
          </Button>
        </form>
      )}
    </Card>
  );
}
