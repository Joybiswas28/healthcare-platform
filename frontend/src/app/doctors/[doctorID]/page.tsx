import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { DoctorProfile } from "./doctor-profile";

export default function DoctorPage() {
  return (
    <Container className="py-8">
      <Suspense fallback={<Spinner />}>
        <DoctorProfile />
      </Suspense>
    </Container>
  );
}
