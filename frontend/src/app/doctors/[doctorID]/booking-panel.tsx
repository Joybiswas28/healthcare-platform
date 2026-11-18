"use client";

import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { useAuth } from "@/components/auth-provider";
import { Alert, Button, ButtonLink } from "@/components/ui";
import { api, errorMessage, type Appointment, type Chamber, type Schedule } from "@/lib/api/client";
import { clock, prettyDate, taka, upcomingDates } from "@/lib/format";

type Availability = { remaining?: number; booked?: number; max_patients?: number };

export function BookingPanel({ schedule, chamber }: { schedule: Schedule; chamber: Chamber }) {
  const { user, loading: authLoading } = useAuth();
  const pathname = usePathname();
  const dates = upcomingDates(schedule.weekday ?? 0);
  const [date, setDate] = useState(dates[0] ?? "");
  const [availability, setAvailability] = useState<Availability | null>(null);
  const [booking, setBooking] = useState(false);
  const [error, setError] = useState("");
  const [booked, setBooked] = useState<Appointment | null>(null);

  useEffect(() => {
    if (!date || !schedule.id) return;
    let cancelled = false;
    api
      .GET("/api/v1/schedules/{scheduleID}/availability", {
        params: { path: { scheduleID: schedule.id }, query: { date } },
      })
      .then(({ data }) => {
        if (!cancelled) setAvailability(data ?? null);
      });
    return () => {
      cancelled = true;
    };
  }, [date, schedule.id]);

  async function book() {
    if (!schedule.id) return;
    setBooking(true);
    setError("");
    const { data, error } = await api.POST("/api/v1/appointments", {
      body: { schedule_id: schedule.id, date },
    });
    setBooking(false);
    if (data) setBooked(data);
    else setError(errorMessage(error));
  }

  if (booked) {
    return (
      <div className="mt-4 rounded-lg border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-900">
        <p className="font-semibold">Booked! Your serial number is {booked.serial_no}.</p>
        <p className="mt-1">
          {prettyDate(booked.date)} · {clock(booked.start_time)} at {booked.chamber_name}
        </p>
        <p className="mt-1">
          Ticket code: <span className="font-mono font-semibold tracking-wider">{booked.ticket_code}</span> — show
          this at the chamber.
        </p>
        <ButtonLink href="/appointments" variant="secondary" className="mt-3">
          View my appointments
        </ButtonLink>
      </div>
    );
  }

  const full = availability?.remaining === 0;

  return (
    <div className="mt-4 space-y-3 rounded-lg border border-slate-200 bg-slate-50 p-4">
      {dates.length === 0 ? (
        <p className="text-sm text-slate-600">No dates in the next 30 days.</p>
      ) : (
        <>
          <div>
            <p className="mb-2 text-sm font-medium text-slate-700">Choose a date</p>
            <div className="flex flex-wrap gap-2">
              {dates.map((d) => (
                <button
                  key={d}
                  onClick={() => {
                    setDate(d);
                    setAvailability(null);
                  }}
                  className={`rounded-lg border px-3 py-1.5 text-sm ${
                    d === date ? "border-teal-700 bg-teal-700 text-white" : "border-slate-300 bg-white hover:border-teal-600"
                  }`}
                  aria-pressed={d === date}
                >
                  {prettyDate(d)}
                </button>
              ))}
            </div>
          </div>

          <p className="text-sm text-slate-600">
            {availability
              ? full
                ? "This session is full. Please choose another date."
                : `${availability.remaining} of ${availability.max_patients} serials left · fee ${taka(chamber.fee_paisa)}`
              : "Checking serials…"}
          </p>

          {error && <Alert>{error}</Alert>}

          {authLoading ? null : user ? (
            user.role === "patient" ? (
              <Button onClick={book} disabled={booking || full || !availability}>
                {booking ? "Booking…" : "Book serial"}
              </Button>
            ) : (
              <p className="text-sm text-slate-600">Only patient accounts can book serials.</p>
            )
          ) : (
            <ButtonLink href={`/login?next=${encodeURIComponent(pathname)}`}>Log in to book</ButtonLink>
          )}
        </>
      )}
    </div>
  );
}
