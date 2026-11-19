"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { LoadFailure, loadFailure } from "@/components/load-failure";
import { ButtonLink, Card, Spinner } from "@/components/ui";
import { api, type Pharmacy, type Review } from "@/lib/api/client";
import { dateTime } from "@/lib/format";
import { mapLink } from "@/lib/geo";
import { PharmacySummary } from "../nearby-pharmacies";

export function PharmacyProfile() {
  const { pharmacyID } = useParams<{ pharmacyID: string }>();
  const [pharmacy, setPharmacy] = useState<Pharmacy | null>(null);
  const [reviews, setReviews] = useState<Review[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    api.GET("/api/v1/pharmacies/{pharmacyID}", { params: { path: { pharmacyID } } }).then(({ data, error, response }) => {
      if (data?.pharmacy) {
        setPharmacy(data.pharmacy);
        setReviews(data.reviews ?? []);
      } else setError(loadFailure(response.status, error, "pharmacy"));
    });
  }, [pharmacyID]);

  if (error) return <LoadFailure failure={error} backHref="/pharmacies" backLabel="Back to pharmacies" />;
  if (!pharmacy) return <Spinner />;
  const map = mapLink(pharmacy.latitude, pharmacy.longitude);

  return (
    <div className="space-y-4">
      <Link href="/pharmacies" className="text-sm text-teal-800 hover:underline">
        ← Pharmacies near you
      </Link>
      <Card className="space-y-3">
        <PharmacySummary pharmacy={pharmacy} />
        <p className="text-sm text-slate-600">
          {pharmacy.phone}
          {pharmacy.opening_hours ? ` · Open ${pharmacy.opening_hours}` : ""}
          {map && (
            <>
              {" · "}
              <a href={map} target="_blank" rel="noopener noreferrer" className="text-teal-800 hover:underline">
                Map
              </a>
            </>
          )}
        </p>
        <p className="text-xs text-slate-500">DGDA licence {pharmacy.drug_license_no}</p>
        <ButtonLink href="/medicines/new">Order medicine</ButtonLink>
      </Card>

      <section>
        <h2 className="mb-2 text-lg font-semibold text-slate-900">Reviews</h2>
        {reviews.length === 0 && <p className="text-sm text-slate-600">No reviews yet.</p>}
        <ul className="space-y-2">
          {reviews.map((r, i) => (
            <li key={i}>
              <Card className="p-4">
                <p className="text-amber-500" aria-label={`${r.rating} out of 5`}>
                  {"★".repeat(r.rating ?? 0)}
                  <span className="text-slate-300">{"★".repeat(5 - (r.rating ?? 0))}</span>
                </p>
                {r.comment && <p className="mt-1 text-sm text-slate-700">{r.comment}</p>}
                <p className="mt-1 text-xs text-slate-500">{dateTime(r.created_at)}</p>
              </Card>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
