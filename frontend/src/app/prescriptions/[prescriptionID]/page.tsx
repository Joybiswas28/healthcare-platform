import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { PrescriptionDetail } from "./prescription-detail";

export default function PrescriptionPage() {
  return (
    <Container className="max-w-3xl py-8">
      <Suspense fallback={<Spinner />}>
        <PrescriptionDetail />
      </Suspense>
    </Container>
  );
}
