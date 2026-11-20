"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import { DoctorSearchForm } from "@/components/doctor-search-form";
import { T } from "@/components/t";
import { Alert, Badge, Button, Spinner } from "@/components/ui";
import { api, errorMessage, type DoctorSummary } from "@/lib/api/client";
import { taka } from "@/lib/format";

const PAGE_SIZE = 20;

export function DoctorResults() {
  const params = useSearchParams();
  const name = params.get("name") ?? "";
  const specialization = params.get("specialization") ?? "";
  const location = params.get("location") ?? "";
  const online = params.get("online") === "true";

  const [doctors, setDoctors] = useState<DoctorSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [hasMore, setHasMore] = useState(false);
  const [error, setError] = useState("");

  async function load(offset: number) {
    setLoading(true);
    setError("");
    const { data, error } = await api.GET("/api/v1/doctors", {
      params: {
        query: {
          name: name || undefined,
          specialization: specialization || undefined,
          location: location || undefined,
          online: online || undefined,
          limit: PAGE_SIZE,
          offset,
        },
      },
    });
    setLoading(false);
    if (error || !data) {
      setError(errorMessage(error));
      return;
    }
    const page = data.doctors ?? [];
    setDoctors((prev) => (offset === 0 ? page : [...prev, ...page]));
    setHasMore(page.length === PAGE_SIZE);
  }

  useEffect(() => {
    // The filters live in the URL; a new search starts again from the first page.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load(0);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [name, specialization, location, online]);

  return (
    <>
      <div className="mb-6">
        <DoctorSearchForm
          key={params.toString()}
          initial={{ name, specialization, location, online }}
        />
      </div>

      {error && <Alert>{error}</Alert>}

      {!loading && !error && doctors.length === 0 && (
        <p className="py-8 text-slate-600"><T>No doctors match this search. Try a different specialty or district.</T></p>
      )}

      <ul className="grid gap-3">
        {doctors.map((d) => (
          <li key={d.id}>
            <Link
              href={`/doctors/${d.id}`}
              className="block rounded-xl border border-slate-200 bg-surface p-5 shadow-sm transition hover:border-teal-600"
            >
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h2 className="font-semibold text-slate-900">{d.name}</h2>
                  <p className="text-sm text-slate-600">
                    {[d.specialization, d.degrees].filter(Boolean).join(" · ") || <T>General practice</T>}
                  </p>
                  {d.experience_years ? (
                    <p className="mt-1 text-xs text-slate-500"><T vars={{ n: d.experience_years }}>{"{n} years of experience"}</T></p>
                  ) : null}
                </div>
                <div className="text-right text-sm">
                  {d.min_fee_paisa != null && (
                    <p className="text-slate-700">
                      <T>Fee from</T> <span className="font-semibold">{taka(d.min_fee_paisa)}</span>
                    </p>
                  )}
                  {d.consultation_enabled && (
                    <p className="mt-1">
                      <Badge tone="teal">
                        <T vars={{ n: d.consultation_fee_tokens }}>{"Online · {n} tokens"}</T>
                      </Badge>
                    </p>
                  )}
                </div>
              </div>
              {d.districts && d.districts.length > 0 && (
                <p className="mt-3 text-xs text-slate-500"><T vars={{ places: d.districts.join(", ") }}>{"Chambers in {places}"}</T></p>
              )}
            </Link>
          </li>
        ))}
      </ul>

      {loading && <Spinner />}
      {!loading && hasMore && (
        <div className="mt-6 text-center">
          <Button variant="secondary" onClick={() => load(doctors.length)}>
            Show more
          </Button>
        </div>
      )}
    </>
  );
}
