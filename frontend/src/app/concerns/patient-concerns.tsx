"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";
import { TextArea } from "@/components/textarea";
import { Alert, Button, Card, Field, Spinner } from "@/components/ui";
import { useT } from "@/components/use-lang";
import { api, errorMessage, type Concern } from "@/lib/api/client";
import { ConcernCard } from "./concern-card";
import { T } from "@/components/t";

export function PatientConcerns() {
  const t = useT();
  const router = useRouter();
  const [items, setItems] = useState<Concern[] | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [writing, setWriting] = useState(false);

  useEffect(() => {
    api.GET("/api/v1/concerns", { params: { query: { limit: 50 } } }).then(({ data, error }) => {
      if (data) setItems(data.concerns ?? []);
      else setError(errorMessage(error));
    });
  }, []);

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    setBusy(true);
    setError("");
    const specialization = String(form.get("specialization")).trim();
    const { data, error } = await api.POST("/api/v1/concerns", {
      body: {
        title: String(form.get("title")).trim(),
        description: String(form.get("description")).trim(),
        specialization: specialization || undefined,
      },
    });
    setBusy(false);
    if (data?.id) router.push(`/concerns/${data.id}`);
    else setError(errorMessage(error));
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900"><T>My concerns</T></h1>
          <p className="text-sm text-slate-600">
            <T>Describe a health problem. Doctors see it without your name and offer to help; you choose one.</T>
          </p>
        </div>
        {!writing && <Button onClick={() => setWriting(true)}>Post a concern</Button>}
      </div>

      {writing && (
        <Card>
          <form onSubmit={onSubmit} className="space-y-4">
            <Field label="Title" name="title" minLength={5} maxLength={150} required placeholder={t("e.g. Back pain after long sitting")} />
            <TextArea
              label="Details"
              name="description"
              minLength={20}
              maxLength={5000}
              required
              hint="Doctors see this text but not your name or phone number."
            />
            <Field
              label="Specialty (optional)"
              name="specialization"
              maxLength={100}
              placeholder={t("e.g. Orthopedics")}
              hint="Helps the right doctors find it."
            />
            {error && <Alert>{error}</Alert>}
            <div className="flex gap-2">
              <Button type="submit" disabled={busy}>
                {busy ? "Posting…" : "Post"}
              </Button>
              <Button type="button" variant="secondary" onClick={() => setWriting(false)}>
                <T>Cancel</T>
              </Button>
            </div>
          </form>
        </Card>
      )}

      {!writing && error && <Alert>{error}</Alert>}
      {items === null ? (
        !error && <Spinner />
      ) : items.length === 0 ? (
        <p className="text-sm text-slate-600"><T>You haven&apos;t posted any concerns yet.</T></p>
      ) : (
        <ul className="space-y-3">
          {items.map((c) => (
            <li key={c.id}>
              <ConcernCard concern={c} showOffers />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
