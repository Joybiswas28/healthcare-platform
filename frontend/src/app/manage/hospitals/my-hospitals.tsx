"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Alert, Badge, Button, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type Hospital } from "@/lib/api/client";
import { HospitalLogo } from "@/app/hospitals/hospital-results";
import { HospitalForm } from "./hospital-form";
import { T } from "@/components/t";

export const VERIFICATION = {
  pending: { label: "Waiting for verification", tone: "amber" },
  approved: { label: "Verified", tone: "teal" },
  rejected: { label: "Verification rejected", tone: "red" },
} as const;

export function MyHospitals() {
  const router = useRouter();
  const [items, setItems] = useState<Hospital[] | null>(null);
  const [error, setError] = useState("");
  const [registering, setRegistering] = useState(false);

  useEffect(() => {
    api.GET("/api/v1/manage/hospitals").then(({ data, error }) => {
      if (data) setItems(data.hospitals ?? []);
      else setError(errorMessage(error));
    });
  }, []);

  if (!items) return error ? <Alert>{error}</Alert> : <Spinner />;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold text-slate-900"><T>My hospitals</T></h1>
        {!registering && <Button onClick={() => setRegistering(true)}>Register a hospital</Button>}
      </div>

      {registering && (
        <Card>
          <h2 className="mb-1 font-semibold text-slate-900"><T>Register a hospital</T></h2>
          <p className="mb-4 text-sm text-slate-600">
            <T>The hospital stays hidden from patients until an admin checks the licence number.</T>
          </p>
          <HospitalForm
            submitLabel="Submit for verification"
            onSave={async (input) => {
              const { data, error } = await api.POST("/api/v1/hospitals", { body: input });
              if (!data?.id) return errorMessage(error);
              router.push(`/manage/hospitals/${data.id}`);
              return "";
            }}
          />
          <Button variant="secondary" className="mt-3" onClick={() => setRegistering(false)}>
            <T>Cancel</T>
          </Button>
        </Card>
      )}

      {items.length === 0 && !registering && (
        <Card>
          <p className="text-slate-600">
            <T>
              {"You don't manage a hospital yet. Register one, or ask your hospital's owner account to add your phone number as staff."}
            </T>
          </p>
        </Card>
      )}

      <ul className="space-y-3">
        {items.map((h) => {
          const v = VERIFICATION[h.verification_status ?? "pending"];
          return (
            <li key={h.id}>
              <Link
                href={`/manage/hospitals/${h.id}`}
                className="flex gap-4 rounded-xl border border-slate-200 bg-surface p-4 shadow-sm transition hover:border-teal-600"
              >
                <HospitalLogo hospital={h} />
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <h2 className="font-semibold text-slate-900">{h.name}</h2>
                    <Badge tone={v.tone}>{v.label}</Badge>
                    {h.my_role && <Badge>{h.my_role === "owner" ? "Owner" : "Staff"}</Badge>}
                  </div>
                  <p className="text-sm text-slate-600">{[h.area, h.district].filter(Boolean).join(", ")}</p>
                  <p className="text-xs text-slate-500">
                    <T vars={{ n: h.doctor_count ?? 0 }}>{h.doctor_count === 1 ? "{n} doctor" : "{n} doctors"}</T>
                  </p>
                </div>
              </Link>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
