"use client";

import { useEffect, useState, type FormEvent } from "react";
import { Alert, Button, Spinner } from "@/components/ui";
import { api, errorMessage, type Concern } from "@/lib/api/client";
import { ConcernCard } from "./concern-card";
import { T } from "@/components/t";
import { useT } from "@/components/use-lang";

export function DoctorFeed() {
  const [specialization, setSpecialization] = useState("");
  const [items, setItems] = useState<Concern[] | null>(null);
  const [error, setError] = useState("");
  const t = useT();

  useEffect(() => {
    api
      .GET("/api/v1/concerns", { params: { query: { specialization: specialization || undefined, limit: 50 } } })
      .then(({ data, error }) => {
        if (data) setItems(data.concerns ?? []);
        else setError(errorMessage(error));
      });
  }, [specialization]);

  function onFilter(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setItems(null);
    setSpecialization(String(new FormData(e.currentTarget).get("specialization")).trim());
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-slate-900"><T>Patient concerns</T></h1>
        <p className="text-sm text-slate-600">
          <T>Open questions from patients. Patients stay anonymous until they accept your offer.</T>
        </p>
      </div>
      <form onSubmit={onFilter} className="flex gap-2">
        <input
          name="specialization"
          defaultValue={specialization}
          placeholder={t("Filter by specialty")}
          aria-label={t("Filter by specialty")}
          className="flex-1 rounded-lg border border-slate-300 bg-surface px-3 py-2 text-sm outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-600/20"
        />
        <Button type="submit" variant="secondary">
          <T>Filter</T>
        </Button>
      </form>
      {error && <Alert>{error}</Alert>}
      {items === null ? (
        !error && <Spinner />
      ) : items.length === 0 ? (
        <p className="text-sm text-slate-600"><T>No open concerns right now.</T></p>
      ) : (
        <ul className="space-y-3">
          {items.map((c) => (
            <li key={c.id}>
              <ConcernCard concern={c} showOffers={false} />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
