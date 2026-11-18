"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Alert, Badge, Button, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type Pharmacy } from "@/lib/api/client";
import { VERIFICATION } from "@/app/manage/hospitals/my-hospitals";
import { PharmacyForm } from "./pharmacy-form";

export function MyPharmacies() {
  const router = useRouter();
  const [items, setItems] = useState<Pharmacy[] | null>(null);
  const [error, setError] = useState("");
  const [registering, setRegistering] = useState(false);

  useEffect(() => {
    api.GET("/api/v1/manage/pharmacies").then(({ data, error }) => {
      if (data) {
        setItems(data.pharmacies ?? []);
        if ((data.pharmacies ?? []).length === 0) setRegistering(true);
      } else setError(errorMessage(error));
    });
  }, []);

  if (!items) return error ? <Alert>{error}</Alert> : <Spinner />;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold text-slate-900">My pharmacies</h1>
        {!registering && <Button onClick={() => setRegistering(true)}>Register a pharmacy</Button>}
      </div>

      {registering && (
        <Card>
          <h2 className="mb-1 font-semibold text-slate-900">Register a pharmacy</h2>
          <p className="mb-4 text-sm text-slate-600">
            Patients see the pharmacy after an admin checks the drug licence number.
          </p>
          <PharmacyForm
            submitLabel="Submit for verification"
            onSave={async (input) => {
              const { data, error } = await api.POST("/api/v1/pharmacies", { body: input });
              if (!data?.id) return errorMessage(error);
              router.push(`/manage/pharmacies/${data.id}`);
              return "";
            }}
          />
          {items.length > 0 && (
            <Button variant="secondary" className="mt-3" onClick={() => setRegistering(false)}>
              Cancel
            </Button>
          )}
        </Card>
      )}

      <ul className="space-y-3">
        {items.map((p) => {
          const v = VERIFICATION[p.verification_status ?? "pending"];
          return (
            <li key={p.id}>
              <Link
                href={`/manage/pharmacies/${p.id}`}
                className="block rounded-xl border border-slate-200 bg-surface p-4 shadow-sm transition hover:border-teal-600"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <h2 className="font-semibold text-slate-900">{p.name}</h2>
                  <Badge tone={v.tone}>{v.label}</Badge>
                  {p.accepting_orders === false && <Badge tone="amber">Not taking orders</Badge>}
                </div>
                <p className="text-sm text-slate-600">{[p.address, p.area, p.district].filter(Boolean).join(", ")}</p>
              </Link>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
