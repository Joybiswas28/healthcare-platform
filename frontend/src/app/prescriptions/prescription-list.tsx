"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { Alert, Badge, Button, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type Prescription } from "@/lib/api/client";
import { dateTime } from "@/lib/format";

const PAGE_SIZE = 20;

type Filter = { consultation_id?: string; appointment_id?: string };

// Used on /prescriptions and, filtered, inside a consultation.
export function PrescriptionList({ asDoctor, filter, compact }: { asDoctor: boolean; filter?: Filter; compact?: boolean }) {
  const [items, setItems] = useState<Prescription[]>([]);
  const [loading, setLoading] = useState(true);
  const [hasMore, setHasMore] = useState(false);
  const [error, setError] = useState("");
  const consultationID = filter?.consultation_id;
  const appointmentID = filter?.appointment_id;

  async function load(offset: number) {
    setLoading(true);
    const { data, error } = await api.GET("/api/v1/prescriptions", {
      params: { query: { consultation_id: consultationID, appointment_id: appointmentID, limit: PAGE_SIZE, offset } },
    });
    setLoading(false);
    if (!data) {
      setError(errorMessage(error));
      return;
    }
    const page = data.prescriptions ?? [];
    setItems((prev) => (offset === 0 ? page : [...prev, ...page]));
    setHasMore(page.length === PAGE_SIZE);
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load(0);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [consultationID, appointmentID]);

  if (error) return <Alert>{error}</Alert>;
  if (loading && items.length === 0) return <Spinner />;
  if (items.length === 0) {
    return compact ? null : (
      <Card>
        <p className="text-slate-600">No prescriptions yet.</p>
      </Card>
    );
  }

  return (
    <div className="space-y-3">
      <ul className="space-y-3">
        {items.map((p) => (
          <li key={p.id}>
            <Link
              href={`/prescriptions/${p.id}`}
              className="block rounded-xl border border-slate-200 bg-surface p-4 shadow-sm transition hover:border-teal-600"
            >
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <p className="font-semibold text-slate-900">{asDoctor ? p.patient_name : p.doctor_name}</p>
                  <p className="text-sm text-slate-600">
                    {p.diagnosis || p.chief_complaints || `${p.medicines?.length ?? 0} medicines`}
                  </p>
                </div>
                {p.status === "revoked" ? <Badge tone="red">Revoked</Badge> : <Badge tone="teal">Valid</Badge>}
              </div>
              <p className="mt-2 text-xs text-slate-500">
                {dateTime(p.issued_at)} · Code <span className="font-mono">{p.code}</span>
              </p>
            </Link>
          </li>
        ))}
      </ul>
      {!loading && hasMore && (
        <div className="text-center">
          <Button variant="secondary" onClick={() => load(items.length)}>
            Show more
          </Button>
        </div>
      )}
    </div>
  );
}
