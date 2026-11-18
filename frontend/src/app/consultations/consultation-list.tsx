"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ConsultationBadge } from "@/components/consultation-status";
import { Alert, Button, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type Consultation, type ConsultationStatus } from "@/lib/api/client";
import { dateTime, tokens } from "@/lib/format";

const PAGE_SIZE = 20;

const FILTERS: { value: ConsultationStatus | ""; label: string }[] = [
  { value: "", label: "All" },
  { value: "requested", label: "Requested" },
  { value: "accepted", label: "Accepted" },
  { value: "active", label: "Active" },
  { value: "completed", label: "Completed" },
];

export function ConsultationList({ asDoctor }: { asDoctor: boolean }) {
  const [status, setStatus] = useState<ConsultationStatus | "">("");
  const [items, setItems] = useState<Consultation[]>([]);
  const [loading, setLoading] = useState(true);
  const [hasMore, setHasMore] = useState(false);
  const [error, setError] = useState("");

  async function load(offset: number, filter: ConsultationStatus | "") {
    setLoading(true);
    setError("");
    const { data, error } = await api.GET("/api/v1/consultations", {
      params: { query: { status: filter || undefined, limit: PAGE_SIZE, offset } },
    });
    setLoading(false);
    if (!data) {
      setError(errorMessage(error));
      return;
    }
    const page = data.consultations ?? [];
    setItems((prev) => (offset === 0 ? page : [...prev, ...page]));
    setHasMore(page.length === PAGE_SIZE);
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load(0, status);
  }, [status]);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap gap-2" role="tablist">
        {FILTERS.map((f) => (
          <button
            key={f.value}
            role="tab"
            aria-selected={status === f.value}
            onClick={() => setStatus(f.value)}
            className={`rounded-full px-3 py-1 text-sm ${
              status === f.value ? "bg-teal-700 text-white" : "bg-surface text-slate-700 ring-1 ring-slate-200 hover:ring-teal-600"
            }`}
          >
            {f.label}
          </button>
        ))}
      </div>

      {error && <Alert>{error}</Alert>}
      {!loading && !error && items.length === 0 && (
        <Card>
          <p className="text-slate-600">
            {asDoctor
              ? "No consultations here yet. Patients can request you once online consultations are turned on in your dashboard."
              : "No consultations here yet. Pick a doctor who consults online, or post a concern and let doctors come to you."}
          </p>
        </Card>
      )}

      <ul className="space-y-3">
        {items.map((c) => (
          <li key={c.id}>
            <Link
              href={`/consultations/${c.id}`}
              className="block rounded-xl border border-slate-200 bg-surface p-5 shadow-sm transition hover:border-teal-600"
            >
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <p className="font-semibold text-slate-900">{c.subject}</p>
                  <p className="text-sm text-slate-600">
                    {asDoctor ? `Patient: ${c.patient_name}` : c.doctor_name} · {tokens(c.fee_tokens)}
                  </p>
                </div>
                <ConsultationBadge status={c.status} asDoctor={asDoctor} />
              </div>
              <p className="mt-2 text-xs text-slate-500">Requested {dateTime(c.requested_at)}</p>
            </Link>
          </li>
        ))}
      </ul>

      {loading && <Spinner />}
      {!loading && hasMore && (
        <div className="text-center">
          <Button variant="secondary" onClick={() => load(items.length, status)}>
            Show more
          </Button>
        </div>
      )}
    </div>
  );
}
