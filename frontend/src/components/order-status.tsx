import type { Order } from "@/lib/api/client";
import { dateTime } from "@/lib/format";
import { Badge } from "./ui";

const LABELS: Record<string, { label: string; tone: "slate" | "teal" | "amber" | "red" }> = {
  confirmed: { label: "Order placed", tone: "amber" },
  preparing: { label: "Being prepared", tone: "amber" },
  ready: { label: "Ready", tone: "teal" },
  rider_assigned: { label: "Rider on the way to pharmacy", tone: "teal" },
  out_for_delivery: { label: "Out for delivery", tone: "teal" },
  delivered: { label: "Delivered", tone: "slate" },
  cancelled: { label: "Cancelled", tone: "red" },
  // events also use these
  dispatched: { label: "Sent out", tone: "teal" },
  picked_up: { label: "Picked up by rider", tone: "teal" },
};

export function orderStatusLabel(status: string | undefined, fulfilment?: string): string {
  if (status === "ready" && fulfilment === "pickup") return "Ready for pickup";
  return LABELS[status ?? ""]?.label ?? status ?? "";
}

export function OrderBadge({ order }: { order: Order }) {
  const s = LABELS[order.status ?? ""] ?? { tone: "slate" as const };
  return <Badge tone={s.tone}>{orderStatusLabel(order.status, order.fulfilment)}</Badge>;
}

export function OrderTimeline({ order }: { order: Order }) {
  const events = order.events ?? [];
  if (events.length === 0) return null;
  return (
    <ol className="relative space-y-3 border-l border-slate-200 pl-4">
      {events.map((e, i) => (
        <li key={i} className="text-sm">
          <span className="absolute -left-1.5 mt-1.5 h-3 w-3 rounded-full border-2 border-white bg-teal-700" aria-hidden />
          <p className="font-medium text-slate-900">{orderStatusLabel(e.status, order.fulfilment)}</p>
          {e.note && <p className="text-slate-600">{e.note}</p>}
          <p className="text-xs text-slate-500">{dateTime(e.created_at)}</p>
        </li>
      ))}
    </ol>
  );
}

// Unfinished orders change on someone else's action; pages poll them.
export function isActiveOrder(order: Order | null): boolean {
  return !!order && order.status !== "delivered" && order.status !== "cancelled";
}
