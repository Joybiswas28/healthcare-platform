"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { DeliveryCodeForm } from "@/components/delivery-code-form";
import { OrderBadge, OrderTimeline } from "@/components/order-status";
import { RequireAuth } from "@/components/require-auth";
import { LoadFailure, loadFailure } from "@/components/load-failure";
import { Alert, Button, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type Order } from "@/lib/api/client";
import { taka } from "@/lib/format";
import { mapLink } from "@/lib/geo";
import { T } from "@/components/t";
import { useT } from "@/components/use-lang";

export function RiderOrder() {
  return (
    <RequireAuth>
      {(user) => (user.role === "rider" ? <Inner /> : <p className="text-sm text-slate-600"><T>This page is for rider accounts.</T></p>)}
    </RequireAuth>
  );
}

function Inner() {
  const { orderID } = useParams<{ orderID: string }>();
  const router = useRouter();
  const t = useT();
  const [order, setOrder] = useState<Order | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const reload = useCallback(async () => {
    const { data, error, response } = await api.GET("/api/v1/rider/orders/{orderID}", { params: { path: { orderID } } });
    if (data) setOrder(data);
    else setError(loadFailure(response.status, error, "delivery"));
  }, [orderID]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    reload();
  }, [reload]);

  async function pickedUp() {
    if (!confirm(t("Did you collect the medicines from the pharmacy?"))) return;
    setBusy(true);
    const { data, error } = await api.POST("/api/v1/rider/orders/{orderID}/picked-up", { params: { path: { orderID } } });
    setBusy(false);
    if (data) setOrder(data);
    else setError(errorMessage(error));
  }

  async function release() {
    if (!confirm(t("Give this job back? Another rider can take it."))) return;
    setBusy(true);
    const { error } = await api.POST("/api/v1/rider/orders/{orderID}/release", { params: { path: { orderID } } });
    setBusy(false);
    if (error) setError(errorMessage(error));
    else router.push("/rider");
  }

  async function delivered(code: string) {
    const { data, error } = await api.POST("/api/v1/rider/orders/{orderID}/delivered", {
      params: { path: { orderID } },
      body: { code },
    });
    if (!data) return errorMessage(error);
    setOrder(data);
    return "";
  }

  if (!order) return error ? <LoadFailure failure={error} backHref="/rider" backLabel="Back to deliveries" /> : <Spinner />;

  const toPharmacy = order.status === "rider_assigned";
  const patientMap = mapLink(order.latitude, order.longitude);

  return (
    <div className="space-y-4">
      <Link href="/rider" className="text-sm text-teal-800 hover:underline">
        ← <T>Deliveries</T>
      </Link>

      <Card className="space-y-4">
        <div className="flex items-start justify-between gap-3">
          <h1 className="text-xl font-semibold text-slate-900"><T>Delivery</T></h1>
          <OrderBadge order={order} />
        </div>

        <div className={`rounded-lg border p-3 text-sm ${toPharmacy ? "border-teal-300 bg-teal-50" : "border-slate-200"}`}>
          <p className="text-xs font-medium uppercase tracking-wide text-slate-500">
            <T>1. Collect from</T>
          </p>
          <p className="font-semibold text-slate-900">{order.pharmacy_name}</p>
          <p className="text-slate-700">{order.pharmacy_address}</p>
          <a href={`tel:${order.pharmacy_phone}`} className="text-teal-800 underline">
            {order.pharmacy_phone}
          </a>
        </div>

        <div className={`rounded-lg border p-3 text-sm ${order.status === "out_for_delivery" ? "border-teal-300 bg-teal-50" : "border-slate-200"}`}>
          <p className="text-xs font-medium uppercase tracking-wide text-slate-500">
            <T>2. Deliver to</T>
          </p>
          <p className="font-semibold text-slate-900">{order.patient_name}</p>
          <p className="text-slate-700">{order.address}</p>
          <p>
            <a href={`tel:${order.contact_phone}`} className="text-teal-800 underline">
              {order.contact_phone}
            </a>
            {patientMap && (
              <>
                {" · "}
                <a href={patientMap} target="_blank" rel="noopener noreferrer" className="text-teal-800 underline">
                  <T>Open in maps</T>
                </a>
              </>
            )}
          </p>
        </div>

        <p className="text-sm text-slate-700">
          <T vars={{ amount: taka(order.total_paisa), earn: taka(order.delivery_charge_paisa) }}>
            {"Collect {amount} cash from the patient · you earn {earn}"}
          </T>
        </p>
      </Card>

      {error && <Alert>{error}</Alert>}

      {toPharmacy && (
        <Card className="flex flex-wrap gap-2">
          <Button onClick={pickedUp} disabled={busy}>
            I&apos;ve picked it up
          </Button>
          <Button variant="secondary" onClick={release} disabled={busy}>
            <T>Give the job back</T>
          </Button>
        </Card>
      )}
      {order.status === "out_for_delivery" && (
        <Card>
          <DeliveryCodeForm label="Mark delivered" onSubmit={delivered} />
        </Card>
      )}
      {order.status === "delivered" && <Alert kind="success">Delivered. Thank you!</Alert>}

      <Card>
        <h2 className="mb-3 font-semibold text-slate-900"><T>Progress</T></h2>
        <OrderTimeline order={order} />
      </Card>
    </div>
  );
}
