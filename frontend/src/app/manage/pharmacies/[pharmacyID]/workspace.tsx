"use client";

import Link from "next/link";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import { OrderBadge } from "@/components/order-status";
import { RequireAuth } from "@/components/require-auth";
import { LoadFailure, loadFailure } from "@/components/load-failure";
import { Alert, Badge, Button, Card, Field, Spinner } from "@/components/ui";
import { api, errorMessage, type MedicineRequest, type Order, type Pharmacy } from "@/lib/api/client";
import { dateTime, taka, todayInDhaka } from "@/lib/format";
import { km } from "@/lib/geo";
import { VERIFICATION } from "@/app/manage/hospitals/my-hospitals";
import { PharmacyForm } from "../pharmacy-form";

const TABS = [
  { id: "requests", label: "New requests" },
  { id: "orders", label: "Orders" },
  { id: "profile", label: "Profile" },
  { id: "statement", label: "Statement" },
] as const;
type Tab = (typeof TABS)[number]["id"];

const POLL_MS = 20000;

export function PharmacyWorkspace() {
  return (
    <RequireAuth>
      {(user) => (user.role === "pharmacy" ? <Inner /> : <p className="text-sm text-slate-600">This page is for pharmacy accounts.</p>)}
    </RequireAuth>
  );
}

function Inner() {
  const { pharmacyID } = useParams<{ pharmacyID: string }>();
  const params = useSearchParams();
  const router = useRouter();
  const tab = (TABS.find((t) => t.id === params.get("tab"))?.id ?? "requests") as Tab;
  const [pharmacy, setPharmacy] = useState<Pharmacy | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    api.GET("/api/v1/manage/pharmacies/{pharmacyID}", { params: { path: { pharmacyID } } }).then(({ data, error, response }) => {
      if (data) setPharmacy(data);
      else setError(loadFailure(response.status, error, "pharmacy"));
    });
  }, [pharmacyID]);

  if (error) return <LoadFailure failure={error} backHref="/manage/pharmacies" backLabel="My pharmacies" />;
  if (!pharmacy) return <Spinner />;
  const v = VERIFICATION[pharmacy.verification_status ?? "pending"];

  return (
    <div className="space-y-6">
      <Link href="/manage/pharmacies" className="text-sm text-teal-800 hover:underline">
        ← My pharmacies
      </Link>
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="text-2xl font-semibold text-slate-900">{pharmacy.name}</h1>
        <Badge tone={v.tone}>{v.label}</Badge>
        {pharmacy.accepting_orders === false && <Badge tone="amber">Not taking orders</Badge>}
      </div>
      {pharmacy.verification_status === "pending" && (
        <Alert kind="info">An admin is checking your drug licence. Requests from patients appear here once you are verified.</Alert>
      )}
      {pharmacy.verification_status === "rejected" && pharmacy.verification_note && (
        <Alert>Verification was rejected: {pharmacy.verification_note}. Correct the profile and save it to ask again.</Alert>
      )}

      <div className="flex gap-1 overflow-x-auto border-b border-slate-200" role="tablist">
        {TABS.map((t) => (
          <button
            key={t.id}
            role="tab"
            aria-selected={tab === t.id}
            onClick={() => router.replace(`/manage/pharmacies/${pharmacyID}?tab=${t.id}`, { scroll: false })}
            className={`whitespace-nowrap border-b-2 px-4 py-2 text-sm font-medium ${
              tab === t.id ? "border-teal-700 text-teal-800" : "border-transparent text-slate-600 hover:text-slate-900"
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === "requests" && <RequestsTab pharmacyID={pharmacyID} />}
      {tab === "orders" && <OrdersTab pharmacyID={pharmacyID} />}
      {tab === "statement" && <StatementTab pharmacyID={pharmacyID} />}
      {tab === "profile" && (
        <Card>
          <PharmacyForm
            initial={pharmacy}
            submitLabel="Save"
            onSave={async (input) => {
              const { data, error } = await api.PUT("/api/v1/manage/pharmacies/{pharmacyID}", {
                params: { path: { pharmacyID } },
                body: input,
              });
              if (!data) return errorMessage(error);
              setPharmacy(data);
              return "";
            }}
          />
        </Card>
      )}
    </div>
  );
}

function RequestsTab({ pharmacyID }: { pharmacyID: string }) {
  const [items, setItems] = useState<MedicineRequest[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let stopped = false;
    async function load() {
      const { data, error } = await api.GET("/api/v1/manage/pharmacies/{pharmacyID}/requests", {
        params: { path: { pharmacyID }, query: { limit: 50 } },
      });
      if (stopped) return;
      if (data) setItems(data.requests ?? []);
      else setError(errorMessage(error));
    }
    load();
    const t = setInterval(load, POLL_MS);
    return () => {
      stopped = true;
      clearInterval(t);
    };
  }, [pharmacyID]);

  if (!items) return error ? <Alert>{error}</Alert> : <Spinner />;
  if (items.length === 0) return <p className="text-sm text-slate-600">No open requests near you right now. This list refreshes by itself.</p>;

  return (
    <ul className="space-y-3">
      {items.map((r) => (
        <li key={r.id}>
          <Link
            href={`/manage/pharmacies/${pharmacyID}/requests/${r.id}`}
            className="block rounded-xl border border-slate-200 bg-surface p-4 shadow-sm transition hover:border-teal-600"
          >
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <p className="font-semibold text-slate-900">
                  {r.prescription
                    ? `Prescription · ${(r.prescription.medicines ?? []).length} medicines`
                    : r.has_attachment
                      ? "Paper prescription"
                      : `${(r.items ?? []).length} medicines`}
                </p>
                <p className="text-sm text-slate-600">
                  {r.fulfilment === "pickup" ? "Pickup" : "Delivery"} · {km(r.distance_km)} away · {r.quote_count ?? 0} quotes so far
                </p>
              </div>
              {r.my_quote ? <Badge tone="teal">Quoted {taka(r.my_quote.total_paisa)}</Badge> : <Badge tone="amber">Not quoted</Badge>}
            </div>
            <p className="mt-1 text-xs text-slate-500">
              {dateTime(r.created_at)} · open until {dateTime(r.expires_at)}
            </p>
          </Link>
        </li>
      ))}
    </ul>
  );
}

const ORDER_FILTERS = [
  { id: "active", label: "In progress" },
  { id: "delivered", label: "Delivered" },
  { id: "cancelled", label: "Cancelled" },
] as const;

function OrdersTab({ pharmacyID }: { pharmacyID: string }) {
  const [status, setStatus] = useState<(typeof ORDER_FILTERS)[number]["id"]>("active");
  const [items, setItems] = useState<Order[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let stopped = false;
    async function load() {
      const { data, error } = await api.GET("/api/v1/manage/pharmacies/{pharmacyID}/orders", {
        params: { path: { pharmacyID }, query: { status, limit: 50 } },
      });
      if (stopped) return;
      if (data) setItems(data.orders ?? []);
      else setError(errorMessage(error));
    }
    load();
    const t = status === "active" ? setInterval(load, POLL_MS) : undefined;
    return () => {
      stopped = true;
      clearInterval(t);
    };
  }, [pharmacyID, status]);

  return (
    <div className="space-y-4">
      <div className="flex gap-2">
        {ORDER_FILTERS.map((f) => (
          <button
            key={f.id}
            onClick={() => {
              setItems(null);
              setStatus(f.id);
            }}
            className={`rounded-full px-3 py-1 text-sm ${status === f.id ? "bg-teal-700 text-white" : "bg-surface text-slate-700 ring-1 ring-slate-200"}`}
          >
            {f.label}
          </button>
        ))}
      </div>
      {error && <Alert>{error}</Alert>}
      {items === null ? (
        !error && <Spinner />
      ) : items.length === 0 ? (
        <p className="text-sm text-slate-600">No orders here.</p>
      ) : (
        <ul className="space-y-3">
          {items.map((o) => (
            <li key={o.id}>
              <Link
                href={`/manage/pharmacies/${pharmacyID}/orders/${o.id}`}
                className="block rounded-xl border border-slate-200 bg-surface p-4 shadow-sm transition hover:border-teal-600"
              >
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <p className="font-semibold text-slate-900">{o.patient_name}</p>
                    <p className="text-sm text-slate-600">
                      {o.fulfilment === "pickup" ? "Pickup" : "Delivery"} · {taka(o.total_paisa)}
                    </p>
                  </div>
                  <OrderBadge order={o} />
                </div>
                <p className="mt-1 text-xs text-slate-500">{dateTime(o.created_at)}</p>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function StatementTab({ pharmacyID }: { pharmacyID: string }) {
  const today = todayInDhaka();
  const [from, setFrom] = useState(`${today.slice(0, 8)}01`);
  const [to, setTo] = useState(today);
  const [result, setResult] = useState<{ delivered_orders?: number; medicines_total_paisa?: number; delivery_total_paisa?: number; commission_due_paisa?: number } | null>(null);
  const [error, setError] = useState("");

  async function load() {
    setError("");
    const { data, error } = await api.GET("/api/v1/manage/pharmacies/{pharmacyID}/statement", {
      params: { path: { pharmacyID }, query: { from, to } },
    });
    if (data) setResult(data);
    else setError(errorMessage(error));
  }

  return (
    <Card className="space-y-4">
      <div className="flex flex-wrap items-end gap-3">
        <Field label="From" type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
        <Field label="To" type="date" value={to} onChange={(e) => setTo(e.target.value)} />
        <Button onClick={load}>Show</Button>
      </div>
      {error && <Alert>{error}</Alert>}
      {result && (
        <dl className="grid gap-3 sm:grid-cols-4">
          {[
            ["Delivered orders", String(result.delivered_orders ?? 0)],
            ["Medicine sales", taka(result.medicines_total_paisa)],
            ["Delivery charges", taka(result.delivery_total_paisa)],
            ["Platform commission due", taka(result.commission_due_paisa)],
          ].map(([label, value]) => (
            <div key={label} className="rounded-lg bg-slate-50 p-3">
              <dt className="text-xs text-slate-500">{label}</dt>
              <dd className="text-lg font-semibold text-slate-900">{value}</dd>
            </div>
          ))}
        </dl>
      )}
    </Card>
  );
}
