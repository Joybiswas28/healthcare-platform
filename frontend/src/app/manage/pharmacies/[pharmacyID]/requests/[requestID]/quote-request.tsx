"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { RequireAuth } from "@/components/require-auth";
import { LoadFailure, loadFailure } from "@/components/load-failure";
import { Alert, Badge, Button, Card, Field, Spinner } from "@/components/ui";
import { api, errorMessage, type MedicineRequest, type QuotedItem } from "@/lib/api/client";
import { dateTime, taka } from "@/lib/format";
import { km } from "@/lib/geo";

type Row = { name: string; strength: string; quantity: string; available: boolean; price: string };

const input =
  "w-full rounded-lg border border-slate-300 bg-surface px-2 py-1.5 text-sm outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-600/20";

export function QuoteRequest() {
  return (
    <RequireAuth>
      {(user) => (user.role === "pharmacy" ? <Inner /> : <p className="text-sm text-slate-600">This page is for pharmacy accounts.</p>)}
    </RequireAuth>
  );
}

// Start the quote from my earlier quote, else from what the patient asked for.
function initialRows(r: MedicineRequest): Row[] {
  if (r.my_quote?.items?.length) {
    return r.my_quote.items.map((i) => ({
      name: i.name,
      strength: i.strength ?? "",
      quantity: i.quantity ?? "",
      available: i.available,
      price: i.available ? String((i.price_paisa ?? 0) / 100) : "",
    }));
  }
  const asked = r.prescription?.medicines?.length
    ? r.prescription.medicines.map((m) => ({ name: m.name, strength: m.strength ?? "", quantity: m.duration ? `for ${m.duration}` : "" }))
    : (r.items ?? []).map((i) => ({ name: i.name, strength: i.strength ?? "", quantity: i.quantity ?? "" }));
  const rows = asked.map((a) => ({ ...a, available: true, price: "" }));
  return rows.length ? rows : [{ name: "", strength: "", quantity: "", available: true, price: "" }];
}

function Inner() {
  const { pharmacyID, requestID } = useParams<{ pharmacyID: string; requestID: string }>();
  const [req, setReq] = useState<MedicineRequest | null>(null);
  const [rows, setRows] = useState<Row[]>([]);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  const reload = useCallback(async () => {
    const { data, error, response } = await api.GET("/api/v1/manage/pharmacies/{pharmacyID}/requests/{requestID}", {
      params: { path: { pharmacyID, requestID } },
    });
    if (data) {
      setReq(data);
      setRows(initialRows(data));
    } else setError(loadFailure(response.status, error, "request"));
  }, [pharmacyID, requestID]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    reload();
  }, [reload]);

  function update(i: number, patch: Partial<Row>) {
    setRows((prev) => prev.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  }

  const items: QuotedItem[] = rows
    .filter((r) => r.name.trim())
    .map((r) => ({
      name: r.name.trim(),
      strength: r.strength.trim() || undefined,
      quantity: r.quantity.trim() || undefined,
      available: r.available,
      price_paisa: r.available ? Math.round(Number(r.price || 0) * 100) : 0,
    }));
  const medicinesTotal = items.reduce((s, i) => s + i.price_paisa, 0);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    if (items.length === 0) {
      setError("Add at least one medicine.");
      return;
    }
    if (items.some((i) => i.available && i.price_paisa <= 0)) {
      setError("Give a price for every available medicine, or untick it.");
      return;
    }
    setBusy(true);
    setError("");
    setNotice("");
    const { data, error } = await api.POST("/api/v1/manage/pharmacies/{pharmacyID}/requests/{requestID}/quote", {
      params: { path: { pharmacyID, requestID } },
      body: {
        items,
        delivery_charge_paisa: req?.fulfilment === "pickup" ? 0 : Math.round(Number(f.get("delivery") || 0) * 100),
        eta_minutes: Number(f.get("eta")),
        note: String(f.get("note") ?? "").trim() || undefined,
      },
    });
    setBusy(false);
    if (!data) {
      setError(errorMessage(error));
      return;
    }
    setNotice(`Quote sent: ${taka(data.total_paisa)}. The patient sees it now.`);
    reload();
  }

  async function withdraw() {
    const quoteID = req?.my_quote?.id;
    if (!quoteID || !confirm("Withdraw your quote?")) return;
    const { error } = await api.POST("/api/v1/manage/pharmacies/{pharmacyID}/quotes/{quoteID}/withdraw", {
      params: { path: { pharmacyID, quoteID } },
    });
    if (error) setError(errorMessage(error));
    else reload();
  }

  if (!req) return error ? <LoadFailure failure={error} backHref={`/manage/pharmacies/${pharmacyID}`} backLabel="Back to requests" /> : <Spinner />;
  const mine = req.my_quote;
  const canQuote = req.status === "open" && (!mine || mine.status === "pending" || mine.status === "withdrawn");

  return (
    <div className="space-y-4">
      <Link href={`/manage/pharmacies/${pharmacyID}`} className="text-sm text-teal-800 hover:underline">
        ← Requests
      </Link>

      <Card className="space-y-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <h1 className="text-xl font-semibold text-slate-900">
            {req.fulfilment === "pickup" ? "Pickup" : "Delivery"} request · {km(req.distance_km)} away
          </h1>
          <Badge tone={req.status === "open" ? "amber" : "slate"}>{req.status}</Badge>
        </div>
        <p className="text-xs text-slate-500">
          {dateTime(req.created_at)} · open until {dateTime(req.expires_at)} · {req.quote_count ?? 0} quotes. The patient&apos;s identity is
          shared only when they choose you.
        </p>
        {req.prescription && (
          <div className="rounded-lg bg-slate-50 p-3 text-sm">
            <p className="font-medium text-slate-900">
              Prescription {req.prescription.code} · {req.prescription.doctor_name} (BMDC {req.prescription.doctor_bmdc})
              {req.prescription.status === "revoked" && <span className="text-red-700"> — REVOKED</span>}
            </p>
            <ul className="mt-1 list-inside list-disc text-slate-700">
              {(req.prescription.medicines ?? []).map((m, i) => (
                <li key={i}>
                  {[m.form, m.name, m.strength].filter(Boolean).join(" ")} — {m.dosage}, {m.duration}
                </li>
              ))}
            </ul>
          </div>
        )}
        {req.has_attachment && (
          <a
            href={`/api/v1/manage/pharmacies/${pharmacyID}/requests/${req.id}/attachment`}
            className="inline-block text-sm text-teal-800 underline"
          >
            📎 Open the uploaded prescription
          </a>
        )}
        {(req.items ?? []).length > 0 && (
          <ul className="list-inside list-disc text-sm text-slate-700">
            {req.items!.map((i, n) => (
              <li key={n}>{[i.name, i.strength, i.quantity].filter(Boolean).join(" · ")}</li>
            ))}
          </ul>
        )}
        {req.note && <p className="text-sm text-slate-700">Patient&apos;s note: “{req.note}”</p>}
      </Card>

      {notice && <Alert kind="success">{notice}</Alert>}
      {mine && mine.status !== "withdrawn" && (
        <Alert kind={mine.status === "accepted" ? "success" : "info"}>
          Your quote: {taka(mine.total_paisa)} · {mine.status}
          {mine.status === "pending" && (
            <>
              {" · "}
              <button onClick={withdraw} className="font-medium underline">
                Withdraw
              </button>
            </>
          )}
        </Alert>
      )}

      {canQuote && (
        <Card>
          <form onSubmit={submit} className="space-y-4">
            <h2 className="font-semibold text-slate-900">{mine && mine.status === "pending" ? "Revise your quote" : "Send a quote"}</h2>
            <div className="space-y-2">
              <div className="hidden grid-cols-[1fr_6rem_7rem_5rem_7rem_auto] gap-2 text-xs text-slate-500 sm:grid">
                <span>Medicine</span>
                <span>Strength</span>
                <span>Quantity</span>
                <span>In stock</span>
                <span>Price (৳, total)</span>
                <span />
              </div>
              {rows.map((r, i) => (
                <div key={i} className="grid grid-cols-2 gap-2 sm:grid-cols-[1fr_6rem_7rem_5rem_7rem_auto]">
                  <input aria-label="Medicine" value={r.name} onChange={(e) => update(i, { name: e.target.value })} className={`${input} col-span-2 sm:col-span-1`} />
                  <input aria-label="Strength" value={r.strength} onChange={(e) => update(i, { strength: e.target.value })} className={input} />
                  <input aria-label="Quantity" placeholder="e.g. 10 tablets" value={r.quantity} onChange={(e) => update(i, { quantity: e.target.value })} className={input} />
                  <label className="flex items-center gap-1 text-sm">
                    <input type="checkbox" checked={r.available} onChange={(e) => update(i, { available: e.target.checked })} className="accent-teal-700" />
                    <span className="sm:hidden">In stock</span>
                  </label>
                  <input
                    aria-label="Price in taka"
                    type="number"
                    min={0}
                    step="0.01"
                    disabled={!r.available}
                    value={r.price}
                    onChange={(e) => update(i, { price: e.target.value })}
                    className={`${input} disabled:bg-slate-100`}
                  />
                  <button type="button" onClick={() => setRows((p) => p.filter((_, j) => j !== i))} className="px-2 text-sm text-red-700" aria-label="Remove">
                    ✕
                  </button>
                </div>
              ))}
              <button
                type="button"
                onClick={() => setRows((p) => [...p, { name: "", strength: "", quantity: "", available: true, price: "" }])}
                className="text-sm text-teal-800 hover:underline"
              >
                + Add medicine
              </button>
            </div>
            <div className="grid gap-3 sm:grid-cols-3">
              {req.fulfilment !== "pickup" && (
                <Field
                  label="Delivery charge (৳)"
                  name="delivery"
                  type="number"
                  min={0}
                  step={1}
                  defaultValue={mine ? (mine.delivery_charge_paisa ?? 0) / 100 : 50}
                />
              )}
              <Field label="Ready in (minutes)" name="eta" type="number" min={5} max={1440} required defaultValue={mine?.eta_minutes ?? 45} />
            </div>
            <Field label="Note (optional)" name="note" maxLength={500} defaultValue={mine?.note ?? ""} placeholder="e.g. Brand substitute: Ace instead of Napa" />
            <p className="text-sm text-slate-700">
              Medicines total: <span className="font-semibold">{taka(medicinesTotal)}</span>
            </p>
            {error && <Alert>{error}</Alert>}
            <Button type="submit" disabled={busy}>
              {busy ? "Sending…" : mine && mine.status === "pending" ? "Update quote" : "Send quote"}
            </Button>
          </form>
        </Card>
      )}
      {!canQuote && error && <Alert>{error}</Alert>}
    </div>
  );
}
