import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { ConsultationRoom } from "./consultation-room";

export default function ConsultationPage() {
  return (
    <Container className="max-w-3xl py-8">
      <Suspense fallback={<Spinner />}>
        <ConsultationRoom />
      </Suspense>
    </Container>
  );
}
