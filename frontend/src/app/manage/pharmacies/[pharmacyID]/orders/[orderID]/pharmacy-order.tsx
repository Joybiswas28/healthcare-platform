"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { DeliveryCodeForm } from "@/components/delivery-code-form";
import { isActiveOrder, OrderBadge, OrderTimeline } from "@/components/order-status";
import { RequireAuth } from "@/components/require-auth";
import { TextArea } from "@/components/textarea";
import { Alert, Button, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type Order } from "@/lib/api/client";
import { taka } from "@/lib/format";
import { mapLink } from "@/lib/geo";

const POLL_MS = 15000;

export function PharmacyOrder() {
  return (
    <RequireAuth>
      {(user) => (user.role === "pharmacy" ? <Inner /> : <p className="text-sm text-slate-600">This page is for pharmacy accounts.</p>)}
    </RequireAuth>
  );
}

function Inner() {
  const { pharmacyID, orderID } = useParams<{ pharmacyID: string; orderID: string }>();
  const [order, setOrder] = useState<Order | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const path = { pharmacyID, orderID };

  const reload = useCallback(async () => {
    const { data, error, response } = await api.GET("/api/v1/manage/pharmacies/{pharmacyID}/orders/{orderID}", {
      params: { path: { pharmacyID, orderID } },
    });
    if (data) setOrder(data);
    else setError(response.status === 404 ? "This order could not be found." : errorMessage(error));
  }, [pharmacyID, orderID]);

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

  async function step(kind: "preparing" | "ready") {
    setBusy(true);
    setError("");
    const url =
      kind === "preparing"
        ? "/api/v1/manage/pharmacies/{pharmacyID}/orders/{orderID}/preparing"
        : "/api/v1/manage/pharmacies/{pharmacyID}/orders/{orderID}/ready";
    const { data, error } = await api.POST(url, { params: { path } });
    setBusy(false);
    if (data) setOrder(data);
    else setError(errorMessage(error));
  }

  async function dispatch(mode: "pharmacy" | "rider") {
    const question =
      mode === "rider"
        ? "Offer this delivery to platform riders nearby? A rider collects it from you."
        : "Send it with your own delivery person now?";
    if (!confirm(question)) return;
    setBusy(true);
    setError("");
    const { data, error } = await api.POST("/api/v1/manage/pharmacies/{pharmacyID}/orders/{orderID}/dispatch", {
      params: { path },
      body: { mode },
    });
    setBusy(false);
    if (data) setOrder(data);
    else setError(errorMessage(error));
  }

  async function complete(code: string) {
    const { data, error } = await api.POST("/api/v1/manage/pharmacies/{pharmacyID}/orders/{orderID}/complete", {
      params: { path },
      body: { code },
    });
    if (!data) return errorMessage(error);
    setOrder(data);
    return "";
  }

  async function cancel(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    const { data, error } = await api.POST("/api/v1/manage/pharmacies/{pharmacyID}/orders/{orderID}/cancel", {
      params: { path },
      body: { reason: String(new FormData(e.currentTarget).get("reason")).trim() },
    });
    setBusy(false);
    if (data) {
      setOrder(data);
      setCancelling(false);
    } else setError(errorMessage(error));
  }

  if (!order) return error ? <Alert>{error}</Alert> : <Spinner />;

  const s = order.status;
  const delivery = order.fulfilment === "delivery";
  const handOver = (s === "ready" && !delivery) || (s === "out_for_delivery" && order.delivery_mode === "pharmacy");
  // Same rule as the backend: until it leaves the shop (no rider assigned yet).
  const canCancel = s === "confirmed" || s === "preparing" || s === "ready";
  const map = mapLink(order.latitude, order.longitude);

  return (
    <div className="space-y-4">
      <Link href={`/manage/pharmacies/${pharmacyID}?tab=orders`} className="text-sm text-teal-800 hover:underline">
        ← Orders
      </Link>

      <Card className="space-y-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h1 className="text-xl font-semibold text-slate-900">{order.patient_name}</h1>
            <p className="text-sm text-slate-600">
              <a href={`tel:${order.contact_phone}`} className="text-teal-800 underline">
                {order.contact_phone}
              </a>
              {delivery && order.address ? ` · ${order.address}` : " · Pickup"}
              {delivery && map && (
                <>
                  {" · "}
                  <a href={map} target="_blank" rel="noopener noreferrer" className="text-teal-800 underline">
                    Map
                  </a>
                </>
              )}
            </p>
          </div>
          <OrderBadge order={order} />
        </div>

        <ul className="space-y-1 border-t border-slate-100 pt-3 text-sm">
          {(order.items ?? []).map((i, n) => (
            <li key={n} className={`flex justify-between gap-3 ${i.available ? "text-slate-700" : "text-slate-400 line-through"}`}>
              <span>{[i.name, i.strength, i.quantity].filter(Boolean).join(" · ")}</span>
              <span>{i.available ? taka(i.price_paisa) : "not available"}</span>
            </li>
          ))}
          {(order.delivery_charge_paisa ?? 0) > 0 && (
            <li className="flex justify-between text-slate-600">
              <span>Delivery</span>
              <span>{taka(order.delivery_charge_paisa)}</span>
            </li>
          )}
          <li className="flex justify-between border-t border-slate-100 pt-2 font-semibold text-slate-900">
            <span>Cash to collect</span>
            <span>{taka(order.total_paisa)}</span>
          </li>
          {order.commission_paisa != null && s === "delivered" && (
            <li className="flex justify-between text-xs text-slate-500">
              <span>Platform commission</span>
              <span>{taka(order.commission_paisa)}</span>
            </li>
          )}
        </ul>
        {order.prescription_id && (
          <p className="text-xs text-slate-500">Dispense only against the prescription shown on the request.</p>
        )}
      </Card>

      {error && <Alert>{error}</Alert>}

      {active && (
        <Card className="space-y-3">
          <h2 className="font-semibold text-slate-900">Next step</h2>
          {s === "confirmed" && (
            <Button onClick={() => step("preparing")} disabled={busy}>
              Start preparing
            </Button>
          )}
          {s === "preparing" && (
            <Button onClick={() => step("ready")} disabled={busy}>
              {delivery ? "Packed and ready" : "Ready — tell the patient to come"}
            </Button>
          )}
          {s === "ready" && delivery && !order.delivery_mode && (
            <div className="flex flex-wrap gap-2">
              <Button onClick={() => dispatch("pharmacy")} disabled={busy}>
                Send with our own staff
              </Button>
              <Button variant="secondary" onClick={() => dispatch("rider")} disabled={busy}>
                Ask a platform rider
              </Button>
            </div>
          )}
          {s === "ready" && order.delivery_mode === "rider" && <p className="text-sm text-slate-600">Waiting for a nearby rider to take the job.</p>}
          {s === "rider_assigned" && (
            <p className="text-sm text-slate-600">
              {order.rider_name} ({order.rider_phone}) is coming to collect it.
            </p>
          )}
          {s === "out_for_delivery" && order.delivery_mode === "rider" && (
            <p className="text-sm text-slate-600">{order.rider_name} is delivering it.</p>
          )}
          {handOver && <DeliveryCodeForm label={delivery ? "Mark delivered" : "Hand over"} onSubmit={complete} />}

          {canCancel &&
            (cancelling ? (
              <form onSubmit={cancel} className="space-y-2 border-t border-slate-100 pt-3">
                <TextArea label="Reason (sent to the patient by SMS)" name="reason" rows={2} minLength={5} required autoFocus />
                <div className="flex gap-2">
                  <Button type="submit" variant="danger" disabled={busy}>
                    Cancel order
                  </Button>
                  <Button type="button" variant="secondary" onClick={() => setCancelling(false)}>
                    Back
                  </Button>
                </div>
              </form>
            ) : (
              <button onClick={() => setCancelling(true)} className="block text-sm text-red-700 hover:underline">
                Cancel this order
              </button>
            ))}
        </Card>
      )}

      {s === "cancelled" && (
        <Alert>
          Cancelled by the {order.cancelled_by}
          {order.cancel_reason ? `: ${order.cancel_reason}` : "."}
        </Alert>
      )}

      <Card>
        <h2 className="mb-3 font-semibold text-slate-900">Progress</h2>
        <OrderTimeline order={order} />
      </Card>
    </div>
  );
}
