"use client";

import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { useAuth } from "@/components/auth-provider";
import { PersonPicker, useFamily } from "@/components/family";
import { T } from "@/components/t";
import { Alert, Button, ButtonLink } from "@/components/ui";
import { useT } from "@/components/use-lang";
import { api, errorMessage, type Appointment, type Chamber, type Schedule } from "@/lib/api/client";
import { clock, prettyDate, taka, upcomingDates } from "@/lib/format";

type Availability = { remaining?: number; booked?: number; max_patients?: number };

export function BookingPanel({ schedule, chamber }: { schedule: Schedule; chamber: Chamber }) {
  const { user, loading: authLoading } = useAuth();
  const pathname = usePathname();
  const t = useT();
  const dates = upcomingDates(schedule.weekday ?? 0);
  const [date, setDate] = useState(dates[0] ?? "");
  const [availability, setAvailability] = useState<Availability | null>(null);
  const [booking, setBooking] = useState(false);
  const [error, setError] = useState("");
  const [booked, setBooked] = useState<Appointment | null>(null);
  const [forMember, setForMember] = useState("");
  const family = useFamily(user?.role === "patient");

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
      body: { schedule_id: schedule.id, date, family_member_id: forMember || undefined },
    });
    setBooking(false);
    if (data) setBooked(data);
    else setError(errorMessage(error));
  }

  if (booked) {
    return (
      <div className="mt-4 rounded-lg border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-900">
        <p className="font-semibold">
          {booked.family_member_id ? (
            <T vars={{ name: booked.patient_name ?? "", n: booked.serial_no ?? 0 }}>{"Booked for {name}! The serial number is {n}."}</T>
          ) : (
            <T vars={{ n: booked.serial_no }}>{"Booked! Your serial number is {n}."}</T>
          )}
        </p>
        <p className="mt-1">
          {prettyDate(booked.date)} · {clock(booked.start_time)} · {booked.chamber_name}
        </p>
        <p className="mt-1">
          <T>Ticket code:</T> <span className="font-mono font-semibold tracking-wider">{booked.ticket_code}</span> — <T>show this at the chamber.</T>
        </p>
        <ButtonLink href="/appointments" variant="secondary" className="mt-3">
          <T>View my appointments</T>
        </ButtonLink>
      </div>
    );
  }

  const full = availability?.remaining === 0;

  return (
    <div className="mt-4 space-y-3 rounded-lg border border-slate-200 bg-slate-50 p-4">
      {dates.length === 0 ? (
        <p className="text-sm text-slate-600">
          <T>No dates in the next 30 days.</T>
        </p>
      ) : (
        <>
          <div>
            <p className="mb-2 text-sm font-medium text-slate-700">
              <T>Choose a date</T>
            </p>
            <div className="flex flex-wrap gap-2">
              {dates.map((d) => (
                <button
                  key={d}
                  onClick={() => {
                    setDate(d);
                    setAvailability(null);
                  }}
                  className={`rounded-lg border px-3 py-1.5 text-sm ${
                    d === date ? "border-teal-700 bg-teal-700 text-white" : "border-slate-300 bg-surface hover:border-teal-600"
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
                ? t("This session is full. Please choose another date.")
                : t("{left} of {max} serials left · fee {fee}", { left: availability.remaining, max: availability.max_patients, fee: taka(chamber.fee_paisa) })
              : t("Checking serials…")}
          </p>

          {error && <Alert>{error}</Alert>}

          {authLoading ? null : user ? (
            user.role === "patient" ? (
              <div className="space-y-3">
                {family && <PersonPicker members={family} me={user.name ?? ""} value={forMember} onChange={setForMember} />}
                <Button onClick={book} disabled={booking || full || !availability}>
                  {booking ? "Booking…" : "Book serial"}
                </Button>
              </div>
            ) : (
              <p className="text-sm text-slate-600">
                <T>Only patient accounts can book serials.</T>
              </p>
            )
          ) : (
            <ButtonLink href={`/login?next=${encodeURIComponent(pathname)}`}>Log in to book</ButtonLink>
          )}
        </>
      )}
    </div>
  );
}
