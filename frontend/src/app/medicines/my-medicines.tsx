"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { OrderBadge } from "@/components/order-status";
import { Alert, Badge, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type MedicineRequest, type Order } from "@/lib/api/client";
import { dateTime, taka } from "@/lib/format";

const REQUEST_STATUS = {
  open: { label: "Waiting for quotes", tone: "amber" },
  ordered: { label: "Ordered", tone: "teal" },
  cancelled: { label: "Cancelled", tone: "red" },
  expired: { label: "Expired", tone: "slate" },
} as const;

export function MyMedicines() {
  const [orders, setOrders] = useState<Order[] | null>(null);
  const [requests, setRequests] = useState<MedicineRequest[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    Promise.all([
      api.GET("/api/v1/medicine-orders", { params: { query: { limit: 50 } } }),
      api.GET("/api/v1/medicine-requests", { params: { query: { limit: 50 } } }),
    ]).then(([o, r]) => {
      if (o.data) setOrders(o.data.orders ?? []);
      else setError(errorMessage(o.error));
      if (r.data) setRequests(r.data.requests ?? []);
    });
  }, []);

  if (error) return <Alert>{error}</Alert>;
  if (!orders || !requests) return <Spinner />;

  const open = requests.filter((r) => r.status === "open");
  const past = requests.filter((r) => r.status !== "open" && r.status !== "ordered");

  return (
    <div className="space-y-8">
      <section className="space-y-3">
        <h2 className="text-lg font-semibold text-slate-900">Waiting for quotes</h2>
        {open.length === 0 && <p className="text-sm text-slate-600">No open requests.</p>}
        {open.map((r) => (
          <RequestRow key={r.id} request={r} />
        ))}
      </section>

      <section className="space-y-3">
        <h2 className="text-lg font-semibold text-slate-900">Orders</h2>
        {orders.length === 0 && (
          <Card>
            <p className="text-slate-600">No orders yet.</p>
          </Card>
        )}
        {orders.map((o) => (
          <Link
            key={o.id}
            href={`/medicines/orders/${o.id}`}
            className="block rounded-xl border border-slate-200 bg-surface p-4 shadow-sm transition hover:border-teal-600"
          >
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <p className="font-semibold text-slate-900">{o.pharmacy_name}</p>
                <p className="text-sm text-slate-600">
                  {(o.items ?? []).filter((i) => i.available).length} items · {taka(o.total_paisa)} cash on{" "}
                  {o.fulfilment === "pickup" ? "pickup" : "delivery"}
                </p>
              </div>
              <OrderBadge order={o} />
            </div>
            <p className="mt-1 text-xs text-slate-500">{dateTime(o.created_at)}</p>
          </Link>
        ))}
      </section>

      {past.length > 0 && (
        <details>
          <summary className="cursor-pointer text-sm text-slate-600">Cancelled and expired requests ({past.length})</summary>
          <div className="mt-3 space-y-3">
            {past.map((r) => (
              <RequestRow key={r.id} request={r} />
            ))}
          </div>
        </details>
      )}
    </div>
  );
}

function RequestRow({ request: r }: { request: MedicineRequest }) {
  const s = REQUEST_STATUS[r.status ?? "open"];
  return (
    <Link
      href={`/medicines/requests/${r.id}`}
      className="block rounded-xl border border-slate-200 bg-surface p-4 shadow-sm transition hover:border-teal-600"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="font-semibold text-slate-900">
            {r.prescription ? `Prescription by ${r.prescription.doctor_name}` : r.has_attachment ? "Paper prescription" : "Medicine list"}
          </p>
          <p className="text-sm text-slate-600">
            {r.fulfilment === "pickup" ? "Pickup" : "Delivery"} · {r.quote_count ?? 0} quote{r.quote_count === 1 ? "" : "s"}
          </p>
        </div>
        <Badge tone={s.tone}>{s.label}</Badge>
      </div>
      <p className="mt-1 text-xs text-slate-500">
        {dateTime(r.created_at)}
        {r.status === "open" ? ` · open until ${dateTime(r.expires_at)}` : ""}
      </p>
    </Link>
  );
}
