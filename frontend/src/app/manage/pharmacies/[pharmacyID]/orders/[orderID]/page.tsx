import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { PharmacyOrder } from "./pharmacy-order";

export default function PharmacyOrderPage() {
  return (
    <Container className="max-w-3xl py-8">
      <Suspense fallback={<Spinner />}>
        <PharmacyOrder />
      </Suspense>
    </Container>
  );
}
