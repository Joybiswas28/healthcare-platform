"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { Alert, Badge, Button, ButtonLink, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type Appointment } from "@/lib/api/client";
import { clock, prettyDate, taka, todayInDhaka } from "@/lib/format";

const PAGE_SIZE = 20;

const STATUS: Record<string, { label: string; tone: "slate" | "teal" | "amber" | "red" }> = {
  confirmed: { label: "Confirmed", tone: "teal" },
  completed: { label: "Completed", tone: "slate" },
  cancelled: { label: "Cancelled", tone: "red" },
  no_show: { label: "Missed", tone: "amber" },
  rescheduled: { label: "Rescheduled", tone: "amber" },
};

export function AppointmentList() {
  const [items, setItems] = useState<Appointment[]>([]);
  const [loading, setLoading] = useState(true);
  const [hasMore, setHasMore] = useState(false);
  const [error, setError] = useState("");
  const [cancelling, setCancelling] = useState<string | null>(null);

  async function load(offset: number) {
    setLoading(true);
    const { data, error } = await api.GET("/api/v1/appointments", {
      params: { query: { limit: PAGE_SIZE, offset } },
    });
    setLoading(false);
    if (!data) {
      setError(errorMessage(error));
      return;
    }
    const page = data.appointments ?? [];
    setItems((prev) => (offset === 0 ? page : [...prev, ...page]));
    setHasMore(page.length === PAGE_SIZE);
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load(0);
  }, []);

  async function cancel(a: Appointment) {
    if (!a.id || !confirm(`Cancel your serial with ${a.doctor_name} on ${prettyDate(a.date)}?`)) return;
    setCancelling(a.id);
    setError("");
    const { data, error } = await api.POST("/api/v1/appointments/{appointmentID}/cancel", {
      params: { path: { appointmentID: a.id } },
    });
    setCancelling(null);
    if (!data) {
      setError(errorMessage(error));
      return;
    }
    setItems((prev) => prev.map((x) => (x.id === data.id ? data : x)));
  }

  const today = todayInDhaka();

  if (!loading && items.length === 0 && !error) {
    return (
      <Card>
        <p className="text-slate-700">You have no appointments yet.</p>
        <ButtonLink href="/doctors" className="mt-4">
          Find a doctor
        </ButtonLink>
      </Card>
    );
  }

  return (
    <div className="space-y-3">
      {error && <Alert>{error}</Alert>}
      {items.map((a) => {
        const status = STATUS[a.status ?? ""] ?? { label: a.status ?? "", tone: "slate" as const };
        const upcoming = a.status === "confirmed" && (a.date ?? "") >= today;
        return (
          <Card key={a.id}>
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <div className="flex items-center gap-2">
                  <Link href={`/doctors/${a.doctor_id}`} className="font-semibold text-slate-900 hover:underline">
                    {a.doctor_name}
                  </Link>
                  <Badge tone={status.tone}>{status.label}</Badge>
                </div>
                <p className="mt-1 text-sm text-slate-700">
                  {prettyDate(a.date)} · {clock(a.start_time)} – {clock(a.end_time)}
                </p>
                <p className="text-sm text-slate-600">
                  {a.hospital_name ? `${a.hospital_name} · ` : ""}
                  {a.chamber_name}
                  {a.room ? ` · Room ${a.room}` : ""}, {a.chamber_district}
                </p>
                {a.status === "cancelled" && a.cancelled_by && a.cancelled_by !== "patient" && (
                  <p className="mt-1 text-xs text-red-700">
                    Cancelled by the {a.cancelled_by}
                    {a.cancel_reason ? `: ${a.cancel_reason}` : ""}
                  </p>
                )}
              </div>
              <div className="text-right">
                <p className="text-sm text-slate-600">Serial</p>
                <p className="text-2xl font-semibold text-slate-900">{a.serial_no}</p>
              </div>
            </div>
            <div className="mt-3 flex flex-wrap items-center justify-between gap-3 border-t border-slate-100 pt-3 text-sm">
              <span className="text-slate-600">
                Ticket <span className="font-mono font-semibold tracking-wider text-slate-900">{a.ticket_code}</span> ·
                Fee {taka(a.fee_paisa)} (pay at the chamber)
              </span>
              {upcoming && (
                <Button variant="danger" onClick={() => cancel(a)} disabled={cancelling === a.id}>
                  {cancelling === a.id ? "Cancelling…" : "Cancel"}
                </Button>
              )}
            </div>
          </Card>
        );
      })}
      {loading && <Spinner />}
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
