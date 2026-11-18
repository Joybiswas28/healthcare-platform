import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { OrderDetail } from "./order-detail";

export default function MedicineOrderPage() {
  return (
    <Container className="max-w-3xl py-8">
      <Suspense fallback={<Spinner />}>
        <OrderDetail />
      </Suspense>
    </Container>
  );
}
