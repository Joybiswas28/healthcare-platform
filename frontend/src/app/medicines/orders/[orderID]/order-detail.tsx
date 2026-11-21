"use client";

import { MedicinesFor } from "@/components/family";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { isActiveOrder, OrderBadge, OrderTimeline } from "@/components/order-status";
import { RequireAuth } from "@/components/require-auth";
import { RatingForm } from "@/components/rating";
import { LoadFailure, loadFailure } from "@/components/load-failure";
import { Alert, Button, Card, Spinner } from "@/components/ui";
import { useT } from "@/components/use-lang";
import { api, errorMessage, type Order } from "@/lib/api/client";
import { dateTime, taka } from "@/lib/format";
import { mapLink } from "@/lib/geo";
import { T } from "@/components/t";

const POLL_MS = 10000;
// The patient may cancel until the pharmacy has the order ready.
const CANCELLABLE = new Set(["confirmed", "preparing"]);

export function OrderDetail() {
  return <RequireAuth>{() => <Detail />}</RequireAuth>;
}

function Detail() {
  const t = useT();
  const { orderID } = useParams<{ orderID: string }>();
  const [order, setOrder] = useState<Order | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [reviewed, setReviewed] = useState(false);

  const reload = useCallback(async () => {
    const { data, error, response } = await api.GET("/api/v1/medicine-orders/{orderID}", { params: { path: { orderID } } });
    if (data) setOrder(data);
    else setError(loadFailure(response.status, error, "order"));
  }, [orderID]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    reload();
  }, [reload]);

  const active = isActiveOrder(order);
  useEffect(() => {
    if (!active) return;
    const t = setInterval(reload, POLL_MS);
    return () => clearInterval(t);
  }, [active, reload]);

  async function cancel() {
    if (!confirm(t("Cancel this order? The pharmacy will be told."))) return;
    setBusy(true);
    const { data, error } = await api.POST("/api/v1/medicine-orders/{orderID}/cancel", {
      params: { path: { orderID } },
      body: {},
    });
    setBusy(false);
    if (data) setOrder(data);
    else setError(errorMessage(error));
  }

  if (!order) return error ? <LoadFailure failure={error} backHref="/medicines" backLabel="Back to medicines" /> : <Spinner />;

  const riderMap = mapLink(order.rider_location?.latitude, order.rider_location?.longitude);
  const showCode = !!order.delivery_code && active;

  return (
    <div className="space-y-4">
      <Link href="/medicines" className="text-sm text-teal-800 hover:underline">
        ← <T>Medicines</T>
      </Link>

      <Card className="space-y-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h1 className="text-xl font-semibold text-slate-900">{order.pharmacy_name}</h1>
            <p className="text-sm text-slate-600">
              {order.pharmacy_address} · {order.pharmacy_phone}
            </p>
            <MedicinesFor name={order.for_name} age={order.patient_age} gender={order.patient_gender} />
          </div>
          <OrderBadge order={order} />
        </div>

        {showCode && (
          <div className="rounded-lg border border-teal-200 bg-teal-50 p-4 text-center">
            <p className="text-sm text-teal-900">
              {t(order.fulfilment === "pickup" ? "Give this code to the pharmacy when you collect — only after you have the medicines." : "Give this code to the person delivering — only after you have the medicines.")}
            </p>
            <p className="mt-1 font-mono text-3xl font-semibold tracking-[0.3em] text-teal-900">{order.delivery_code}</p>
          </div>
        )}

        {order.rider_name && active && (
          <p className="text-sm text-slate-700">
            Rider: <span className="font-medium">{order.rider_name}</span>
            {order.rider_phone && (
              <>
                {" · "}
                <a href={`tel:${order.rider_phone}`} className="text-teal-800 underline">
                  {order.rider_phone}
                </a>
              </>
            )}
            {riderMap && (
              <>
                {" · "}
                <a href={riderMap} target="_blank" rel="noopener noreferrer" className="text-teal-800 underline">
                  <T>Where is the rider?</T>
                </a>{" "}
                <span className="text-xs text-slate-500">(updated {dateTime(order.rider_location?.at)})</span>
              </>
            )}
          </p>
        )}

        <ul className="space-y-1 border-t border-slate-100 pt-3 text-sm">
          {(order.items ?? [])
            .filter((i) => i.available)
            .map((i, n) => (
              <li key={n} className="flex justify-between gap-3 text-slate-700">
                <span>{[i.name, i.strength, i.quantity].filter(Boolean).join(" · ")}</span>
                <span>{taka(i.price_paisa)}</span>
              </li>
            ))}
          {(order.delivery_charge_paisa ?? 0) > 0 && (
            <li className="flex justify-between text-slate-600">
              <span><T>Delivery</T></span>
              <span>{taka(order.delivery_charge_paisa)}</span>
            </li>
          )}
          <li className="flex justify-between border-t border-slate-100 pt-2 font-semibold text-slate-900">
            <span>{t(order.fulfilment === "pickup" ? "Total, cash on pickup" : "Total, cash on delivery")}</span>
            <span>{taka(order.total_paisa)}</span>
          </li>
        </ul>

        {order.status === "cancelled" && (
          <Alert>
            {t(order.cancelled_by === "patient" ? "Cancelled by you" : "Cancelled by the pharmacy")}
            {order.cancel_reason ? `: ${order.cancel_reason}` : "."}
          </Alert>
        )}
        {error && <Alert>{error}</Alert>}
        {CANCELLABLE.has(order.status ?? "") && (
          <Button variant="danger" onClick={cancel} disabled={busy}>
            <T>Cancel order</T>
          </Button>
        )}
      </Card>

      {order.status === "delivered" && !order.reviewed && !reviewed && (
        <ReviewForm orderID={orderID} onDone={() => setReviewed(true)} />
      )}
      {(reviewed || (order.status === "delivered" && order.reviewed)) && <Alert kind="success">Thanks for rating the pharmacy.</Alert>}

      <Card>
        <h2 className="mb-3 font-semibold text-slate-900"><T>Progress</T></h2>
        <OrderTimeline order={order} />
      </Card>
    </div>
  );
}

function ReviewForm({ orderID, onDone }: { orderID: string; onDone: () => void }) {
  async function send(rating: number, comment: string | undefined): Promise<string> {
    const { error } = await api.POST("/api/v1/medicine-orders/{orderID}/review", {
      params: { path: { orderID } },
      body: { rating, comment },
    });
    if (error) return errorMessage(error);
    onDone();
    return "";
  }

  return (
    <Card>
      <RatingForm title="Rate the pharmacy" send={send} />
    </Card>
  );
}
