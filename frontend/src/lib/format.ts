// Helpers for the API's conventions: money in paisa, dates as YYYY-MM-DD and
// times as HH:MM, all in Bangladesh time (UTC+6, no daylight saving).

const DHAKA_OFFSET_MS = 6 * 60 * 60 * 1000;

export const WEEKDAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

export function taka(paisa: number | null | undefined): string {
  if (paisa == null) return "—";
  const amount = paisa / 100;
  return `৳${amount.toLocaleString("en-IN", { maximumFractionDigits: 2 })}`;
}

// "17:00" -> "5:00 PM"
export function clock(hhmm: string | undefined): string {
  if (!hhmm) return "";
  const [h, m] = hhmm.split(":").map(Number);
  const suffix = h >= 12 ? "PM" : "AM";
  const hour = h % 12 === 0 ? 12 : h % 12;
  return `${hour}:${String(m).padStart(2, "0")} ${suffix}`;
}

// Today's date in Dhaka as a UTC-midnight Date, so the UTC getters give the
// Dhaka calendar date whatever the browser's own time zone is.
function dhakaToday(): Date {
  const now = new Date(Date.now() + DHAKA_OFFSET_MS);
  return new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
}

function isoDate(d: Date): string {
  return d.toISOString().slice(0, 10);
}

// The dates in the next `days` days (today included) that fall on `weekday`.
// The backend accepts bookings up to 30 days ahead.
export function upcomingDates(weekday: number, days = 30): string[] {
  const start = dhakaToday();
  const out: string[] = [];
  for (let i = 0; i <= days; i++) {
    const d = new Date(start.getTime() + i * 86_400_000);
    if (d.getUTCDay() === weekday) out.push(isoDate(d));
  }
  return out;
}

export function todayInDhaka(): string {
  return isoDate(dhakaToday());
}

// "2026-11-20" -> "Fri, 20 Nov 2026"
export function prettyDate(iso: string | undefined): string {
  if (!iso) return "";
  const d = new Date(`${iso}T00:00:00Z`);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("en-GB", { weekday: "short", day: "numeric", month: "short", year: "numeric", timeZone: "UTC" });
}

// A timestamp shown in Dhaka time: "18 Nov, 9:41 PM".
export function dateTime(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("en-GB", {
    day: "numeric",
    month: "short",
    hour: "numeric",
    minute: "2-digit",
    hour12: true,
    timeZone: "Asia/Dhaka",
  });
}

export function fileSize(bytes: number | undefined): string {
  if (!bytes) return "";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function tokens(n: number | undefined): string {
  return `${n ?? 0} token${n === 1 ? "" : "s"}`;
}

export const ROLE_LABELS: Record<string, string> = {
  patient: "Patient",
  doctor: "Doctor",
  hospital_admin: "Hospital admin",
  pharmacy: "Pharmacy",
  rider: "Delivery rider",
  admin: "Admin",
};
