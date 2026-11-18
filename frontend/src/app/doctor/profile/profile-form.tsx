"use client";

import Link from "next/link";
import { useEffect, useState, type FormEvent } from "react";
import { TextArea } from "@/components/textarea";
import { Alert, Badge, Button, Card, Field, Spinner } from "@/components/ui";
import { api, errorMessage, type Schemas } from "@/lib/api/client";

type Profile = Schemas["DoctorProfile"];

const STATUS = {
  pending: { label: "Waiting for verification", tone: "amber" },
  approved: { label: "BMDC verified", tone: "teal" },
  rejected: { label: "Verification rejected", tone: "red" },
} as const;

export function ProfileForm() {
  const [profile, setProfile] = useState<Profile | null>(null);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.GET("/api/v1/doctor/profile").then(({ data, error }) => {
      if (data) setProfile(data);
      else setError(errorMessage(error));
    });
  }, []);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const text = (k: string) => String(f.get(k) ?? "").trim();
    const years = text("experience_years");
    setBusy(true);
    setError("");
    setSaved(false);
    const { data, error } = await api.PUT("/api/v1/doctor/profile", {
      body: {
        bmdc_reg_no: text("bmdc_reg_no"),
        specialization: text("specialization"),
        degrees: text("degrees"),
        experience_years: years ? Number(years) : undefined,
        bio: text("bio") || undefined,
      },
    });
    setBusy(false);
    if (data) {
      setProfile(data);
      setSaved(true);
    } else setError(errorMessage(error));
  }

  if (!profile) return error ? <Alert>{error}</Alert> : <Spinner />;
  const status = profile.verification_status ?? "pending";
  const verified = status === "approved";

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <Badge tone={STATUS[status].tone}>{STATUS[status].label}</Badge>
        {verified && (
          <Link href={`/doctors/${profile.user_id}`} className="text-sm text-teal-800 hover:underline">
            View my public page
          </Link>
        )}
      </div>
      {status === "pending" && !profile.bmdc_reg_no && (
        <Alert kind="info">
          Add your BMDC registration number and qualifications. An admin checks them before patients can find you.
        </Alert>
      )}
      {status === "pending" && profile.bmdc_reg_no && <Alert kind="info">Your details are with an admin for checking.</Alert>}
      {status === "rejected" && (
        <Alert>
          Verification was rejected{profile.verification_note ? `: ${profile.verification_note}` : "."} Correct the details and save
          to ask again.
        </Alert>
      )}

      <Card>
        <form onSubmit={submit} className="space-y-4">
          <Field
            label="BMDC registration number"
            name="bmdc_reg_no"
            defaultValue={profile.bmdc_reg_no ?? ""}
            required
            placeholder="A-12345"
            disabled={verified}
            hint={verified ? "Verified numbers can't be changed. Contact support if it is wrong." : undefined}
          />
          {/* A disabled input isn't submitted; send the verified number unchanged. */}
          {verified && <input type="hidden" name="bmdc_reg_no" value={profile.bmdc_reg_no ?? ""} />}
          <Field label="Specialty" name="specialization" defaultValue={profile.specialization ?? ""} required placeholder="e.g. Medicine" />
          <Field label="Degrees" name="degrees" defaultValue={profile.degrees ?? ""} required placeholder="MBBS, FCPS (Medicine)" />
          <Field
            label="Years of experience"
            name="experience_years"
            type="number"
            min={0}
            max={70}
            defaultValue={profile.experience_years ?? ""}
          />
          <TextArea label="About you" name="bio" rows={4} maxLength={2000} defaultValue={profile.bio ?? ""} hint="Shown on your public page." />
          {error && <Alert>{error}</Alert>}
          {saved && <Alert kind="success">Saved.</Alert>}
          <Button type="submit" disabled={busy}>
            {busy ? "Saving…" : "Save"}
          </Button>
        </form>
      </Card>
    </div>
  );
}
