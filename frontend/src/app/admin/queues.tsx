"use client";

import Link from "next/link";
import { useCallback, useEffect, useState, type FormEvent, type ReactNode } from "react";
import { Alert, Badge, Button, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type Hospital, type Pharmacy, type Rider, type Schemas } from "@/lib/api/client";
import { dateTime } from "@/lib/format";
import { mapLink } from "@/lib/geo";
import { VerifyActions } from "./verify-actions";

type Status = "pending" | "approved" | "rejected";
type AdminDoctor = Schemas["AdminDoctor"];

const STATUS_TONE = { pending: "amber", approved: "teal", rejected: "red" } as const;

// A list filtered by verification status, with paging. `load` fetches one page.
function useQueue<T>(load: (status: Status, offset: number) => Promise<{ items: T[]; error?: unknown }>) {
  const [status, setStatus] = useState<Status>("pending");
  const [items, setItems] = useState<T[] | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [error, setError] = useState("");

  const fetchPage = useCallback(
    async (offset: number) => {
      const res = await load(status, offset);
      if (res.error) {
        setError(errorMessage(res.error));
        return;
      }
      setItems((prev) => (offset === 0 ? res.items : [...(prev ?? []), ...res.items]));
      setHasMore(res.items.length === 50);
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [status],
  );

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setItems(null);
    setError("");
    fetchPage(0);
  }, [fetchPage]);

  // After a decision the item moves to another list; drop it from this one.
  const remove = (pred: (t: T) => boolean) => setItems((prev) => prev?.filter((t) => !pred(t)) ?? null);

  return { status, setStatus, items, hasMore, error, more: () => fetchPage(items?.length ?? 0), remove };
}

function QueueShell({
  status,
  setStatus,
  items,
  hasMore,
  error,
  more,
  empty,
  children,
}: {
  status: Status;
  setStatus: (s: Status) => void;
  items: unknown[] | null;
  hasMore: boolean;
  error: string;
  more: () => void;
  empty: string;
  children: ReactNode;
}) {
  return (
    <div className="space-y-4">
      <div className="flex gap-2">
        {(["pending", "approved", "rejected"] as const).map((s) => (
          <button
            key={s}
            onClick={() => setStatus(s)}
            className={`rounded-full px-3 py-1 text-sm capitalize ${
              status === s ? "bg-teal-700 text-white" : "bg-white text-slate-700 ring-1 ring-slate-200 hover:ring-teal-600"
            }`}
          >
            {s === "pending" ? "Waiting" : s}
          </button>
        ))}
      </div>
      {error && <Alert>{error}</Alert>}
      {items === null ? !error && <Spinner /> : items.length === 0 ? <p className="text-sm text-slate-600">{empty}</p> : children}
      {items && hasMore && (
        <Button variant="secondary" onClick={more}>
          Show more
        </Button>
      )}
    </div>
  );
}

// ---- doctors

export function DoctorsQueue() {
  const q = useQueue<AdminDoctor>(async (status, offset) => {
    const { data, error } = await api.GET("/api/v1/admin/doctors", { params: { query: { status, limit: 50, offset } } });
    return { items: data?.doctors ?? [], error: data ? undefined : error };
  });

  return (
    <QueueShell {...q} empty={q.status === "pending" ? "No doctors waiting for verification." : "None."}>
      <ul className="space-y-3">
        {q.items?.map((d) => (
          <li key={d.user_id}>
            <Card className="space-y-3">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <p className="font-semibold text-slate-900">{d.name}</p>
                  <p className="text-sm text-slate-600">
                    {[d.specialization, d.degrees].filter(Boolean).join(" · ") || "No specialty given"}
                    {d.experience_years ? ` · ${d.experience_years} years` : ""}
                  </p>
                  <p className="text-sm text-slate-700">
                    BMDC <span className="font-mono font-medium">{d.bmdc_reg_no ?? "— not given —"}</span> · {d.phone}
                  </p>
                  <p className="text-xs text-slate-500">Signed up {dateTime(d.created_at)}</p>
                  {d.verification_note && <p className="text-xs text-red-700">Note: {d.verification_note}</p>}
                </div>
                <Badge tone={STATUS_TONE[d.verification_status ?? "pending"]}>{d.verification_status}</Badge>
              </div>
              {q.status === "pending" && !d.bmdc_reg_no && (
                <Alert kind="info">The doctor hasn&apos;t entered a BMDC number yet. Check it on the BMDC register before approving.</Alert>
              )}
              <VerifyActions
                current={d.verification_status}
                onDecide={async (body) => {
                  const { error } = await api.POST("/api/v1/admin/doctors/{doctorID}/verification", {
                    params: { path: { doctorID: d.user_id ?? "" } },
                    body,
                  });
                  if (error) return errorMessage(error);
                  q.remove((x) => x.user_id === d.user_id);
                  return "";
                }}
              />
              {d.verification_status === "approved" && <CommissionEditor doctorID={d.user_id ?? ""} current={d.commission_bps} />}
            </Card>
          </li>
        ))}
      </ul>
    </QueueShell>
  );
}

function CommissionEditor({ doctorID, current }: { doctorID: string; current?: number | null }) {
  const [value, setValue] = useState(current);
  const [msg, setMsg] = useState("");

  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const percent = Number(new FormData(e.currentTarget).get("percent"));
    const { data, error } = await api.PUT("/api/v1/admin/doctors/{doctorID}/commission", {
      params: { path: { doctorID } },
      body: { commission_bps: Math.round(percent * 100) },
    });
    if (data) {
      setValue(data.commission_bps);
      setMsg("Saved.");
    } else setMsg(errorMessage(error));
  }

  return (
    <form onSubmit={save} className="flex flex-wrap items-center gap-2 border-t border-slate-100 pt-3 text-sm">
      <span className="text-slate-700">Platform share of online consultation fees:</span>
      <input
        name="percent"
        type="number"
        min={0}
        max={100}
        step={0.5}
        defaultValue={value != null ? value / 100 : ""}
        placeholder="default"
        aria-label="Commission percent"
        className="w-24 rounded-lg border border-slate-300 px-2 py-1"
      />
      <span>%</span>
      <Button type="submit" variant="secondary">
        Save
      </Button>
      {msg && <span className="text-xs text-slate-500">{msg}</span>}
    </form>
  );
}

// ---- hospitals

export function HospitalsQueue() {
  const q = useQueue<Hospital>(async (status, offset) => {
    const { data, error } = await api.GET("/api/v1/admin/hospitals", { params: { query: { status, limit: 50, offset } } });
    return { items: data?.hospitals ?? [], error: data ? undefined : error };
  });

  return (
    <QueueShell {...q} empty={q.status === "pending" ? "No hospitals waiting for verification." : "None."}>
      <ul className="space-y-3">
        {q.items?.map((h) => {
          const map = mapLink(h.latitude, h.longitude);
          return (
            <li key={h.id}>
              <Card className="space-y-3">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <p className="font-semibold text-slate-900">{h.name}</p>
                    <p className="text-sm text-slate-700">
                      DGHS licence <span className="font-mono font-medium">{h.registration_no}</span>
                    </p>
                    <p className="text-sm text-slate-600">
                      {[h.address, h.area, h.district].filter(Boolean).join(", ")} · {h.phone}
                      {map && (
                        <>
                          {" · "}
                          <a href={map} target="_blank" rel="noopener noreferrer" className="text-teal-800 underline">
                            Map
                          </a>
                        </>
                      )}
                    </p>
                    <p className="text-xs text-slate-500">Registered {dateTime(h.created_at)}</p>
                    {h.verification_note && <p className="text-xs text-red-700">Note: {h.verification_note}</p>}
                  </div>
                  <Badge tone={STATUS_TONE[h.verification_status ?? "pending"]}>{h.verification_status}</Badge>
                </div>
                {h.verification_status === "approved" && (
                  <Link href={`/hospitals/${h.id}`} className="text-sm text-teal-800 hover:underline">
                    Public page
                  </Link>
                )}
                <VerifyActions
                  current={h.verification_status}
                  onDecide={async (body) => {
                    const { error } = await api.POST("/api/v1/admin/hospitals/{hospitalID}/verification", {
                      params: { path: { hospitalID: h.id ?? "" } },
                      body,
                    });
                    if (error) return errorMessage(error);
                    q.remove((x) => x.id === h.id);
                    return "";
                  }}
                />
              </Card>
            </li>
          );
        })}
      </ul>
    </QueueShell>
  );
}

// ---- pharmacies

export function PharmaciesQueue() {
  const q = useQueue<Pharmacy>(async (status, offset) => {
    const { data, error } = await api.GET("/api/v1/admin/pharmacies", { params: { query: { status, limit: 50, offset } } });
    return { items: data?.pharmacies ?? [], error: data ? undefined : error };
  });

  return (
    <QueueShell {...q} empty={q.status === "pending" ? "No pharmacies waiting for verification." : "None."}>
      <ul className="space-y-3">
        {q.items?.map((p) => {
          const map = mapLink(p.latitude, p.longitude);
          return (
            <li key={p.id}>
              <Card className="space-y-3">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <p className="font-semibold text-slate-900">{p.name}</p>
                    <p className="text-sm text-slate-700">
                      DGDA drug licence <span className="font-mono font-medium">{p.drug_license_no}</span>
                    </p>
                    <p className="text-sm text-slate-600">
                      {[p.address, p.area, p.district].filter(Boolean).join(", ")} · {p.phone}
                      {map && (
                        <>
                          {" · "}
                          <a href={map} target="_blank" rel="noopener noreferrer" className="text-teal-800 underline">
                            Map
                          </a>
                        </>
                      )}
                    </p>
                    <p className="text-xs text-slate-500">Registered {dateTime(p.created_at)}</p>
                    {p.verification_note && <p className="text-xs text-red-700">Note: {p.verification_note}</p>}
                  </div>
                  <Badge tone={STATUS_TONE[p.verification_status ?? "pending"]}>{p.verification_status}</Badge>
                </div>
                <VerifyActions
                  current={p.verification_status}
                  onDecide={async (body) => {
                    const { error } = await api.POST("/api/v1/admin/pharmacies/{pharmacyID}/verification", {
                      params: { path: { pharmacyID: p.id ?? "" } },
                      body,
                    });
                    if (error) return errorMessage(error);
                    q.remove((x) => x.id === p.id);
                    return "";
                  }}
                />
              </Card>
            </li>
          );
        })}
      </ul>
    </QueueShell>
  );
}

// ---- riders

const VEHICLES: Record<string, string> = { bicycle: "Bicycle", motorcycle: "Motorcycle", car: "Car", on_foot: "On foot" };

export function RidersQueue() {
  const q = useQueue<Rider>(async (status, offset) => {
    const { data, error } = await api.GET("/api/v1/admin/riders", { params: { query: { status, limit: 50, offset } } });
    return { items: data?.riders ?? [], error: data ? undefined : error };
  });

  return (
    <QueueShell {...q} empty={q.status === "pending" ? "No riders waiting for verification." : "None."}>
      <ul className="space-y-3">
        {q.items?.map((r) => (
          <li key={r.user_id}>
            <Card className="space-y-3">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <p className="font-semibold text-slate-900">{r.name}</p>
                  <p className="text-sm text-slate-700">
                    NID <span className="font-mono font-medium">{r.nid_number}</span> · {VEHICLES[r.vehicle ?? ""] ?? r.vehicle}
                  </p>
                  <p className="text-sm text-slate-600">{r.phone}</p>
                  <p className="text-xs text-slate-500">Applied {dateTime(r.created_at)}</p>
                  {r.verification_note && <p className="text-xs text-red-700">Note: {r.verification_note}</p>}
                </div>
                <Badge tone={STATUS_TONE[r.verification_status ?? "pending"]}>{r.verification_status}</Badge>
              </div>
              <VerifyActions
                current={r.verification_status}
                onDecide={async (body) => {
                  const { error } = await api.POST("/api/v1/admin/riders/{userID}/verification", {
                    params: { path: { userID: r.user_id ?? "" } },
                    body,
                  });
                  if (error) return errorMessage(error);
                  q.remove((x) => x.user_id === r.user_id);
                  return "";
                }}
              />
            </Card>
          </li>
        ))}
      </ul>
    </QueueShell>
  );
}
