import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { RiderOrder } from "./rider-order";

export default function RiderOrderPage() {
  return (
    <Container className="max-w-2xl py-8">
      <Suspense fallback={<Spinner />}>
        <RiderOrder />
      </Suspense>
    </Container>
  );
}
