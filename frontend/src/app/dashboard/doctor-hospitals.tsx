"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { T } from "@/components/t";
import { Alert, Badge, Button, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type Invitation } from "@/lib/api/client";

// Hospitals that invited the doctor, and the ones they already work at.
export function DoctorHospitals() {
  const [items, setItems] = useState<Invitation[] | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState<string | null>(null);

  const reload = useCallback(async () => {
    const { data, error } = await api.GET("/api/v1/doctor/hospitals");
    if (data) setItems(data.hospitals ?? []);
    else setError(errorMessage(error));
  }, []);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    reload();
  }, [reload]);

  async function act(h: Invitation, kind: "accept" | "decline" | "leave") {
    const hospitalID = h.hospital_id ?? "";
    if (kind === "leave" && !confirm(`Leave ${h.hospital_name}? Booking stops for your chambers there.`)) return;
    setBusy(hospitalID);
    setError("");
    setNotice("");
    if (kind === "leave") {
      const { data, error } = await api.POST("/api/v1/doctor/hospitals/{hospitalID}/leave", { params: { path: { hospitalID } } });
      if (!data) setError(errorMessage(error));
      else if (data.upcoming_appointments) {
        setNotice(`You left ${h.hospital_name}. The hospital will handle your ${data.upcoming_appointments} upcoming booking(s).`);
      }
    } else {
      const path =
        kind === "accept" ? "/api/v1/doctor/hospitals/{hospitalID}/accept" : "/api/v1/doctor/hospitals/{hospitalID}/decline";
      const { error } = await api.POST(path, { params: { path: { hospitalID } } });
      if (error) setError(errorMessage(error));
    }
    setBusy(null);
    reload();
  }

  return (
    <Card>
      <h2 className="font-semibold text-slate-900">
        <T>Hospitals</T>
      </h2>
      {items === null ? (
        error ? <Alert>{error}</Alert> : <Spinner />
      ) : (
        <div className="mt-2 space-y-3">
          {notice && <Alert kind="info">{notice}</Alert>}
          {error && <Alert>{error}</Alert>}
          {items.length === 0 && (
            <p className="text-sm text-slate-600"><T>No hospital invitations. Hospitals invite you by your BMDC number.</T></p>
          )}
          {items.map((h) => (
            <div key={h.hospital_id} className="rounded-lg border border-slate-200 p-3 text-sm">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <Link href={`/hospitals/${h.hospital_id}`} className="font-medium text-slate-900 hover:underline">
                  {h.hospital_name}
                </Link>
                <Badge tone={h.status === "active" ? "teal" : "amber"}>{h.status === "active" ? "Working here" : "Invitation"}</Badge>
              </div>
              <p className="text-slate-600">
                {[h.designation, h.department, h.district].filter(Boolean).join(" · ")}
              </p>
              <div className="mt-2 flex gap-2">
                {h.status === "invited" ? (
                  <>
                    <Button onClick={() => act(h, "accept")} disabled={busy !== null}>
                      Accept
                    </Button>
                    <Button variant="secondary" onClick={() => act(h, "decline")} disabled={busy !== null}>
                      Decline
                    </Button>
                  </>
                ) : (
                  <Button variant="danger" onClick={() => act(h, "leave")} disabled={busy !== null}>
                    Leave
                  </Button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}
