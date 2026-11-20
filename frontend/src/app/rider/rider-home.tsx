"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { OrderBadge } from "@/components/order-status";
import { Alert, Badge, Button, Card, Field, Select, Spinner } from "@/components/ui";
import { api, errorMessage, type Job, type Order, type Rider } from "@/lib/api/client";
import { dateTime, taka } from "@/lib/format";
import { currentPosition, km } from "@/lib/geo";
import { VERIFICATION } from "@/app/manage/hospitals/my-hospitals";
import { T } from "@/components/t";
import { useT } from "@/components/use-lang";

// While on duty and this page is open, report the position this often.
const LOCATION_EVERY_MS = 60_000;
const JOBS_EVERY_MS = 20_000;

const VEHICLES = { bicycle: "Bicycle", motorcycle: "Motorcycle", car: "Car", on_foot: "On foot" } as const;

export function RiderHome() {
  const [rider, setRider] = useState<Rider | null>(null);
  const [missing, setMissing] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    api.GET("/api/v1/rider/profile").then(({ data, error, response }) => {
      if (data) setRider(data);
      else if (response.status === 404) setMissing(true);
      else setError(errorMessage(error));
    });
  }, []);

  if (error) return <Alert>{error}</Alert>;
  if (!rider && !missing) return <Spinner />;
  if (!rider || rider.verification_status !== "approved") {
    return <ProfileForm rider={rider} onSaved={(r) => (setRider(r), setMissing(false))} />;
  }
  return <OnDuty rider={rider} onChange={setRider} />;
}

function ProfileForm({ rider, onSaved }: { rider: Rider | null; onSaved: (r: Rider) => void }) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const t = useT();

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    setBusy(true);
    setError("");
    const { data, error } = await api.PUT("/api/v1/rider/profile", {
      body: {
        nid_number: String(f.get("nid")).replace(/\s+/g, ""),
        vehicle: String(f.get("vehicle")) as keyof typeof VEHICLES,
      },
    });
    setBusy(false);
    if (data) onSaved(data);
    else setError(errorMessage(error));
  }

  const v = rider ? VERIFICATION[rider.verification_status ?? "pending"] : null;
  return (
    <Card className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="font-semibold text-slate-900"><T>Rider profile</T></h2>
        {v && <Badge tone={v.tone}>{v.label}</Badge>}
      </div>
      {!rider && <p className="text-sm text-slate-600"><T>Add your NID and vehicle. An admin verifies you before you can take deliveries.</T></p>}
      {rider?.verification_status === "pending" && (
        <Alert kind="info">An admin is checking your details. You can still correct them below.</Alert>
      )}
      {rider?.verification_status === "rejected" && (
        <Alert>
          {rider.verification_note ? (
            <T vars={{ note: rider.verification_note }}>{"Verification was rejected: {note}. Correct the details and save to ask again."}</T>
          ) : (
            <T>Verification was rejected. Correct the details and save to ask again.</T>
          )}
        </Alert>
      )}
      <form onSubmit={submit} className="grid gap-4 sm:grid-cols-2">
        <Field label="National ID number" name="nid" inputMode="numeric" required defaultValue={rider?.nid_number} hint="10, 13 or 17 digits" />
        <Select label="Vehicle" name="vehicle" defaultValue={rider?.vehicle ?? "motorcycle"}>
          {Object.entries(VEHICLES).map(([k, label]) => (
            <option key={k} value={k}>
              {t(label)}
            </option>
          ))}
        </Select>
        {error && (
          <div className="sm:col-span-2">
            <Alert>{error}</Alert>
          </div>
        )}
        <div>
          <Button type="submit" disabled={busy}>
            {busy ? "Saving…" : "Save"}
          </Button>
        </div>
      </form>
    </Card>
  );
}

function OnDuty({ rider, onChange }: { rider: Rider; onChange: (r: Rider) => void }) {
  const router = useRouter();
  const [jobs, setJobs] = useState<Job[] | null>(null);
  const [orders, setOrders] = useState<Order[]>([]);
  const [earnings, setEarnings] = useState<{ deliveries?: number; earning_paisa?: number } | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const on = !!rider.available;
  const t = useT();

  const loadOrders = useCallback(async () => {
    const [o, e] = await Promise.all([api.GET("/api/v1/rider/orders", { params: { query: { limit: 20 } } }), api.GET("/api/v1/rider/earnings")]);
    setOrders(o.data?.orders ?? []);
    if (e.data) setEarnings(e.data);
  }, []);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    loadOrders();
  }, [loadOrders]);

  // Jobs and position updates only while on duty.
  useEffect(() => {
    if (!on) return;
    let stopped = false;
    async function loadJobs() {
      const { data, error } = await api.GET("/api/v1/rider/jobs");
      if (stopped) return;
      if (data) setJobs(data.jobs ?? []);
      else setError(errorMessage(error));
    }
    async function report() {
      try {
        const c = await currentPosition();
        await api.PUT("/api/v1/rider/location", { body: c });
      } catch {
        // The next attempt tries again; the duty toggle already showed any permission problem.
      }
    }
    loadJobs();
    const j = setInterval(loadJobs, JOBS_EVERY_MS);
    const l = setInterval(report, LOCATION_EVERY_MS);
    return () => {
      stopped = true;
      clearInterval(j);
      clearInterval(l);
    };
  }, [on]);

  async function toggle() {
    setBusy(true);
    setError("");
    try {
      const body = on ? { available: false } : { available: true, ...(await currentPosition()) };
      const { data, error } = await api.PUT("/api/v1/rider/availability", { body });
      if (data) onChange(data);
      else setError(errorMessage(error));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
    setBusy(false);
  }

  async function accept(job: Job) {
    if (!job.order_id || !confirm(t("Take this delivery from {pharmacy}? You'll collect {amount} cash.", { pharmacy: job.pharmacy_name, amount: taka(job.cash_to_collect_paisa) }))) return;
    setBusy(true);
    const { data, error } = await api.POST("/api/v1/rider/jobs/{orderID}/accept", { params: { path: { orderID: job.order_id } } });
    setBusy(false);
    if (data?.id) router.push(`/rider/orders/${data.id}`);
    else setError(errorMessage(error));
  }

  const current = orders.filter((o) => o.status === "rider_assigned" || o.status === "out_for_delivery");
  const done = orders.filter((o) => o.status === "delivered" || o.status === "cancelled");

  return (
    <div className="space-y-6">
      <Card className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <p className="font-semibold text-slate-900">
            <T>{on ? "You're on duty" : "You're off duty"}</T>
          </p>
          <p className="text-sm text-slate-600">
            <T>{on ? "Keep this page open so pharmacies and patients see where you are." : "Go on duty to see deliveries near you."}</T>
          </p>
        </div>
        <Button onClick={toggle} disabled={busy} variant={on ? "secondary" : "primary"}>
          {on ? "Go off duty" : "Go on duty"}
        </Button>
      </Card>

      {error && <Alert>{error}</Alert>}

      {current.length > 0 && (
        <section className="space-y-3">
          <h2 className="text-lg font-semibold text-slate-900"><T>Current delivery</T></h2>
          {current.map((o) => (
            <OrderRow key={o.id} order={o} />
          ))}
        </section>
      )}

      {on && current.length === 0 && (
        <section className="space-y-3">
          <h2 className="text-lg font-semibold text-slate-900"><T>Jobs near you</T></h2>
          {jobs === null ? (
            <Spinner />
          ) : jobs.length === 0 ? (
            <p className="text-sm text-slate-600"><T>No deliveries waiting within 5 km. This list refreshes by itself.</T></p>
          ) : (
            jobs.map((j) => (
              <Card key={j.order_id}>
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <p className="font-semibold text-slate-900">{j.pharmacy_name}</p>
                    <p className="text-sm text-slate-600">{j.pharmacy_address}</p>
                    <p className="text-sm text-slate-600">
                      <T vars={{ a: km(j.distance_to_pharmacy_km), b: km(j.delivery_distance_km) }}>{"{a} to the pharmacy · {b} to the patient"}</T>
                    </p>
                  </div>
                  <div className="text-right">
                    <p className="text-lg font-semibold text-teal-800">
                      <T vars={{ amount: taka(j.earning_paisa) }}>{"You earn {amount}"}</T>
                    </p>
                    <p className="text-xs text-slate-500">
                      <T vars={{ amount: taka(j.cash_to_collect_paisa) }}>{"Collect {amount} cash"}</T>
                    </p>
                  </div>
                </div>
                <Button className="mt-3" onClick={() => accept(j)} disabled={busy}>
                  <T>Take this job</T>
                </Button>
              </Card>
            ))
          )}
        </section>
      )}

      {earnings && (
        <Card className="flex flex-wrap justify-between gap-3">
          <span className="text-sm text-slate-600">
            <T vars={{ n: earnings.deliveries ?? 0 }}>{"Completed deliveries: {n}"}</T>
          </span>
          <span className="text-sm font-semibold text-slate-900">
            <T vars={{ amount: taka(earnings.earning_paisa) }}>{"Earned {amount}"}</T>
          </span>
        </Card>
      )}

      {done.length > 0 && (
        <details>
          <summary className="cursor-pointer text-sm text-slate-600">
            <T vars={{ n: done.length }}>{"Past deliveries ({n})"}</T>
          </summary>
          <div className="mt-3 space-y-3">
            {done.map((o) => (
              <OrderRow key={o.id} order={o} />
            ))}
          </div>
        </details>
      )}
    </div>
  );
}

function OrderRow({ order: o }: { order: Order }) {
  return (
    <Link href={`/rider/orders/${o.id}`} className="block rounded-xl border border-slate-200 bg-surface p-4 shadow-sm transition hover:border-teal-600">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="font-semibold text-slate-900">{o.pharmacy_name} → {o.patient_name}</p>
          <p className="text-sm text-slate-600">
            <T vars={{ amount: taka(o.total_paisa) }}>{"Collect {amount} cash"}</T>
          </p>
        </div>
        <OrderBadge order={o} />
      </div>
      <p className="mt-1 text-xs text-slate-500">{dateTime(o.created_at)}</p>
    </Link>
  );
}
