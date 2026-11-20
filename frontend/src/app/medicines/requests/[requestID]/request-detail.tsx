"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { RequireAuth } from "@/components/require-auth";
import { LoadFailure, loadFailure } from "@/components/load-failure";
import { Alert, Badge, Button, Card, Spinner } from "@/components/ui";
import { useT } from "@/components/use-lang";
import { api, errorMessage, type MedicineRequest, type Quote } from "@/lib/api/client";
import { dateTime, taka } from "@/lib/format";
import { km } from "@/lib/geo";
import { Stars } from "@/app/pharmacies/nearby-pharmacies";
import { T } from "@/components/t";

const POLL_MS = 15000;

export function RequestDetail() {
  return <RequireAuth>{() => <Detail />}</RequireAuth>;
}

function Detail() {
  const t = useT();
  const { requestID } = useParams<{ requestID: string }>();
  const router = useRouter();
  const [req, setReq] = useState<MedicineRequest | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState<string | null>(null);

  const reload = useCallback(async () => {
    const { data, error, response } = await api.GET("/api/v1/medicine-requests/{requestID}", { params: { path: { requestID } } });
    if (data) setReq(data);
    else setError(loadFailure(response.status, error, "request"));
  }, [requestID]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    reload();
  }, [reload]);

  // New quotes arrive while the request is open.
  const open = req?.status === "open";
  useEffect(() => {
    if (!open) return;
    const t = setInterval(reload, POLL_MS);
    return () => clearInterval(t);
  }, [open, reload]);

  async function accept(q: Quote) {
    if (!q.id || !confirm(t(req?.fulfilment === "pickup" ? "Order from {pharmacy} for {total}, cash on pickup?" : "Order from {pharmacy} for {total}, cash on delivery?", { pharmacy: q.pharmacy_name, total: taka(q.total_paisa) }))) return;
    setBusy(q.id);
    setError("");
    const { data, error } = await api.POST("/api/v1/quotes/{quoteID}/accept", { params: { path: { quoteID: q.id } } });
    setBusy(null);
    if (data?.id) router.push(`/medicines/orders/${data.id}`);
    else setError(errorMessage(error));
  }

  async function cancel() {
    if (!confirm(t("Cancel this request? Pharmacies' quotes will be dropped."))) return;
    setBusy("cancel");
    const { data, error } = await api.POST("/api/v1/medicine-requests/{requestID}/cancel", { params: { path: { requestID } } });
    setBusy(null);
    if (data) setReq(data);
    else setError(errorMessage(error));
  }

  if (!req) return error ? <LoadFailure failure={error} backHref="/medicines" backLabel="Back to medicines" /> : <Spinner />;
  const quotes = req.quotes ?? [];

  return (
    <div className="space-y-4">
      <Link href="/medicines" className="text-sm text-teal-800 hover:underline">
        ← <T>Medicines</T>
      </Link>

      <Card className="space-y-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <h1 className="text-xl font-semibold text-slate-900"><T>Medicine request</T></h1>
          <Badge tone={req.status === "open" ? "amber" : req.status === "ordered" ? "teal" : "slate"}>{req.status}</Badge>
        </div>
        {req.prescription && (
          <p className="text-sm text-slate-700">
            {t("Prescription by {doctor} ({date}):", { doctor: req.prescription.doctor_name, date: dateTime(req.prescription.issued_at) })}{" "}
            {(req.prescription.medicines ?? []).map((m) => `${m.name}${m.strength ? ` ${m.strength}` : ""}`).join(", ")}
          </p>
        )}
        {req.has_attachment && (
          <a href={`/api/v1/medicine-requests/${req.id}/attachment`} className="text-sm text-teal-800 underline">
            📎 Your uploaded prescription
          </a>
        )}
        {(req.items ?? []).length > 0 && (
          <ul className="list-inside list-disc text-sm text-slate-700">
            {req.items!.map((i, n) => (
              <li key={n}>{[i.name, i.strength, i.quantity].filter(Boolean).join(" · ")}</li>
            ))}
          </ul>
        )}
        <p className="text-sm text-slate-600">
          {req.fulfilment === "pickup" ? t("Pickup from the pharmacy") : t("Delivery to {address}", { address: req.address })} · {req.contact_phone}
        </p>
        {req.status === "open" && <p className="text-xs text-slate-500">{t("Open until {date}. New quotes appear here automatically.", { date: dateTime(req.expires_at) })}</p>}
        {req.status === "ordered" && req.order_id && (
          <Alert kind="success">
            <T>You ordered.</T>{" "}
            <Link href={`/medicines/orders/${req.order_id}`} className="font-medium underline">
              <T>Track the order</T>
            </Link>
          </Alert>
        )}
      </Card>

      {error && <Alert>{error}</Alert>}

      <section className="space-y-3">
        <h2 className="text-lg font-semibold text-slate-900">Quotes ({quotes.length})</h2>
        {quotes.length === 0 && req.status === "open" && (
          <p className="text-sm text-slate-600"><T>No quotes yet. Nearby pharmacies have been notified.</T></p>
        )}
        {quotes.map((q) => (
          <Card key={q.id}>
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <Link href={`/pharmacies/${q.pharmacy_id}`} className="font-semibold text-slate-900 hover:underline">
                  {q.pharmacy_name}
                </Link>
                <p className="text-sm text-slate-600">
                  {[q.pharmacy_area, km(q.distance_km)].filter(Boolean).join(" · ")} · {t("ready in about {n} min", { n: q.eta_minutes })}
                </p>
                <Stars rating={q.pharmacy_rating} count={q.pharmacy_review_count} />
              </div>
              <div className="text-right">
                <p className="text-xl font-semibold text-slate-900">{taka(q.total_paisa)}</p>
                <p className="text-xs text-slate-500">
                  {t("medicines {amount}", { amount: taka(q.medicines_total_paisa) })}
                  {(q.delivery_charge_paisa ?? 0) > 0 ? ` + ${t("delivery {amount}", { amount: taka(q.delivery_charge_paisa) })}` : ""}
                </p>
              </div>
            </div>
            <ul className="mt-3 space-y-1 text-sm">
              {(q.items ?? []).map((i, n) => (
                <li key={n} className={`flex justify-between gap-3 ${i.available ? "text-slate-700" : "text-slate-400 line-through"}`}>
                  <span>{[i.name, i.strength, i.quantity].filter(Boolean).join(" · ")}</span>
                  <span>{i.available ? taka(i.price_paisa) : "not available"}</span>
                </li>
              ))}
            </ul>
            {q.note && <p className="mt-2 text-sm text-slate-600">“{q.note}”</p>}
            <div className="mt-3 flex items-center justify-between gap-3">
              <span className="text-xs text-slate-500">
                {q.available_count} of {(q.items ?? []).length} available
              </span>
              {req.status === "open" && q.status === "pending" ? (
                <Button onClick={() => accept(q)} disabled={busy !== null}>
                  {busy === q.id ? "Ordering…" : "Order from here"}
                </Button>
              ) : (
                <Badge tone={q.status === "accepted" ? "teal" : "slate"}>{q.status}</Badge>
              )}
            </div>
          </Card>
        ))}
      </section>

      {req.status === "open" && (
        <Button variant="danger" onClick={cancel} disabled={busy !== null}>
          <T>Cancel request</T>
        </Button>
      )}
    </div>
  );
}
