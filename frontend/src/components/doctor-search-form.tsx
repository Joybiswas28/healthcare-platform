"use client";

import { useRouter } from "next/navigation";
import type { FormEvent } from "react";
import { T } from "./t";
import { useT } from "./use-lang";

type Props = {
  initial?: { name?: string; specialization?: string; location?: string; online?: boolean; sort?: string };
};

// Submits to /doctors with the filters in the URL, so results can be shared and
// the back button works.
export function DoctorSearchForm({ initial = {} }: Props) {
  const router = useRouter();
  const t = useT();

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const params = new URLSearchParams();
    for (const key of ["name", "specialization", "location", "sort"]) {
      const value = String(form.get(key) ?? "").trim();
      if (value) params.set(key, value);
    }
    if (form.get("online")) params.set("online", "true");
    const qs = params.toString();
    router.push(qs ? `/doctors?${qs}` : "/doctors");
  }

  const input =
    "w-full rounded-lg border border-slate-300 bg-surface px-3 py-2.5 text-sm outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-600/20";

  return (
    <form onSubmit={onSubmit} className="rounded-xl border border-slate-200 bg-surface p-3 shadow-sm">
      <div className="grid gap-2 sm:grid-cols-[1fr_1fr_1fr_auto]">
        <input name="name" placeholder={t("Doctor's name")} aria-label={t("Doctor's name")} defaultValue={initial.name} className={input} />
        <input
          name="specialization"
          placeholder={t("Specialty, e.g. Cardiology")}
          aria-label={t("Specialty")}
          defaultValue={initial.specialization}
          className={input}
        />
        <input
          name="location"
          placeholder={t("District or area")}
          aria-label={t("District or area")}
          defaultValue={initial.location}
          className={input}
        />
        <button className="rounded-lg bg-teal-700 px-5 py-2.5 text-sm font-medium text-white hover:bg-teal-800 dark:hover:bg-teal-600">
          <T>Search</T>
        </button>
      </div>
      <div className="mt-2 flex flex-wrap items-center justify-between gap-2 px-1 text-sm text-slate-600">
        <label className="flex items-center gap-2">
          <input type="checkbox" name="online" defaultChecked={initial.online} className="accent-teal-700" />
          <T>Only doctors who consult online</T>
        </label>
        <label className="flex items-center gap-2">
          <T>Sort by</T>
          <select
            name="sort"
            defaultValue={initial.sort ?? ""}
            onChange={(e) => e.currentTarget.form?.requestSubmit()}
            className="rounded-lg border border-slate-300 bg-surface px-2 py-1 text-sm outline-none focus:border-teal-600"
          >
            <option value="">{t("Name")}</option>
            <option value="rating">{t("Highest rated")}</option>
            <option value="fee">{t("Lowest fee")}</option>
          </select>
        </label>
      </div>
    </form>
  );
}
