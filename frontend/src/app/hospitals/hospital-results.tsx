"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";
import { Alert, Button, Spinner } from "@/components/ui";
import { api, errorMessage, type Hospital } from "@/lib/api/client";

const PAGE_SIZE = 20;
const FILTERS = ["name", "location", "department", "facility"] as const;
const PLACEHOLDERS = { name: "Hospital name", location: "District or area", department: "Department, e.g. Cardiology", facility: "Facility, e.g. ICU" };

export function HospitalResults() {
  const router = useRouter();
  const params = useSearchParams();
  const filters = Object.fromEntries(FILTERS.map((k) => [k, params.get(k) ?? ""])) as Record<(typeof FILTERS)[number], string>;
  const key = params.toString();

  const [items, setItems] = useState<Hospital[]>([]);
  const [loading, setLoading] = useState(true);
  const [hasMore, setHasMore] = useState(false);
  const [error, setError] = useState("");

  async function load(offset: number) {
    setLoading(true);
    setError("");
    const query = Object.fromEntries(FILTERS.map((k) => [k, filters[k] || undefined]));
    const { data, error } = await api.GET("/api/v1/hospitals", { params: { query: { ...query, limit: PAGE_SIZE, offset } } });
    setLoading(false);
    if (!data) {
      setError(errorMessage(error));
      return;
    }
    const page = data.hospitals ?? [];
    setItems((prev) => (offset === 0 ? page : [...prev, ...page]));
    setHasMore(page.length === PAGE_SIZE);
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load(0);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const next = new URLSearchParams();
    for (const k of FILTERS) {
      const v = String(form.get(k) ?? "").trim();
      if (v) next.set(k, v);
    }
    router.push(next.size ? `/hospitals?${next}` : "/hospitals");
  }

  return (
    <>
      <form key={key} onSubmit={onSubmit} className="mb-6 rounded-xl border border-slate-200 bg-white p-3 shadow-sm">
        <div className="grid gap-2 sm:grid-cols-[1fr_1fr_1fr_1fr_auto]">
          {FILTERS.map((k) => (
            <input
              key={k}
              name={k}
              defaultValue={filters[k]}
              placeholder={PLACEHOLDERS[k]}
              aria-label={PLACEHOLDERS[k]}
              className="w-full rounded-lg border border-slate-300 px-3 py-2.5 text-sm outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-600/20"
            />
          ))}
          <Button type="submit">Search</Button>
        </div>
      </form>

      {error && <Alert>{error}</Alert>}
      {!loading && !error && items.length === 0 && <p className="py-8 text-slate-600">No hospitals match this search.</p>}

      <ul className="grid gap-3 sm:grid-cols-2">
        {items.map((h) => (
          <li key={h.id}>
            <Link
              href={`/hospitals/${h.id}`}
              className="flex h-full gap-4 rounded-xl border border-slate-200 bg-white p-4 shadow-sm transition hover:border-teal-600"
            >
              <HospitalLogo hospital={h} />
              <div className="min-w-0">
                <h2 className="font-semibold text-slate-900">{h.name}</h2>
                <p className="text-sm text-slate-600">{[h.area, h.district].filter(Boolean).join(", ")}</p>
                <p className="mt-1 text-xs text-slate-500">
                  {h.doctor_count ?? 0} doctors
                  {(h.departments ?? []).length > 0 ? ` · ${h.departments!.slice(0, 3).join(", ")}` : ""}
                  {(h.departments ?? []).length > 3 ? "…" : ""}
                </p>
              </div>
            </Link>
          </li>
        ))}
      </ul>

      {loading && <Spinner />}
      {!loading && hasMore && (
        <div className="mt-6 text-center">
          <Button variant="secondary" onClick={() => load(items.length)}>
            Show more
          </Button>
        </div>
      )}
    </>
  );
}

export function HospitalLogo({ hospital: h, size = "h-14 w-14" }: { hospital: Hospital; size?: string }) {
  if (h.logo_url) {
    // Photos come from the API through this app's proxy, so next/image's optimiser adds nothing.
    // eslint-disable-next-line @next/next/no-img-element
    return <img src={h.logo_url} alt="" className={`${size} shrink-0 rounded-lg border border-slate-200 object-cover`} />;
  }
  return (
    <div className={`${size} grid shrink-0 place-items-center rounded-lg bg-teal-50 text-lg font-semibold text-teal-800`} aria-hidden>
      {h.name?.charAt(0) ?? "H"}
    </div>
  );
}
