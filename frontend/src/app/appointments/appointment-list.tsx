"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ForMember } from "@/components/family";
import { RatingForm, StarRow } from "@/components/rating";
import { ShareWithAppointment } from "@/components/records";
import { Alert, Badge, Button, ButtonLink, Card, Spinner } from "@/components/ui";
import { useT } from "@/components/use-lang";
import { api, errorMessage, type Appointment } from "@/lib/api/client";
import { clock, prettyDate, taka, todayInDhaka } from "@/lib/format";
import { T } from "@/components/t";

const PAGE_SIZE = 20;

const STATUS: Record<string, { label: string; tone: "slate" | "teal" | "amber" | "red" }> = {
  confirmed: { label: "Confirmed", tone: "teal" },
  completed: { label: "Completed", tone: "slate" },
  cancelled: { label: "Cancelled", tone: "red" },
  no_show: { label: "Missed", tone: "amber" },
  rescheduled: { label: "Rescheduled", tone: "amber" },
};

export function AppointmentList() {
  const t = useT();
  const [items, setItems] = useState<Appointment[]>([]);
  const [loading, setLoading] = useState(true);
  const [hasMore, setHasMore] = useState(false);
  const [error, setError] = useState("");
  const [cancelling, setCancelling] = useState<string | null>(null);
  // appointment id -> the rating the patient gave; undefined until loaded
  const [ratings, setRatings] = useState<Record<string, number> | undefined>(undefined);
  const [rating, setRating] = useState<string | null>(null);
  const [sharing, setSharing] = useState<string | null>(null);

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
    api.GET("/api/v1/reviews/mine").then(({ data }) => {
      const out: Record<string, number> = {};
      for (const r of data?.reviews ?? []) if (r.appointment_id) out[r.appointment_id] = r.rating ?? 0;
      setRatings(out);
    });
  }, []);

  async function sendReview(a: Appointment, stars: number, comment: string | undefined): Promise<string> {
    const { data, error } = await api.POST("/api/v1/appointments/{appointmentID}/review", {
      params: { path: { appointmentID: a.id ?? "" } },
      body: { rating: stars, comment },
    });
    if (!data) return errorMessage(error);
    setRatings((prev) => ({ ...prev, [a.id ?? ""]: stars }));
    setRating(null);
    return "";
  }

  async function cancel(a: Appointment) {
    if (!a.id || !confirm(t("Cancel your serial with {doctor} on {date}?", { doctor: a.doctor_name, date: prettyDate(a.date) }))) return;
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
        <p className="text-slate-700"><T>You have no appointments yet.</T></p>
        <ButtonLink href="/doctors" className="mt-4">
          <T>Find a doctor</T>
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
                {a.family_member_id && (
                  <p className="mt-1 flex flex-wrap items-center gap-2 text-sm text-slate-700">
                    <T vars={{ name: a.patient_name ?? "" }}>{"For {name}"}</T>
                    <ForMember relation={a.relation} birthDate={a.patient_birth_date} />
                  </p>
                )}
                <p className="mt-1 text-sm text-slate-700">
                  {prettyDate(a.date)} · {clock(a.start_time)} – {clock(a.end_time)}
                </p>
                <p className="text-sm text-slate-600">
                  {a.hospital_name ? `${a.hospital_name} · ` : ""}
                  {a.chamber_name}
                  {a.room ? ` · ${a.room}` : ""}, {a.chamber_district}
                </p>
                {a.status === "cancelled" && a.cancelled_by && a.cancelled_by !== "patient" && (
                  <p className="mt-1 text-xs text-red-700">
                    {t("Cancelled by the {who}", { who: t(a.cancelled_by ?? "") })}
                    {a.cancel_reason ? `: ${a.cancel_reason}` : ""}
                  </p>
                )}
              </div>
              <div className="text-right">
                <p className="text-sm text-slate-600"><T>Serial</T></p>
                <p className="text-2xl font-semibold text-slate-900">{a.serial_no}</p>
              </div>
            </div>
            <div className="mt-3 flex flex-wrap items-center justify-between gap-3 border-t border-slate-100 pt-3 text-sm">
              <span className="text-slate-600">
                {t("Ticket")} <span className="font-mono font-semibold tracking-wider text-slate-900">{a.ticket_code}</span> ·{" "}
                {t("Fee {fee} (pay at the chamber)", { fee: taka(a.fee_paisa) })}
              </span>
              {upcoming && (
                <Button variant="danger" onClick={() => cancel(a)} disabled={cancelling === a.id}>
                  {cancelling === a.id ? "Cancelling…" : "Cancel"}
                </Button>
              )}
              {(upcoming || a.status === "completed") && a.id && (
                <Button variant="secondary" onClick={() => setSharing(sharing === a.id ? null : (a.id ?? null))}>
                  📎 Share reports
                </Button>
              )}
              {a.status === "completed" && ratings && a.id && (
                ratings[a.id] ? (
                  <span className="flex items-center gap-2 text-xs text-slate-500">
                    <T>You rated</T> <StarRow rating={ratings[a.id]} />
                  </span>
                ) : (
                  rating !== a.id && (
                    <Button variant="secondary" onClick={() => setRating(a.id ?? null)}>
                      Rate the doctor
                    </Button>
                  )
                )
              )}
            </div>
            {sharing === a.id && a.id && (
              <div className="mt-3 border-t border-slate-100 pt-3">
                <ShareWithAppointment appointmentID={a.id} />
              </div>
            )}
            {rating === a.id && a.id && !ratings?.[a.id] && (
              <div className="mt-3 border-t border-slate-100 pt-3">
                <RatingForm
                  title="How was your visit?"
                  hint={<T>Your name is not shown with the review.</T>}
                  send={(stars, comment) => sendReview(a, stars, comment)}
                />
              </div>
            )}
          </Card>
        );
      })}
      {loading && <Spinner />}
      {!loading && hasMore && (
        <div className="text-center">
          <Button variant="secondary" onClick={() => load(items.length)}>
            <T>Show more</T>
          </Button>
        </div>
      )}
    </div>
  );
}
