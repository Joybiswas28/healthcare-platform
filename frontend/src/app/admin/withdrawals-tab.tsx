"use client";

import { useEffect, useState, type FormEvent } from "react";
import { Alert, Badge, Button, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type Schemas } from "@/lib/api/client";
import { dateTime, taka } from "@/lib/format";

type Withdrawal = Schemas["Withdrawal"];
type Status = "requested" | "paid" | "rejected";

const METHODS: Record<string, string> = { bkash: "bKash", nagad: "Nagad", bank: "Bank transfer" };
const TONE = { requested: "amber", paid: "teal", rejected: "red" } as const;

export function WithdrawalsTab() {
  const [status, setStatus] = useState<Status>("requested");
  const [items, setItems] = useState<Withdrawal[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    api.GET("/api/v1/admin/withdrawals", { params: { query: { status, limit: 50 } } }).then(({ data, error }) => {
      if (cancelled) return;
      if (data) setItems(data.withdrawals ?? []);
      else setError(errorMessage(error));
    });
    return () => {
      cancelled = true;
    };
  }, [status]);

  return (
    <div className="space-y-4">
      <div className="flex gap-2">
        {(["requested", "paid", "rejected"] as const).map((s) => (
          <button
            key={s}
            onClick={() => {
              setItems(null);
              setStatus(s);
            }}
            className={`rounded-full px-3 py-1 text-sm capitalize ${
              status === s ? "bg-teal-700 text-white" : "bg-white text-slate-700 ring-1 ring-slate-200 hover:ring-teal-600"
            }`}
          >
            {s === "requested" ? "To pay" : s}
          </button>
        ))}
      </div>
      {error && <Alert>{error}</Alert>}
      {items === null ? (
        !error && <Spinner />
      ) : items.length === 0 ? (
        <p className="text-sm text-slate-600">{status === "requested" ? "No withdrawals to pay." : "None."}</p>
      ) : (
        <ul className="space-y-3">
          {items.map((w) => (
            <li key={w.id}>
              <WithdrawalCard
                withdrawal={w}
                onDone={() => setItems((prev) => prev?.filter((x) => x.id !== w.id) ?? null)}
              />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function WithdrawalCard({ withdrawal: w, onDone }: { withdrawal: Withdrawal; onDone: () => void }) {
  const [mode, setMode] = useState<"paid" | "rejected" | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!mode || !w.id) return;
    const value = String(new FormData(e.currentTarget).get("value")).trim();
    setBusy(true);
    setError("");
    const { data, error } = await api.POST("/api/v1/admin/withdrawals/{withdrawalID}/process", {
      params: { path: { withdrawalID: w.id } },
      body: mode === "paid" ? { status: "paid", payout_ref: value } : { status: "rejected", note: value },
    });
    setBusy(false);
    if (data) onDone();
    else setError(errorMessage(error));
  }

  return (
    <Card className="space-y-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="font-semibold text-slate-900">{w.doctor_name}</p>
          <p className="text-sm text-slate-700">
            {METHODS[w.method ?? ""] ?? w.method}: <span className="font-mono">{w.account_details}</span>
          </p>
          <p className="text-xs text-slate-500">
            Requested {dateTime(w.created_at)}
            {w.processed_at ? ` · processed ${dateTime(w.processed_at)}` : ""}
          </p>
          {w.payout_ref && <p className="text-xs text-slate-600">Payout ref: {w.payout_ref}</p>}
          {w.admin_note && <p className="text-xs text-red-700">Note: {w.admin_note}</p>}
        </div>
        <div className="text-right">
          <p className="text-xl font-semibold text-slate-900">{taka(w.amount_paisa)}</p>
          <Badge tone={TONE[w.status ?? "requested"]}>{w.status}</Badge>
        </div>
      </div>

      {w.status === "requested" &&
        (mode ? (
          <form onSubmit={submit} className="flex flex-wrap items-end gap-2">
            <label className="min-w-60 flex-1 text-sm">
              <span className="mb-1 block font-medium text-slate-700">
                {mode === "paid" ? "Transaction ID of the payment you sent" : "Reason (the doctor sees it; money goes back to their balance)"}
              </span>
              <input
                name="value"
                required
                autoFocus
                className="w-full rounded-lg border border-slate-300 px-3 py-2 outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-600/20"
              />
            </label>
            <Button type="submit" variant={mode === "paid" ? "primary" : "danger"} disabled={busy}>
              {mode === "paid" ? "Mark paid" : "Reject"}
            </Button>
            <Button type="button" variant="secondary" onClick={() => setMode(null)}>
              Back
            </Button>
          </form>
        ) : (
          <div className="flex gap-2">
            <Button onClick={() => setMode("paid")}>I&apos;ve sent the money</Button>
            <Button variant="danger" onClick={() => setMode("rejected")}>
              Reject
            </Button>
          </div>
        ))}
      {error && <Alert>{error}</Alert>}
    </Card>
  );
}
