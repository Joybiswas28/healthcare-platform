import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { PaymentResult } from "./payment-result";

// SSLCommerz sends the browser here (FRONTEND_URL/payments/result) after paying.
export default function PaymentResultPage() {
  return (
    <Container className="max-w-lg py-12">
      <Suspense fallback={<Spinner />}>
        <PaymentResult />
      </Suspense>
    </Container>
  );
}
