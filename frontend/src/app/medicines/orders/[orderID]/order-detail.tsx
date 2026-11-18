"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { isActiveOrder, OrderBadge, OrderTimeline } from "@/components/order-status";
import { RequireAuth } from "@/components/require-auth";
import { TextArea } from "@/components/textarea";
import { Alert, Button, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type Order } from "@/lib/api/client";
import { dateTime, taka } from "@/lib/format";
import { mapLink } from "@/lib/geo";

const POLL_MS = 10000;
// The patient may cancel until the pharmacy has the order ready.
const CANCELLABLE = new Set(["confirmed", "preparing"]);

export function OrderDetail() {
  return <RequireAuth>{() => <Detail />}</RequireAuth>;
}

function Detail() {
  const { orderID } = useParams<{ orderID: string }>();
  const [order, setOrder] = useState<Order | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [reviewed, setReviewed] = useState(false);

  const reload = useCallback(async () => {
    const { data, error, response } = await api.GET("/api/v1/medicine-orders/{orderID}", { params: { path: { orderID } } });
    if (data) setOrder(data);
    else setError(response.status === 404 ? "This order could not be found." : errorMessage(error));
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
    if (!confirm("Cancel this order? The pharmacy will be told.")) return;
    setBusy(true);
    const { data, error } = await api.POST("/api/v1/medicine-orders/{orderID}/cancel", {
      params: { path: { orderID } },
      body: {},
    });
    setBusy(false);
    if (data) setOrder(data);
    else setError(errorMessage(error));
  }

  if (!order) return error ? <Alert>{error}</Alert> : <Spinner />;

  const riderMap = mapLink(order.rider_location?.latitude, order.rider_location?.longitude);
  const showCode = !!order.delivery_code && active;

  return (
    <div className="space-y-4">
      <Link href="/medicines" className="text-sm text-teal-800 hover:underline">
        ← Medicines
      </Link>

      <Card className="space-y-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h1 className="text-xl font-semibold text-slate-900">{order.pharmacy_name}</h1>
            <p className="text-sm text-slate-600">
              {order.pharmacy_address} · {order.pharmacy_phone}
            </p>
          </div>
          <OrderBadge order={order} />
        </div>

        {showCode && (
          <div className="rounded-lg border border-teal-200 bg-teal-50 p-4 text-center">
            <p className="text-sm text-teal-900">
              Give this code to the {order.fulfilment === "pickup" ? "pharmacy when you collect" : "person delivering"} — only
              after you have the medicines.
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
                  Where is the rider?
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
              <span>Delivery</span>
              <span>{taka(order.delivery_charge_paisa)}</span>
            </li>
          )}
          <li className="flex justify-between border-t border-slate-100 pt-2 font-semibold text-slate-900">
            <span>Total, cash on {order.fulfilment === "pickup" ? "pickup" : "delivery"}</span>
            <span>{taka(order.total_paisa)}</span>
          </li>
        </ul>

        {order.status === "cancelled" && (
          <Alert>
            Cancelled by {order.cancelled_by === "patient" ? "you" : "the pharmacy"}
            {order.cancel_reason ? `: ${order.cancel_reason}` : "."}
          </Alert>
        )}
        {error && <Alert>{error}</Alert>}
        {CANCELLABLE.has(order.status ?? "") && (
          <Button variant="danger" onClick={cancel} disabled={busy}>
            Cancel order
          </Button>
        )}
      </Card>

      {order.status === "delivered" && !order.reviewed && !reviewed && (
        <ReviewForm orderID={orderID} onDone={() => setReviewed(true)} />
      )}
      {(reviewed || (order.status === "delivered" && order.reviewed)) && <Alert kind="success">Thanks for rating the pharmacy.</Alert>}

      <Card>
        <h2 className="mb-3 font-semibold text-slate-900">Progress</h2>
        <OrderTimeline order={order} />
      </Card>
    </div>
  );
}

function ReviewForm({ orderID, onDone }: { orderID: string; onDone: () => void }) {
  const [rating, setRating] = useState(0);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!rating) {
      setError("Pick 1 to 5 stars.");
      return;
    }
    setBusy(true);
    const { error } = await api.POST("/api/v1/medicine-orders/{orderID}/review", {
      params: { path: { orderID } },
      body: { rating, comment: String(new FormData(e.currentTarget).get("comment")).trim() || undefined },
    });
    setBusy(false);
    if (error) setError(errorMessage(error));
    else onDone();
  }

  return (
    <Card>
      <form onSubmit={submit} className="space-y-3">
        <h2 className="font-semibold text-slate-900">Rate the pharmacy</h2>
        <div className="flex gap-1" role="radiogroup" aria-label="Rating">
          {[1, 2, 3, 4, 5].map((n) => (
            <button
              type="button"
              key={n}
              role="radio"
              aria-checked={rating === n}
              aria-label={`${n} star${n > 1 ? "s" : ""}`}
              onClick={() => setRating(n)}
              className={`text-3xl ${n <= rating ? "text-amber-500" : "text-slate-300"}`}
            >
              ★
            </button>
          ))}
        </div>
        <TextArea label="Comment (optional)" name="comment" rows={2} maxLength={1000} />
        {error && <Alert>{error}</Alert>}
        <Button type="submit" disabled={busy}>
          {busy ? "Saving…" : "Submit rating"}
        </Button>
      </form>
    </Card>
  );
}
