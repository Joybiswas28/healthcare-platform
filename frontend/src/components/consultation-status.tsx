import type { ConsultationStatus } from "@/lib/api/client";
import { Badge } from "./ui";

const LABELS: Record<ConsultationStatus, { label: string; tone: "slate" | "teal" | "amber" | "red" }> = {
  requested: { label: "Waiting for doctor", tone: "amber" },
  accepted: { label: "Accepted — pay to start", tone: "amber" },
  rejected: { label: "Declined", tone: "red" },
  active: { label: "Active", tone: "teal" },
  completed: { label: "Completed", tone: "slate" },
  cancelled: { label: "Cancelled", tone: "red" },
};

// The doctor sees the same states from the other side.
const DOCTOR_LABELS: Partial<Record<ConsultationStatus, string>> = {
  requested: "New request",
  accepted: "Waiting for payment",
};

export function ConsultationBadge({ status, asDoctor }: { status?: ConsultationStatus; asDoctor?: boolean }) {
  if (!status) return null;
  const s = LABELS[status];
  return <Badge tone={s.tone}>{(asDoctor && DOCTOR_LABELS[status]) || s.label}</Badge>;
}
