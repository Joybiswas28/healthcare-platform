import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { Workspace } from "./workspace";

export default function ManageHospitalPage() {
  return (
    <Container className="py-8">
      <Suspense fallback={<Spinner />}>
        <Workspace />
      </Suspense>
    </Container>
  );
}
