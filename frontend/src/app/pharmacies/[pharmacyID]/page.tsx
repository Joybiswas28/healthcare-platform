import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { PharmacyProfile } from "./pharmacy-profile";

export default function PharmacyPage() {
  return (
    <Container className="max-w-3xl py-8">
      <Suspense fallback={<Spinner />}>
        <PharmacyProfile />
      </Suspense>
    </Container>
  );
}
