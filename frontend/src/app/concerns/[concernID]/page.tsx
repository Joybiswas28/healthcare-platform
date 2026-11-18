import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { ConcernDetail } from "./concern-detail";

export default function ConcernPage() {
  return (
    <Container className="max-w-3xl py-8">
      <Suspense fallback={<Spinner />}>
        <ConcernDetail />
      </Suspense>
    </Container>
  );
}
