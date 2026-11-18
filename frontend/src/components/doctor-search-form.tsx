"use client";

import { useRouter } from "next/navigation";
import type { FormEvent } from "react";

type Props = {
  initial?: { name?: string; specialization?: string; location?: string; online?: boolean };
};

// Submits to /doctors with the filters in the URL, so results can be shared and
// the back button works.
export function DoctorSearchForm({ initial = {} }: Props) {
  const router = useRouter();

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const params = new URLSearchParams();
    for (const key of ["name", "specialization", "location"]) {
      const value = String(form.get(key) ?? "").trim();
      if (value) params.set(key, value);
    }
    if (form.get("online")) params.set("online", "true");
    const qs = params.toString();
    router.push(qs ? `/doctors?${qs}` : "/doctors");
  }

  const input =
    "w-full rounded-lg border border-slate-300 bg-white px-3 py-2.5 text-sm outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-600/20";

  return (
    <form onSubmit={onSubmit} className="rounded-xl border border-slate-200 bg-white p-3 shadow-sm">
      <div className="grid gap-2 sm:grid-cols-[1fr_1fr_1fr_auto]">
        <input name="name" placeholder="Doctor's name" aria-label="Doctor's name" defaultValue={initial.name} className={input} />
        <input
          name="specialization"
          placeholder="Specialty, e.g. Cardiology"
          aria-label="Specialty"
          defaultValue={initial.specialization}
          className={input}
        />
        <input
          name="location"
          placeholder="District or area"
          aria-label="District or area"
          defaultValue={initial.location}
          className={input}
        />
        <button className="rounded-lg bg-teal-700 px-5 py-2.5 text-sm font-medium text-white hover:bg-teal-800">
          Search
        </button>
      </div>
      <label className="mt-2 flex items-center gap-2 px-1 text-sm text-slate-600">
        <input type="checkbox" name="online" defaultChecked={initial.online} className="accent-teal-700" />
        Only doctors who consult online
      </label>
    </form>
  );
}
