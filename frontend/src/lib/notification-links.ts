import type { Role } from "@/lib/api/client";
import { bnDigits } from "@/lib/i18n";

// Notifications carry a kind but no link to the exact record, so clicking one
// opens the page where that kind of thing lives for the reader's role.
export function notificationHref(kind: string | undefined, role: Role | undefined): string {
  switch (kind) {
    case "appointment_booked":
    case "appointment_cancelled":
    case "appointment_rescheduled":
      return role === "doctor" ? "/patients" : role === "hospital_admin" ? "/manage/hospitals" : "/appointments";
    case "consultation_requested":
    case "consultation_accepted":
    case "consultation_rejected":
    case "consultation_completed":
    case "consultation_cancelled":
    case "offer_accepted":
      return "/consultations";
    case "offer_received":
      return "/concerns";
    case "prescription_issued":
      return "/prescriptions";
    case "token_purchase_completed":
      return "/wallet";
    case "withdrawal_processed":
      return "/doctor/earnings";
    case "doctor_verification":
      return "/doctor/profile";
    case "hospital_invitation":
      return "/dashboard";
    case "hospital_verification":
    case "hospital_staff_change":
      return "/manage/hospitals";
    case "pharmacy_verification":
    case "medicine_request":
      return "/manage/pharmacies";
    case "rider_verification":
      return "/rider";
    case "medicine_quote":
      return "/medicines";
    case "medicine_order":
      return role === "pharmacy" ? "/manage/pharmacies" : role === "rider" ? "/rider" : "/medicines";
    default:
      return "/dashboard";
  }
}

const ICONS: Record<string, string> = {
  appointment: "📅",
  consultation: "💬",
  offer: "💬",
  prescription: "📄",
  token: "🪙",
  withdrawal: "💸",
  medicine: "💊",
  hospital: "🏥",
  pharmacy: "💊",
  rider: "🛵",
  doctor: "🩺",
};

export function notificationIcon(kind: string | undefined): string {
  const prefix = (kind ?? "").split("_")[0];
  return ICONS[prefix] ?? "🔔";
}

// "just now", "5 min ago", "3 h ago", else a date; in Bangla when the page is.
export function timeAgo(iso: string | undefined): string {
  if (!iso) return "";
  const bn = typeof document !== "undefined" && document.documentElement.getAttribute("data-lang") === "bn";
  const then = new Date(iso).getTime();
  const mins = Math.round((Date.now() - then) / 60_000);
  if (mins < 1) return bn ? "এইমাত্র" : "just now";
  if (mins < 60) return bn ? `${bnDigits(String(mins))} মিনিট আগে` : `${mins} min ago`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return bn ? `${bnDigits(String(hours))} ঘণ্টা আগে` : `${hours} h ago`;
  const days = Math.round(hours / 24);
  if (days < 7) return bn ? `${bnDigits(String(days))} দিন আগে` : `${days} d ago`;
  return new Date(iso).toLocaleDateString(bn ? "bn-BD" : "en-GB", { day: "numeric", month: "short", timeZone: "Asia/Dhaka" });
}
