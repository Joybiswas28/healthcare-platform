"use client";

import { useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import { Alert, ButtonLink, Card, Spinner } from "@/components/ui";
import { useT } from "@/components/use-lang";
import { api, type Payment } from "@/lib/api/client";
import { tokens } from "@/lib/format";
import { T } from "@/components/t";

// The status in the URL only says how the browser came back. The payment's real
// state comes from the backend, which re-checks it with the provider; a payment
// can stay "pending" for a little while, so keep asking for up to a minute.
export function PaymentResult() {
  const t = useT();
  const params = useSearchParams();
  const paymentID = params.get("payment_id");
  const urlStatus = params.get("status");
  const [payment, setPayment] = useState<Payment | null>(null);
  const [gaveUp, setGaveUp] = useState(false);

  useEffect(() => {
    if (!paymentID) return;
    let stopped = false;
    let tries = 0;
    async function check() {
      const { data } = await api.GET("/api/v1/wallet/purchases/{paymentID}", {
        params: { path: { paymentID: paymentID! } },
      });
      if (stopped) return;
      if (data) setPayment(data);
      if (data?.status === "pending" && ++tries < 20) setTimeout(check, 3000);
      else if (data?.status === "pending") setGaveUp(true);
    }
    check();
    return () => {
      stopped = true;
    };
  }, [paymentID]);

  if (!paymentID) {
    return <Alert>{urlStatus === "cancelled" ? "You cancelled the payment." : "No payment to show."}</Alert>;
  }
  if (!payment) return <Spinner label="Checking your payment…" />;

  return (
    <Card className="space-y-4">
      {payment.status === "paid" && (
        <Alert kind="success">{t("Payment received. {tokens} added to your wallet.", { tokens: tokens(payment.tokens) })}</Alert>
      )}
      {payment.status === "failed" && (
        <Alert>
          {t(urlStatus === "cancelled" ? "You cancelled the payment." : "The payment did not go through.")}{" "}
          {t("No money was taken for tokens.")}
        </Alert>
      )}
      {payment.status === "pending" &&
        (gaveUp ? (
          <Alert kind="info">
            <T>The payment is still being confirmed. Your tokens will appear in your wallet as soon as it is.</T>
          </Alert>
        ) : (
          <Spinner label="Waiting for the payment provider to confirm…" />
        ))}
      <div className="flex gap-2">
        <ButtonLink href="/wallet">Go to wallet</ButtonLink>
        <ButtonLink href="/consultations" variant="secondary">
          <T>My consultations</T>
        </ButtonLink>
      </div>
    </Card>
  );
}
