import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { QuoteRequest } from "./quote-request";

export default function PharmacyRequestPage() {
  return (
    <Container className="max-w-3xl py-8">
      <Suspense fallback={<Spinner />}>
        <QuoteRequest />
      </Suspense>
    </Container>
  );
}
