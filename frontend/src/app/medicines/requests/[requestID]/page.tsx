import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { RequestDetail } from "./request-detail";

export default function MedicineRequestPage() {
  return (
    <Container className="max-w-3xl py-8">
      <Suspense fallback={<Spinner />}>
        <RequestDetail />
      </Suspense>
    </Container>
  );
}
