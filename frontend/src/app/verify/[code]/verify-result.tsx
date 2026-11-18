"use client";

import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { MedicineList } from "@/components/prescription-view";
import { Alert, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type PrescriptionVerification } from "@/lib/api/client";
import { dateTime } from "@/lib/format";
import { VerifyForm } from "../verify-form";

export function VerifyResult() {
  const { code } = useParams<{ code: string }>();
  const [result, setResult] = useState<PrescriptionVerification | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    api
      .GET("/api/v1/prescriptions/verify/{code}", { params: { path: { code: decodeURIComponent(code) } } })
      .then(({ data, error, response }) => {
        if (data) setResult(data);
        else
          setError(
            response.status === 404
              ? "No prescription has this code. It may be mistyped — or the prescription is not genuine."
              : errorMessage(error),
          );
      });
  }, [code]);

  if (error) {
    return (
      <div className="space-y-4">
        <Alert>{error}</Alert>
        <VerifyForm initial={decodeURIComponent(code)} />
      </div>
    );
  }
  if (!result) return <Spinner label="Checking…" />;

  const valid = result.status === "issued";

  return (
    <div className="space-y-4">
      {valid ? (
        <Alert kind="success">Genuine prescription, valid. Issued {dateTime(result.issued_at)}.</Alert>
      ) : (
        <Alert>This prescription was revoked by the doctor. Do not dispense from it.</Alert>
      )}
      <Card className="space-y-4">
        <div className="flex flex-wrap justify-between gap-3 text-sm">
          <div>
            <p className="font-semibold text-slate-900">{result.doctor_name}</p>
            <p className="text-slate-600">
              {result.doctor_specialization ? `${result.doctor_specialization} · ` : ""}BMDC {result.doctor_bmdc}
            </p>
          </div>
          <div className="text-right">
            <p className="text-slate-600">Patient</p>
            <p className="font-medium text-slate-900">{result.patient_name}</p>
          </div>
        </div>
        <MedicineList medicines={result.medicines ?? []} />
        <p className="font-mono text-xs text-slate-500">Code {result.code}</p>
      </Card>
    </div>
  );
}
