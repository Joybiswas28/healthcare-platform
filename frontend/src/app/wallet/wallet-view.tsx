"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Alert, Button, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type LedgerEntry, type TokenPackage, type Wallet } from "@/lib/api/client";
import { dateTime, prettyDate, taka, tokens } from "@/lib/format";

const KIND_LABELS: Record<string, string> = {
  token_purchase: "Bought tokens",
  consultation_hold: "Paid for a consultation",
  consultation_refund: "Refund",
  token_expiry: "Tokens expired",
};

// Without SSLCommerz configured the backend hands out a development-only
// checkout URL that marks the payment as paid when POSTed.
const DEV_CHECKOUT = /^\/api\/v1\/dev\/payments\/([0-9a-f-]+)\/succeed$/;

export function WalletView() {
  const router = useRouter();
  const [wallet, setWallet] = useState<Wallet | null>(null);
  const [packages, setPackages] = useState<TokenPackage[]>([]);
  const [tokenValue, setTokenValue] = useState(10000);
  const [history, setHistory] = useState<LedgerEntry[]>([]);
  const [error, setError] = useState("");
  const [buying, setBuying] = useState<string | null>(null);

  useEffect(() => {
    Promise.all([
      api.GET("/api/v1/wallet"),
      api.GET("/api/v1/token-packages"),
      api.GET("/api/v1/wallet/history", { params: { query: { limit: 20 } } }),
    ]).then(([w, p, h]) => {
      if (w.data) setWallet(w.data);
      else setError(errorMessage(w.error));
      setPackages(p.data?.packages ?? []);
      if (p.data?.token_value_paisa) setTokenValue(p.data.token_value_paisa);
      setHistory(h.data?.entries ?? []);
    });
  }, []);

  async function buy(pkg: TokenPackage) {
    if (!pkg.id) return;
    setBuying(pkg.id);
    setError("");
    const { data, error } = await api.POST("/api/v1/wallet/purchases", { body: { package_id: pkg.id } });
    if (!data?.checkout_url || !data.id) {
      setBuying(null);
      setError(errorMessage(error, "Could not start the payment."));
      return;
    }
    const dev = DEV_CHECKOUT.exec(data.checkout_url);
    if (dev) {
      if (!confirm(`Development mode: no real payment is taken. Pretend to pay ${taka(pkg.price_paisa)}?`)) {
        setBuying(null);
        return;
      }
      const res = await api.POST("/api/v1/dev/payments/{paymentID}/succeed", {
        params: { path: { paymentID: dev[1] } },
      });
      router.push(`/payments/result?status=${res.data ? "success" : "failed"}&payment_id=${data.id}`);
      return;
    }
    // SSLCommerz: leave the site to pay; it sends the browser back to /payments/result.
    window.location.assign(data.checkout_url);
  }

  if (!wallet && !error) return <Spinner />;

  return (
    <div className="space-y-6">
      {error && <Alert>{error}</Alert>}

      {wallet && (
        <Card>
          <p className="text-sm text-slate-600">Available</p>
          <p className="text-3xl font-semibold text-slate-900">{tokens(wallet.tokens_available)}</p>
          <p className="text-xs text-slate-500">1 token = {taka(wallet.token_value_paisa ?? tokenValue)}</p>
          {(wallet.lots ?? []).length > 0 && (
            <ul className="mt-4 space-y-1 border-t border-slate-100 pt-3 text-sm text-slate-600">
              {wallet.lots!.map((lot) => (
                <li key={lot.id}>
                  {tokens(lot.tokens_remaining)} of {lot.tokens_total} · expires{" "}
                  {prettyDate(lot.expires_at?.slice(0, 10))}
                </li>
              ))}
            </ul>
          )}
        </Card>
      )}

      <section>
        <h2 className="mb-3 text-lg font-semibold text-slate-900">Buy tokens</h2>
        {packages.length === 0 && <p className="text-sm text-slate-600">No packages are on sale right now.</p>}
        <div className="grid gap-3 sm:grid-cols-3">
          {packages.map((p) => (
            <Card key={p.id}>
              <h3 className="font-semibold text-slate-900">{p.name}</h3>
              <p className="mt-1 text-2xl font-semibold text-teal-800">{taka(p.price_paisa)}</p>
              <p className="text-sm text-slate-600">
                {tokens(p.tokens)} · valid {p.validity_days} days
              </p>
              <Button className="mt-4 w-full" onClick={() => buy(p)} disabled={buying !== null}>
                {buying === p.id ? "Opening payment…" : "Buy"}
              </Button>
            </Card>
          ))}
        </div>
      </section>

      {history.length > 0 && (
        <section>
          <h2 className="mb-3 text-lg font-semibold text-slate-900">History</h2>
          <Card className="p-0">
            <ul className="divide-y divide-slate-100 text-sm">
              {history.map((e, i) => (
                <li key={i} className="flex justify-between gap-3 px-5 py-3">
                  <span>
                    {KIND_LABELS[e.kind ?? ""] ?? e.kind}
                    <span className="block text-xs text-slate-500">{dateTime(e.created_at)}</span>
                  </span>
                  <span className={(e.amount_paisa ?? 0) < 0 ? "text-red-700" : "text-emerald-700"}>
                    {(e.amount_paisa ?? 0) > 0 ? "+" : ""}
                    {tokens(Math.round((e.amount_paisa ?? 0) / tokenValue))}
                  </span>
                </li>
              ))}
            </ul>
          </Card>
        </section>
      )}
    </div>
  );
}
