import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { PharmacyWorkspace } from "./workspace";

export default function ManagePharmacyPage() {
  return (
    <Container className="py-8">
      <Suspense fallback={<Spinner />}>
        <PharmacyWorkspace />
      </Suspense>
    </Container>
  );
}
