"use client";

import Link from "next/link";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Alert, Badge, Button, Card, Field, Select, Spinner } from "@/components/ui";
import { api, errorMessage, type LedgerEntry, type Schemas } from "@/lib/api/client";
import { dateTime, taka } from "@/lib/format";

type Withdrawal = Schemas["Withdrawal"];

const MIN_WITHDRAWAL_PAISA = 50_000; // BDT 500, the backend's minimum
const METHODS = { bkash: "bKash", nagad: "Nagad", bank: "Bank transfer" } as const;
const TONE = { requested: "amber", paid: "teal", rejected: "red" } as const;

const KIND: Record<string, string> = {
  consultation_release: "Consultation completed",
  withdrawal_request: "Withdrawal requested",
  withdrawal_paid: "Withdrawal paid",
  withdrawal_rejected: "Withdrawal returned",
};

export function EarningsView() {
  const [balance, setBalance] = useState<number | null>(null);
  const [pending, setPending] = useState(0);
  const [entries, setEntries] = useState<LedgerEntry[]>([]);
  const [withdrawals, setWithdrawals] = useState<Withdrawal[]>([]);
  const [error, setError] = useState("");

  const reload = useCallback(async () => {
    const [e, w] = await Promise.all([
      api.GET("/api/v1/doctor/earnings", { params: { query: { limit: 30 } } }),
      api.GET("/api/v1/doctor/withdrawals", { params: { query: { limit: 20 } } }),
    ]);
    if (!e.data) {
      setError(errorMessage(e.error));
      return;
    }
    setBalance(e.data.balance_paisa ?? 0);
    setPending(e.data.pending_withdrawal_paisa ?? 0);
    setEntries(e.data.entries ?? []);
    setWithdrawals(w.data?.withdrawals ?? []);
  }, []);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    reload();
  }, [reload]);

  if (balance === null) return error ? <Alert>{error}</Alert> : <Spinner />;
  const hasPending = withdrawals.some((w) => w.status === "requested");

  return (
    <div className="space-y-6">
      <div className="grid gap-4 sm:grid-cols-2">
        <Card>
          <p className="text-sm text-slate-600">Available to withdraw</p>
          <p className="text-3xl font-semibold text-slate-900">{taka(balance)}</p>
          <p className="mt-1 text-xs text-slate-500">
            From completed online consultations, after the platform&apos;s share. Chamber fees are paid to you directly.
          </p>
        </Card>
        <Card>
          <p className="text-sm text-slate-600">Waiting to be paid out</p>
          <p className="text-3xl font-semibold text-slate-900">{taka(pending)}</p>
          <p className="mt-1 text-xs text-slate-500">Withdrawals an admin hasn&apos;t sent yet.</p>
        </Card>
      </div>

      {hasPending ? (
        <Alert kind="info">You have a withdrawal waiting. You can request another once it is paid or rejected.</Alert>
      ) : balance < MIN_WITHDRAWAL_PAISA ? (
        <Alert kind="info">You can withdraw once you have at least {taka(MIN_WITHDRAWAL_PAISA)}.</Alert>
      ) : (
        <WithdrawForm balance={balance} onDone={reload} />
      )}

      {withdrawals.length > 0 && (
        <section className="space-y-2">
          <h2 className="text-lg font-semibold text-slate-900">Withdrawals</h2>
          <Card className="p-0">
            <ul className="divide-y divide-slate-100 text-sm">
              {withdrawals.map((w) => (
                <li key={w.id} className="flex flex-wrap items-center justify-between gap-3 px-5 py-3">
                  <span>
                    <span className="font-medium text-slate-900">{taka(w.amount_paisa)}</span> to{" "}
                    {METHODS[w.method ?? "bkash"]} {w.account_details}
                    <span className="block text-xs text-slate-500">
                      {dateTime(w.created_at)}
                      {w.payout_ref ? ` · paid, ref ${w.payout_ref}` : ""}
                      {w.admin_note ? ` · ${w.admin_note}` : ""}
                    </span>
                  </span>
                  <Badge tone={TONE[w.status ?? "requested"]}>{w.status}</Badge>
                </li>
              ))}
            </ul>
          </Card>
        </section>
      )}

      <section className="space-y-2">
        <h2 className="text-lg font-semibold text-slate-900">History</h2>
        {entries.length === 0 ? (
          <p className="text-sm text-slate-600">
            No earnings yet. Turn on online consultations in your{" "}
            <Link href="/dashboard" className="text-teal-800 underline">
              dashboard
            </Link>
            .
          </p>
        ) : (
          <Card className="p-0">
            <ul className="divide-y divide-slate-100 text-sm">
              {entries.map((e, i) => (
                <li key={i} className="flex justify-between gap-3 px-5 py-3">
                  <span>
                    {e.kind === "consultation_release" && e.reference_id ? (
                      <Link href={`/consultations/${e.reference_id}`} className="hover:underline">
                        {KIND[e.kind]}
                      </Link>
                    ) : (
                      (KIND[e.kind ?? ""] ?? e.kind)
                    )}
                    <span className="block text-xs text-slate-500">{dateTime(e.created_at)}</span>
                  </span>
                  <span className={(e.amount_paisa ?? 0) < 0 ? "text-red-700" : "text-emerald-700"}>
                    {(e.amount_paisa ?? 0) > 0 ? "+" : ""}
                    {taka(e.amount_paisa)}
                  </span>
                </li>
              ))}
            </ul>
          </Card>
        )}
      </section>
    </div>
  );
}

function WithdrawForm({ balance, onDone }: { balance: number; onDone: () => void }) {
  const [method, setMethod] = useState<keyof typeof METHODS>("bkash");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const amount = Math.round(Number(f.get("amount")) * 100);
    if (amount < MIN_WITHDRAWAL_PAISA || amount > balance) {
      setError(`Enter between ${taka(MIN_WITHDRAWAL_PAISA)} and ${taka(balance)}.`);
      return;
    }
    setBusy(true);
    setError("");
    const { error } = await api.POST("/api/v1/doctor/withdrawals", {
      body: { amount_paisa: amount, method, account_details: String(f.get("account")).trim() },
    });
    setBusy(false);
    if (error) setError(errorMessage(error));
    else onDone();
  }

  return (
    <Card>
      <form onSubmit={submit} className="space-y-3">
        <h2 className="font-semibold text-slate-900">Withdraw</h2>
        <div className="grid gap-3 sm:grid-cols-[10rem_10rem_1fr]">
          <Field label="Amount (৳)" name="amount" type="number" min={500} max={balance / 100} step={1} required defaultValue={Math.floor(balance / 100)} />
          <Select label="Send to" value={method} onChange={(e) => setMethod(e.target.value as keyof typeof METHODS)}>
            {Object.entries(METHODS).map(([k, label]) => (
              <option key={k} value={k}>
                {label}
              </option>
            ))}
          </Select>
          <Field
            label={method === "bank" ? "Bank, branch, account name and number" : `${METHODS[method]} number`}
            name="account"
            required
            placeholder={method === "bank" ? "e.g. City Bank, Gulshan, A. Siddiqua, 1234567890" : "01XXXXXXXXX"}
          />
        </div>
        {error && <Alert>{error}</Alert>}
        <Button type="submit" disabled={busy}>
          {busy ? "Requesting…" : "Request withdrawal"}
        </Button>
        <p className="text-xs text-slate-500">An admin sends the money and records the transaction ID here.</p>
      </form>
    </Card>
  );
}
