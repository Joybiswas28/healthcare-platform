"use client";

import { useState } from "react";
import type { Chamber } from "@/lib/api/client";
import { clock, taka, WEEKDAYS } from "@/lib/format";
import { BookingPanel } from "./booking-panel";
import { T } from "./t";
import { Card } from "./ui";

// A chamber with its weekly sessions; picking a session opens the booking panel.
// Used on doctor profiles and hospital pages.
export function ChamberCard({ chamber: c, title }: { chamber: Chamber; title?: string }) {
  const [selected, setSelected] = useState<string | null>(null);
  const schedules = c.schedules ?? [];

  return (
    <Card>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="font-semibold text-slate-900">{title ?? c.name}</h3>
          {c.hospital_name && !title && (
            <p className="text-sm text-slate-700">
              {c.hospital_name}
              {c.room ? ` · ${c.room}` : ""}
            </p>
          )}
          {title && c.room && <p className="text-sm text-slate-700">{c.room}</p>}
          <p className="text-sm text-slate-600">{[c.address, c.area, c.district].filter(Boolean).join(", ")}</p>
          {c.phone && (
            <p className="text-xs text-slate-500">
              <T vars={{ phone: c.phone }}>{"Phone {phone}"}</T>
            </p>
          )}
        </div>
        <p className="text-sm text-slate-700">
          <T>Fee</T> <span className="font-semibold">{taka(c.fee_paisa)}</span>
        </p>
      </div>

      {schedules.length === 0 ? (
        <p className="mt-3 text-sm text-slate-500">
          <T>No sessions yet.</T>
        </p>
      ) : (
        <ul className="mt-4 grid gap-2 sm:grid-cols-2">
          {schedules.map((s) => (
            <li key={s.id}>
              <button
                onClick={() => setSelected(selected === s.id ? null : (s.id ?? null))}
                className={`w-full rounded-lg border px-3 py-2 text-left text-sm transition ${
                  selected === s.id ? "border-teal-700 bg-teal-50 text-teal-900" : "border-slate-200 hover:border-teal-600"
                }`}
                aria-pressed={selected === s.id}
              >
                <span className="font-medium">
                  <T>{WEEKDAYS[s.weekday ?? 0]}</T>
                </span>
                <span className="text-slate-600">
                  {" "}
                  · {clock(s.start_time)} – {clock(s.end_time)}
                </span>
                <span className="block text-xs text-slate-500">
                  <T vars={{ n: s.max_patients }}>{"Up to {n} patients"}</T>
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}

      {schedules.map((s) => (s.id === selected ? <BookingPanel key={s.id} schedule={s} chamber={c} /> : null))}
    </Card>
  );
}
