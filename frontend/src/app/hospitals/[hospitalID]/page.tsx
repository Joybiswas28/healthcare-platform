import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { HospitalProfile } from "./hospital-profile";

export default function HospitalPage() {
  return (
    <Container className="py-8">
      <Suspense fallback={<Spinner />}>
        <HospitalProfile />
      </Suspense>
    </Container>
  );
}
